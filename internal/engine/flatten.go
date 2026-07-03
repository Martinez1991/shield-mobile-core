package engine

import (
	"fmt"
	"strings"

	"shield/internal/ir"
	"shield/internal/smali"
)

// passFlatten rewrites eligible methods into a central-dispatcher ("flattened")
// form: the basic blocks become cases of a packed-switch driven by a state
// register, and every block ends by setting the next state and jumping back to
// the dispatcher. A static CFG reconstruction then sees a flat switch instead of
// the real edges (shield-platform.md §3.2/8; the acceptance criterion of #20).
//
// The transform is gated by the typed IR (internal/ir): only methods whose
// registers are provably int-typed throughout are flattened, so the dispatcher's
// join point cannot introduce a verifier type conflict (a register that is int
// on every path stays int when all blocks merge at the switch). The state
// register is a genuinely-dead local found via liveness — no register-count
// growth. Reference/wide widening (which needs consistent-type analysis across
// the new dispatcher edges) is future work.
//
// It runs after code virtualization and skips VM wrappers; a flattened method
// contains a packed-switch, which the reorder pass already bails on, so the two
// control-flow passes do not fight.
func passFlatten(classes []*smali.Class, includePrefixes []string, seed int64) int {
	fid := 0
	total := 0
	for _, c := range classes {
		if !isOwned(c.Descriptor, includePrefixes) {
			continue
		}
		forEachMethod(c, func(block []string) []string {
			out, ok := flattenMethod(block, &fid)
			if ok {
				total++
			}
			return out
		})
	}
	return total
}

func flattenMethod(block []string, fid *int) ([]string, bool) {
	start := firstBodyIndex(block)
	if start < 0 {
		return block, false
	}
	end := len(block) - 1
	for i := len(block) - 1; i >= 0; i-- {
		if strings.TrimSpace(block[i]) == ".end method" {
			end = i
			break
		}
	}
	if start >= end {
		return block, false
	}
	code := block[start:end]

	// Bail on layout-sensitive constructs and on already-virtualized bodies.
	for _, ln := range code {
		t := strings.TrimSpace(ln)
		if strings.Contains(t, "Lshield/rt/VM;->run(") {
			return block, false
		}
		for _, tok := range bailTokens {
			if strings.HasPrefix(t, tok) {
				return block, false
			}
		}
	}

	// Typed-IR gate: the method must parse and every register must hold a single
	// consistent type wherever it is live, and there must be a dead local to
	// carry the dispatch state.
	m, ok := ir.ParseMethod(block)
	if !ok || !consistentTypes(m) {
		return block, false
	}
	stateReg, ok := freeStateRegister(m)
	if !ok {
		return block, false
	}

	blocks, ok := splitBlocks(code)
	if !ok || len(blocks) < 2 {
		return block, false
	}
	if blocks[len(blocks)-1].fallsThrough() {
		return block, false // would fall off the end of the dispatcher
	}

	id := *fid
	*fid++
	for i := range blocks {
		if blocks[i].label == "" {
			blocks[i].label = fmt.Sprintf(":fl_%d_%d", id, i)
		}
	}
	labelToState := map[string]int{}
	for i, b := range blocks {
		labelToState[b.label] = i
	}

	dispatch := fmt.Sprintf(":fl_%d_disp", id)
	swData := fmt.Sprintf(":fl_%d_sw", id)
	setState := func(state int) []string {
		return []string{
			fmt.Sprintf("    const/16 v%d, 0x%x", stateReg, state),
			"    goto " + dispatch,
		}
	}

	var body []string
	body = append(body, fmt.Sprintf("    const/16 v%d, 0x0", stateReg)) // enter block 0
	body = append(body, "    "+dispatch)
	body = append(body, fmt.Sprintf("    packed-switch v%d, %s", stateReg, swData))
	body = append(body, "    goto "+dispatch) // default: unreachable (state always valid)

	for i, b := range blocks {
		body = append(body, "    "+b.label)
		instrs := b.instrs
		term := ""
		if b.term != "none" && len(instrs) > 0 {
			term = strings.TrimSpace(instrs[len(instrs)-1])
			instrs = instrs[:len(instrs)-1]
		}
		body = append(body, instrs...)

		switch {
		case b.term == "none": // fall through to the next block
			body = append(body, setState(i+1)...)
		case strings.HasPrefix(term, "goto"):
			s, ok := labelToState[lastToken(term)]
			if !ok {
				return block, false
			}
			body = append(body, setState(s)...)
		case strings.HasPrefix(term, "return"), term == "throw", strings.HasPrefix(term, "throw "):
			body = append(body, "    "+term) // leaves the dispatch loop
		case strings.HasPrefix(term, "if-"):
			ts, ok := labelToState[lastToken(term)]
			if !ok {
				return block, false
			}
			taken := fmt.Sprintf(":fl_%d_%d_t", id, i)
			body = append(body, "    "+replaceLastToken(term, taken))
			body = append(body, setState(i+1)...) // not taken -> next block
			body = append(body, "    "+taken)
			body = append(body, setState(ts)...) // taken -> target block
		default:
			return block, false // unmodelled terminator
		}
	}

	body = append(body, "    "+swData)
	body = append(body, "    .packed-switch 0x0")
	for _, b := range blocks {
		body = append(body, "        "+b.label)
	}
	body = append(body, "    .end packed-switch")

	out := make([]string, 0, len(block)+len(body))
	out = append(out, block[:start]...)
	out = append(out, body...)
	out = append(out, block[end:]...)
	return out, true
}

// consistentTypes reports whether every register holds a single, consistent type
// at every program point where it is defined. That is the condition under which
// flattening's central dispatcher — where all blocks merge — cannot create a
// Dalvik verifier type conflict: a register that is (say) always a reference
// stays a reference when the blocks join, so any block that reads it still
// verifies. A slot reused with two different types (int here, ref there), or one
// the IR already sees as conflicted, is refused.
func consistentTypes(m *ir.Method) bool {
	types := m.InferTypes()
	seen := make([]ir.RegType, m.Regs)
	for i := range seen {
		seen[i] = ir.TUnknown
	}
	for i := range types {
		for r := 0; r < m.Regs; r++ {
			t := types[i].Reg(r)
			if t == ir.TUnknown {
				continue
			}
			if t == ir.TConflict {
				return false
			}
			if seen[r] == ir.TUnknown {
				seen[r] = t
			} else if seen[r] != t {
				return false // slot reused with an incompatible type
			}
		}
	}
	return true
}

// freeStateRegister returns a local register that is never live anywhere in the
// method (safe to repurpose as the dispatch state) — found via liveness.
func freeStateRegister(m *ir.Method) (int, bool) {
	lv := m.Liveness()
	everLive := map[int]bool{}
	for i := range m.Insns {
		for r := range lv.In(i) {
			everLive[r] = true
		}
		for r := range lv.Out(i) {
			everLive[r] = true
		}
	}
	for r := 0; r < m.ParamBase; r++ { // locals only; never a parameter slot
		if !everLive[r] {
			return r, true
		}
	}
	return 0, false
}

func lastToken(line string) string {
	f := strings.Fields(line)
	if len(f) == 0 {
		return ""
	}
	return strings.TrimSuffix(f[len(f)-1], ",")
}

func replaceLastToken(line, repl string) string {
	f := strings.Fields(line)
	if len(f) == 0 {
		return line
	}
	f[len(f)-1] = repl
	return strings.Join(f, " ")
}
