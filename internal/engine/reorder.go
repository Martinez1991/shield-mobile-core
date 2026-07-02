package engine

import (
	"fmt"
	"strings"

	"shield/internal/smali"
)

// passReorder physically shuffles the basic blocks of each method and reconnects
// them with explicit gotos (shield-platform.md section 7 "Instruction
// Reordering"; a layout-level step toward the flattening of section 3.2/8).
//
// Safe by construction: execution order and every register's def/use order are
// unchanged — only the physical layout in the file moves, so the Dalvik verifier
// (which ignores layout) sees an identical program. Full dispatcher-based
// flattening with a polymorphic ISA needs type/liveness reconstruction and
// runtime verification and remains on the roadmap.
//
// Methods with try/catch, switches, array-data payloads or monitors are skipped
// (bail) to keep the transformation provably correct. Returns methods reordered.
func passReorder(classes []*smali.Class, seed int64) int {
	mid := 0
	total := 0
	for _, c := range classes {
		forEachMethod(c, func(block []string) []string {
			out, ok := reorderMethod(block, seed, &mid)
			if ok {
				total++
			}
			return out
		})
	}
	return total
}

type blk struct {
	label  string   // start label incl ':' ("" if none)
	instrs []string // instruction lines (original indentation kept)
	term   string   // "uncond" | "cond" | "none"
}

func (b *blk) fallsThrough() bool { return b.term != "uncond" }

var bailTokens = []string{
	".catch", ".catchall", ":try_", ".packed-switch", ".sparse-switch",
	".array-data", "packed-switch ", "sparse-switch ", "fill-array-data",
	"monitor-enter", "monitor-exit",
}

func reorderMethod(block []string, seed int64, mid *int) ([]string, bool) {
	start := firstBodyIndex(block)
	if start < 0 {
		return block, false
	}
	end := len(block) - 1 // ".end method"
	for i := len(block) - 1; i >= 0; i-- {
		if strings.TrimSpace(block[i]) == ".end method" {
			end = i
			break
		}
	}
	if start >= end { // malformed (e.g. .end method before any body): bail safely
		return block, false
	}
	code := block[start:end]

	// Bail on constructs whose correctness depends on layout/ranges.
	for _, ln := range code {
		t := strings.TrimSpace(ln)
		for _, tok := range bailTokens {
			if strings.HasPrefix(t, tok) {
				return block, false
			}
		}
	}

	blocks, ok := splitBlocks(code)
	if !ok || len(blocks) < 3 {
		return block, false
	}
	// Last block must terminate (not fall off the end).
	if blocks[len(blocks)-1].fallsThrough() {
		return block, false
	}

	// Ensure every fall-through successor (indices 1..n-1) has a label.
	id := *mid
	*mid++
	for i := 1; i < len(blocks); i++ {
		if blocks[i].label == "" {
			blocks[i].label = fmt.Sprintf(":shf_%d_%d", id, i)
		}
	}

	order := permuteOrder(len(blocks), uint64(seed)^uint64(id)*0x9E3779B97F4A7C15)

	var body []string
	for pos, bi := range order {
		b := blocks[bi]
		if b.label != "" {
			body = append(body, "    "+b.label)
		}
		body = append(body, b.instrs...)
		if b.fallsThrough() {
			succ := bi + 1
			if !(pos+1 < len(order) && order[pos+1] == succ) {
				body = append(body, "    goto "+blocks[succ].label)
			}
		}
	}

	out := make([]string, 0, len(block))
	out = append(out, block[:start]...)
	out = append(out, body...)
	out = append(out, block[end:]...)
	return out, true
}

// splitBlocks partitions the code lines into basic blocks. Blanks and comments
// are dropped (non-semantic). Returns false if a block would start with a
// move-result/move-exception (must stay attached to its producer).
func splitBlocks(code []string) ([]*blk, bool) {
	var blocks []*blk
	var cur *blk
	newBlk := func(label string) {
		cur = &blk{label: label, term: "none"}
		blocks = append(blocks, cur)
	}
	for _, ln := range code {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, ":") {
			newBlk(t)
			continue
		}
		if cur == nil || cur.term != "none" {
			if strings.HasPrefix(t, "move-result") || strings.HasPrefix(t, "move-exception") {
				return nil, false // would orphan a result move
			}
			newBlk("")
		}
		cur.instrs = append(cur.instrs, ln)
		switch {
		case strings.HasPrefix(t, "goto"), strings.HasPrefix(t, "return"), t == "throw", strings.HasPrefix(t, "throw "):
			cur.term = "uncond"
		case strings.HasPrefix(t, "if-"):
			cur.term = "cond"
		}
	}
	return blocks, true
}

// permuteOrder returns a permutation of [0,n) with index 0 fixed (method entry),
// deterministic in seed.
func permuteOrder(n int, seed uint64) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	s := seed | 1
	for i := n - 1; i >= 2; i-- {
		s = s*6364136223846793005 + 1442695040888963407
		j := 1 + int((s>>33)%uint64(i)) // in [1, i]
		order[i], order[j] = order[j], order[i]
	}
	return order
}
