package ir

import "strings"

// RegType is a reconstructed Dalvik register type. Dalvik registers are typeless
// slots reused across a method, so a type is only meaningful at a given program
// point; InferTypes computes it per instruction.
type RegType uint8

const (
	TUnknown  RegType = iota // bottom: uninitialised / not yet reached
	TInt                     // 32-bit int family (I, Z, B, S, C, and int results)
	TLong                    // 64-bit long (low half of the pair)
	TFloat                   // 32-bit float
	TDouble                  // 64-bit double (low half of the pair)
	TRef                     // object/array reference
	TWideHigh                // upper half of a Long/Double pair (not directly usable)
	TConflict                // top: incompatible types merged at a join point
)

func (t RegType) String() string {
	switch t {
	case TUnknown:
		return "unknown"
	case TInt:
		return "int"
	case TLong:
		return "long"
	case TFloat:
		return "float"
	case TDouble:
		return "double"
	case TRef:
		return "ref"
	case TWideHigh:
		return "wide-high"
	case TConflict:
		return "conflict"
	}
	return "?"
}

// join is the lattice meet-at-merge: Unknown is bottom (absorbed), equal types
// are preserved, everything else conflicts.
func join(a, b RegType) RegType {
	switch {
	case a == b:
		return a
	case a == TUnknown:
		return b
	case b == TUnknown:
		return a
	default:
		return TConflict
	}
}

// State is the register-type vector at a program point, plus the pending
// invoke/array result type consumed by a following move-result*.
type State struct {
	regs []RegType
	last RegType // return type of the most recent invoke / filled-new-array
}

// Reg returns the reconstructed type of register n (TUnknown if out of range).
func (s State) Reg(n int) RegType {
	if n < 0 || n >= len(s.regs) {
		return TUnknown
	}
	return s.regs[n]
}

func (s State) clone() State {
	r := make([]RegType, len(s.regs))
	copy(r, s.regs)
	return State{regs: r, last: s.last}
}

func (s State) equal(o State) bool {
	if s.last != o.last || len(s.regs) != len(o.regs) {
		return false
	}
	for i := range s.regs {
		if s.regs[i] != o.regs[i] {
			return false
		}
	}
	return true
}

func (s State) join(o State) (State, bool) {
	out := s.clone()
	changed := false
	for i := range out.regs {
		j := join(out.regs[i], o.regs[i])
		if j != out.regs[i] {
			out.regs[i] = j
			changed = true
		}
	}
	if l := join(out.last, o.last); l != out.last {
		out.last = l
		changed = true
	}
	return out, changed
}

// descType maps a single field/param type descriptor to a register type.
func descType(d string) RegType {
	if d == "" {
		return TUnknown
	}
	switch d {
	case "I", "Z", "B", "S", "C":
		return TInt
	case "J":
		return TLong
	case "F":
		return TFloat
	case "D":
		return TDouble
	}
	if d[0] == 'L' || d[0] == '[' {
		return TRef
	}
	return TUnknown
}

// EntryTypes seeds the register-type vector at method entry from the parameter
// list (plus the implicit `this` for instance methods). Non-parameter registers
// start Unknown.
func (m *Method) EntryTypes() State {
	st := State{regs: make([]RegType, m.Regs), last: TUnknown}
	off := m.ParamBase
	set := func(t RegType) {
		if off >= 0 && off < len(st.regs) {
			st.regs[off] = t
		}
		if (t == TLong || t == TDouble) && off+1 < len(st.regs) {
			st.regs[off+1] = TWideHigh
			off += 2
		} else {
			off++
		}
	}
	if !m.Static {
		set(TRef) // this
	}
	for _, d := range SplitDescriptors(m.Params) {
		set(descType(d))
	}
	return st
}

// InferTypes runs a forward dataflow fixpoint over the method's CFG and returns
// the register-type vector *before* each instruction (indexed like m.Insns).
func (m *Method) InferTypes() []State {
	n := len(m.Insns)
	in := make([]State, n)
	for i := range in {
		in[i] = State{regs: make([]RegType, m.Regs), last: TUnknown}
	}
	if n == 0 {
		return in
	}
	in[0] = m.EntryTypes()

	work := make([]bool, n)
	queue := []int{0}
	work[0] = true
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		work[i] = false

		out := m.transfer(in[i], m.Insns[i])
		for _, s := range m.successors(i) {
			if s < 0 || s >= n {
				continue
			}
			merged, changed := in[s].join(out)
			if changed {
				in[s] = merged
				if !work[s] {
					work[s] = true
					queue = append(queue, s)
				}
			}
		}
	}
	return in
}

