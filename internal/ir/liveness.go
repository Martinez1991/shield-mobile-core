package ir

import "strings"

// Liveness holds the per-instruction live register sets computed by backward
// dataflow. A register is live at a point if it may be read before being
// overwritten on some path to method exit — the information a transform needs to
// reuse registers safely (e.g. control-flow flattening) or to know which values
// must survive across a call.
//
// Wide values (long/double) are tracked by their low register, following the
// engine-wide convention that the high half is never addressed independently;
// consumers that allocate wide values as pairs stay correct.
type Liveness struct {
	in  []map[int]bool
	out []map[int]bool
}

// In returns the set of registers live on entry to instruction i.
func (l *Liveness) In(i int) map[int]bool { return l.in[i] }

// Out returns the set of registers live immediately after instruction i.
func (l *Liveness) Out(i int) map[int]bool { return l.out[i] }

// LiveInAt reports whether register r is live on entry to instruction i.
func (l *Liveness) LiveInAt(i, r int) bool { return l.in[i][r] }

// Liveness computes live-in/live-out sets via a backward dataflow fixpoint over
// the CFG (the same successors used by type inference).
func (m *Method) Liveness() *Liveness {
	n := len(m.Insns)
	in := make([]map[int]bool, n)
	out := make([]map[int]bool, n)
	for i := range in {
		in[i] = map[int]bool{}
		out[i] = map[int]bool{}
	}

	for changed := true; changed; {
		changed = false
		for i := n - 1; i >= 0; i-- {
			// live-out[i] = union of live-in over successors.
			no := map[int]bool{}
			for _, s := range m.successors(i) {
				if s < 0 || s >= n {
					continue
				}
				for r := range in[s] {
					no[r] = true
				}
			}
			out[i] = no

			// live-in[i] = (live-out[i] \ def) ∪ use.
			def, hasDef, uses := m.defUse(m.Insns[i])
			ni := map[int]bool{}
			for r := range no {
				if hasDef && r == def {
					continue
				}
				ni[r] = true
			}
			for _, u := range uses {
				ni[u] = true
			}
			if !sameSet(ni, in[i]) {
				in[i] = ni
				changed = true
			}
		}
	}
	return &Liveness{in: in, out: out}
}

func sameSet(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// defUse returns the register defined by an instruction (if any) and the
// registers it reads. Classification is conservative: an unmodelled opcode is
// treated as defining nothing and reading every register operand, which can only
// over-approximate liveness (safe — it never frees a register that is live).
func (m *Method) defUse(insn Insn) (def int, hasDef bool, uses []int) {
	regs := m.regOperands(insn.Args)
	op := insn.Op

	switch {
	// /2addr and check-cast read and write their first register.
	case strings.HasSuffix(op, "/2addr") || op == "check-cast":
		if len(regs) == 0 {
			return 0, false, nil
		}
		return regs[0], true, regs
	// opcodes that define their first register and read the rest.
	case definesArg0(op):
		if len(regs) == 0 {
			return 0, false, nil
		}
		return regs[0], true, regs[1:]
	// everything else (invoke/if/return/aput/sput/iput/throw/monitor/switch/goto/
	// nop/filled-new-array and unmodelled ops): no def, all operands are reads.
	default:
		return 0, false, regs
	}
}

// definesArg0 reports whether an opcode writes its first register operand from
// the remaining ones (loads, moves, allocations, and arithmetic).
func definesArg0(op string) bool {
	switch {
	case strings.HasPrefix(op, "const"): // const*, const-wide*, const-string*, const-class
		return true
	case strings.HasPrefix(op, "move-result"), op == "move-exception":
		return true
	case op == "move" || op == "move/16" || op == "move/from16",
		op == "move-object" || op == "move-object/16" || op == "move-object/from16",
		op == "move-wide" || op == "move-wide/16" || op == "move-wide/from16":
		return true
	case op == "new-instance" || op == "new-array":
		return true
	case op == "array-length" || op == "instance-of":
		return true
	case strings.HasPrefix(op, "aget"): // array loads (aput stores -> not here)
		return true
	case fieldGet[op]: // iget*/sget* loads (iput*/sput* stores -> not here)
		return true
	case convType[op] != 0:
		return true
	}
	if _, ok := binResultType(op); ok { // ALU incl. cmp*
		return true
	}
	return false
}

// regOperands returns the register indices mentioned in an operand list, in
// order. It strips the {} of invoke/filled-new-array lists and expands a
// "{vLo .. vHi}" range form.
func (m *Method) regOperands(args []string) []int {
	hasRange := false
	var regs []int
	for _, a := range args {
		t := strings.Trim(a, "{}")
		if t == ".." {
			hasRange = true
			continue
		}
		if r, ok := parseReg(t, m.ParamBase); ok {
			regs = append(regs, r)
		}
	}
	if hasRange && len(regs) >= 2 {
		lo, hi := regs[0], regs[len(regs)-1]
		if hi >= lo {
			full := make([]int, 0, hi-lo+1)
			for r := lo; r <= hi; r++ {
				full = append(full, r)
			}
			return full
		}
	}
	return regs
}