// successors returns the instruction indices control can reach after index i.
func (m *Method) successors(i int) []int {
	insn := m.Insns[i]
	op := insn.Op
	target := func(tok string) []int {
		if idx, ok := m.Labels[tok]; ok {
			return []int{idx}
		}
		return nil
	}
	switch {
	case op == "goto" || op == "goto/16" || op == "goto/32":
		if len(insn.Args) >= 1 {
			return target(insn.Args[0])
		}
		return nil
	case strings.HasPrefix(op, "return") || op == "throw":
		return nil
	case branch2[op]:
		if len(insn.Args) >= 3 {
			return append([]int{i + 1}, target(insn.Args[2])...)
		}
		return []int{i + 1}
	case branchZ[op]:
		if len(insn.Args) >= 2 {
			return append([]int{i + 1}, target(insn.Args[1])...)
		}
		return []int{i + 1}
	default:
		// packed-switch/sparse-switch fall through here: their payload targets are
		// not modelled yet (brick 1), so only the fallthrough edge is followed.
		return []int{i + 1}
	}
}

var branch2 = map[string]bool{
	"if-eq": true, "if-ne": true, "if-lt": true, "if-ge": true, "if-gt": true, "if-le": true,
}
var branchZ = map[string]bool{
	"if-eqz": true, "if-nez": true, "if-ltz": true, "if-gez": true, "if-gtz": true, "if-lez": true,
}

// transfer applies one instruction's effect on the type state, returning the
// post-state. Only the destination register(s) change; unmodelled opcodes are
// transparent (a brick-1 limitation — a consumer that needs an unmodelled def
// must decline, as the VM already does).
func (m *Method) transfer(st State, insn Insn) State {
	out := st.clone()
	reg := func(idx int) (int, bool) {
		if idx >= len(insn.Args) {
			return 0, false
		}
		return parseReg(insn.Args[idx], m.ParamBase)
	}
	setNarrow := func(idx int, t RegType) {
		if r, ok := reg(idx); ok && r >= 0 && r < len(out.regs) {
			out.regs[r] = t
		}
	}
	setWide := func(idx int, t RegType) {
		if r, ok := reg(idx); ok && r >= 0 && r+1 < len(out.regs) {
			out.regs[r] = t
			out.regs[r+1] = TWideHigh
		}
	}
	copyType := func(dstIdx, srcIdx int) {
		r, ok := reg(srcIdx)
		if !ok || r < 0 || r >= len(out.regs) {
			setNarrow(dstIdx, TUnknown)
			return
		}
		setNarrow(dstIdx, out.regs[r])
	}

	op := insn.Op
	switch {
	// constants
	case op == "const/4" || op == "const/16" || op == "const" || op == "const/high16":
		setNarrow(0, TInt)
	case strings.HasPrefix(op, "const-wide"):
		setWide(0, TLong)
	case op == "const-string" || op == "const-string/jumbo" || op == "const-class":
		setNarrow(0, TRef)

	// moves
	case op == "move" || op == "move/16" || op == "move/from16":
		copyType(0, 1)
	case op == "move-object" || op == "move-object/16" || op == "move-object/from16":
		setNarrow(0, TRef)
	case op == "move-wide" || op == "move-wide/16" || op == "move-wide/from16":
		if r, ok := reg(1); ok && r >= 0 && r < len(out.regs) && out.regs[r] == TDouble {
			setWide(0, TDouble)
		} else {
			setWide(0, TLong)
		}
	case op == "move-result":
		setNarrow(0, narrowOf(out.last))
	case op == "move-result-wide":
		setWide(0, wideOf(out.last))
	case op == "move-result-object":
		setNarrow(0, TRef)
	case op == "move-exception":
		setNarrow(0, TRef)

	// invocations: no register def, but set the pending result type
	case strings.HasPrefix(op, "invoke-"), op == "filled-new-array", op == "filled-new-array/range":
		out.last = returnTypeOf(insn.Args)

	// object/array creation and queries
	case op == "new-instance" || op == "new-array":
		setNarrow(0, TRef)
	case op == "check-cast":
		setNarrow(0, TRef)
	case op == "instance-of" || op == "array-length":
		setNarrow(0, TInt)

	// array loads
	case op == "aget" || op == "aget-boolean" || op == "aget-byte" || op == "aget-char" || op == "aget-short":
		setNarrow(0, TInt)
	case op == "aget-wide":
		setWide(0, TLong)
	case op == "aget-object":
		setNarrow(0, TRef)

	// static/instance field loads: type comes from the field descriptor
	case fieldGet[op]:
		setFromField(insn.Args, setNarrow, setWide)

	// conversions and unary ops (dest, src)
	case convType[op] != 0:
		t := convType[op]
		if t == TLong || t == TDouble {
			setWide(0, t)
		} else {
			setNarrow(0, t)
		}

	default:
		// binary arithmetic/logic: classify by the type word in the mnemonic.
		if t, ok := binResultType(op); ok {
			if t == TLong || t == TDouble {
				setWide(0, t)
			} else {
				setNarrow(0, t)
			}
		}
		// otherwise transparent (no modelled def)
	}
	return out
}

// narrowOf/wideOf coerce a pending result type to the shape move-result expects.
func narrowOf(t RegType) RegType {
	if t == TLong || t == TDouble || t == TWideHigh {
		return TUnknown
	}
	return t
}
func wideOf(t RegType) RegType {
	if t == TDouble {
		return TDouble
	}
	return TLong
}

// convType maps conversion / unary opcodes to their result type.
var convType = map[string]RegType{
	"int-to-long": TLong, "int-to-float": TFloat, "int-to-double": TDouble,
	"long-to-int": TInt, "long-to-float": TFloat, "long-to-double": TDouble,
	"float-to-int": TInt, "float-to-long": TLong, "float-to-double": TDouble,
	"double-to-int": TInt, "double-to-long": TLong, "double-to-float": TFloat,
	"int-to-byte": TInt, "int-to-char": TInt, "int-to-short": TInt,
	"neg-int": TInt, "not-int": TInt,
	"neg-long": TLong, "not-long": TLong,
	"neg-float": TFloat, "neg-double": TDouble,
}

var fieldGet = map[string]bool{
	"sget": true, "sget-wide": true, "sget-object": true,
	"sget-boolean": true, "sget-byte": true, "sget-char": true, "sget-short": true,
	"iget": true, "iget-wide": true, "iget-object": true,
	"iget-boolean": true, "iget-byte": true, "iget-char": true, "iget-short": true,
}

// setFromField types the destination (arg 0) of an *get from the field's
// declared type, found after the ':' in the trailing field reference.
func setFromField(args []string, setNarrow, setWide func(int, RegType)) {
	if len(args) == 0 {
		return
	}
	ref := args[len(args)-1]
	ft := ""
	if k := strings.LastIndexByte(ref, ':'); k >= 0 {
		ft = ref[k+1:]
	}
	t := descType(ft)
	if t == TLong || t == TDouble {
		setWide(0, t)
	} else if t == TUnknown {
		setNarrow(0, TInt) // default for an unparsed primitive field
	} else {
		setNarrow(0, t)
	}
}

// returnTypeOf extracts the return type from an invoke's method reference (the
// last operand, "Lcls;->name(params)Ret") and maps it to a register type. A void
// return yields TUnknown (no move-result follows).
func returnTypeOf(args []string) RegType {
	if len(args) == 0 {
		return TUnknown
	}
	ref := args[len(args)-1]
	k := strings.LastIndexByte(ref, ')')
	if k < 0 || k+1 > len(ref) {
		return TUnknown
	}
	return descType(ref[k+1:])
}

// binResultType classifies a binary arithmetic/logic mnemonic (add-int,
// mul-long/2addr, and-int/lit8, cmp-long, ...) by its type word.
func binResultType(op string) (RegType, bool) {
	switch {
	case strings.HasPrefix(op, "cmp"): // cmp-long, cmpl-float, cmpg-double -> int
		return TInt, true
	case strings.Contains(op, "-long"):
		return TLong, true
	case strings.Contains(op, "-double"):
		return TDouble, true
	case strings.Contains(op, "-float"):
		return TFloat, true
	case strings.Contains(op, "-int") || strings.HasPrefix(op, "rsub-"):
		return TInt, true
	}
	return TUnknown, false
}

// parseReg parses a "vN" or "pN" register token to its absolute index.
func parseReg(tok string, paramBase int) (int, bool) {
	if len(tok) < 2 {
		return 0, false
	}
	n := 0
	for _, c := range tok[1:] {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	switch tok[0] {
	case 'v':
		return n, true
	case 'p':
		return paramBase + n, true
	}
	return 0, false
}
