package engine

import (
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"

	"shield/internal/smali"
)

// Code virtualization — the proprietary VM (shield-platform.md section 8).
//
// This is a deliberately *surgical* MVP (section 8.2): it virtualizes only
// straight-line integer methods (static, int params, int return, no branches or
// calls). Such a method is compiled to a tiny register-based bytecode and its
// body is replaced by a call to an injected interpreter Lshield/rt/VM;->run.
//
// Polymorphism (section 8.1): the opcode→handler mapping is a per-build
// permutation derived from the seed, so a dump of one build's VM does not help
// reverse another. Everything is validated: Go compiler/executor equivalence,
// the interpreter algorithm cross-checked on the JVM, and the emitted smali
// assembles to a valid DEX.

// logical opcodes (wire values are permuted per build)
const (
	opLoadArg = iota // dest, argIdx
	opConst          // dest, imm32
	opMove           // dest, src
	opAdd            // dest, a, b
	opSub
	opMul
	opAnd
	opOr
	opXor
	opAddLit // dest, src, imm32
	opMulLit // dest, src, imm32
	opRet    // src
	opCount
)

// vmPermutation returns wire[logical] = byte, a per-build bijection of [0,opCount).
func vmPermutation(seed int64) []byte {
	order := permuteFull(opCount, uint64(seed)*0xD1B54A32D192ED03+0x9E3779B97F4A7C15)
	wire := make([]byte, opCount)
	for i, v := range order {
		wire[i] = byte(v)
	}
	return wire
}

// permuteFull returns a full permutation of [0,n) (unlike permuteOrder which
// pins index 0).
func permuteFull(n int, seed uint64) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	s := seed | 1
	for i := n - 1; i >= 1; i-- {
		s = s*6364136223846793005 + 1442695040888963407
		j := int((s >> 33) % uint64(i+1))
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// --- compiler: smali straight-line int method -> bytecode ----------------

var (
	vmMethodDeclRE = regexp.MustCompile(`^\s*\.method\s+((?:[\w-]+\s+)*)([\w$<>]+)\((.*)\)([\w$/;\[]+)\s*$`)
	vmRegsRE       = regexp.MustCompile(`^\s*\.(registers|locals)\s+(\d+)\s*$`)
)

// compileMethod returns the bytecode for a virtualizable method, or ok=false.
func compileMethod(block []string, wire []byte) ([]byte, bool) {
	decl := block[0]
	m := vmMethodDeclRE.FindStringSubmatch(decl)
	if m == nil {
		return nil, false
	}
	flags, params, ret := m[1], m[3], m[4]
	if !strings.Contains(" "+flags, " static ") || ret != "I" {
		return nil, false
	}
	nargs, ok := countIntParams(params)
	if !ok || nargs == 0 {
		return nil, false // only all-int params (and at least one)
	}

	// register layout
	regKind, regCount, ok := findRegs(block)
	if !ok {
		return nil, false
	}
	numRegs := regCount
	paramBase := regCount - nargs
	if regKind == "locals" {
		numRegs = regCount + nargs
		paramBase = regCount
	}
	if numRegs <= 0 || numRegs > 255 || paramBase < 0 {
		return nil, false
	}

	var code []byte
	emit := func(b ...byte) { code = append(code, b...) }
	emitImm := func(v int64) {
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], uint32(int32(v)))
		code = append(code, buf[:]...)
	}
	reg := func(tok string) (byte, bool) {
		idx, ok := parseReg(tok, paramBase)
		if !ok || idx < 0 || idx >= numRegs {
			return 0, false
		}
		return byte(idx), true
	}

	// load args into their param registers
	for i := 0; i < nargs; i++ {
		emit(wire[opLoadArg], byte(paramBase+i), byte(i))
	}

	body := methodBody(block)
	for _, ln := range body {
		t := strings.TrimSpace(ln)
		fields := splitOperands(t)
		op := fields[0]
		switch {
		case op == "const/4" || op == "const/16" || op == "const":
			d, ok1 := reg(fields[1])
			v, ok2 := parseLit(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			emit(wire[opConst], d)
			emitImm(v)
		case op == "move" || op == "move/16" || op == "move/from16":
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			emit(wire[opMove], d, s)
		case bin3[op] != 0:
			d, ok1 := reg(fields[1])
			a, ok2 := reg(fields[2])
			b, ok3 := reg(fields[3])
			if !ok1 || !ok2 || !ok3 {
				return nil, false
			}
			emit(wire[bin3[op]-1], d, a, b)
		case bin2[op] != 0:
			d, ok1 := reg(fields[1])
			b, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			emit(wire[bin2[op]-1], d, d, b) // dest = dest OP src
		case op == "add-int/lit8" || op == "add-int/lit16":
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			v, ok3 := parseLit(fields[3])
			if !ok1 || !ok2 || !ok3 {
				return nil, false
			}
			emit(wire[opAddLit], d, s)
			emitImm(v)
		case op == "mul-int/lit8" || op == "mul-int/lit16":
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			v, ok3 := parseLit(fields[3])
			if !ok1 || !ok2 || !ok3 {
				return nil, false
			}
			emit(wire[opMulLit], d, s)
			emitImm(v)
		case op == "return":
			s, ok := reg(fields[1])
			if !ok {
				return nil, false
			}
			emit(wire[opRet], s)
		case op == "nop":
			// skip
		default:
			return nil, false // unsupported instruction: bail
		}
	}
	// prepend numRegs header
	return append([]byte{byte(numRegs)}, code...), true
}

// bin3[op] = logical+1 for 3-address ALU ops (0 means not present).
var bin3 = map[string]int{
	"add-int": opAdd + 1, "sub-int": opSub + 1, "mul-int": opMul + 1,
	"and-int": opAnd + 1, "or-int": opOr + 1, "xor-int": opXor + 1,
}

// bin2[op] = logical+1 for /2addr ALU ops.
var bin2 = map[string]int{
	"add-int/2addr": opAdd + 1, "sub-int/2addr": opSub + 1, "mul-int/2addr": opMul + 1,
	"and-int/2addr": opAnd + 1, "or-int/2addr": opOr + 1, "xor-int/2addr": opXor + 1,
}

// vmExec is the Go reference interpreter (mirrors the injected smali VM.run).
func vmExec(code []byte, args []int32, wire []byte) int32 {
	inv := make([]int, opCount)
	for logical, w := range wire {
		inv[w] = logical
	}
	numRegs := int(code[0])
	r := make([]int32, numRegs)
	pc := 1
	rd := func() byte { b := code[pc]; pc++; return b }
	imm := func() int32 { v := int32(binary.BigEndian.Uint32(code[pc:])); pc += 4; return v }
	for pc < len(code) {
		switch inv[code[pc]] {
		case opLoadArg:
			pc++
			d, ai := rd(), rd()
			r[d] = args[ai]
		case opConst:
			pc++
			d := rd()
			r[d] = imm()
		case opMove:
			pc++
			d, s := rd(), rd()
			r[d] = r[s]
		case opAdd:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = r[a] + r[b]
		case opSub:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = r[a] - r[b]
		case opMul:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = r[a] * r[b]
		case opAnd:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = r[a] & r[b]
		case opOr:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = r[a] | r[b]
		case opXor:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = r[a] ^ r[b]
		case opAddLit:
			pc++
			d, s := rd(), rd()
			r[d] = r[s] + imm()
		case opMulLit:
			pc++
			d, s := rd(), rd()
			r[d] = r[s] * imm()
		case opRet:
			pc++
			s := rd()
			return r[s]
		default:
			return -1
		}
	}
	return -1
}

// --- smali parsing helpers ------------------------------------------------

func countIntParams(params string) (int, bool) {
	n := 0
	for i := 0; i < len(params); i++ {
		switch params[i] {
		case 'I':
			n++
		default:
			return 0, false // non-int param -> not virtualizable
		}
	}
	return n, true
}

func findRegs(block []string) (string, int, bool) {
	for _, ln := range block {
		if m := vmRegsRE.FindStringSubmatch(ln); m != nil {
			n, _ := strconv.Atoi(m[2])
			return m[1], n, true
		}
	}
	return "", 0, false
}

func methodBody(block []string) []string {
	start := firstBodyIndex(block)
	if start < 0 {
		return nil
	}
	end := len(block) - 1
	for i := len(block) - 1; i >= 0; i-- {
		if strings.TrimSpace(block[i]) == ".end method" {
			end = i
			break
		}
	}
	return block[start:end]
}

func splitOperands(t string) []string {
	t = strings.ReplaceAll(t, ",", " ")
	return strings.Fields(t)
}

func parseReg(tok string, paramBase int) (int, bool) {
	tok = strings.TrimSpace(strings.TrimSuffix(tok, ","))
	if len(tok) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(tok[1:])
	if err != nil {
		return 0, false
	}
	switch tok[0] {
	case 'v':
		return n, true
	case 'p':
		return paramBase + n, true
	}
	return 0, false
}

func parseLit(tok string) (int64, bool) {
	tok = strings.TrimSpace(strings.TrimSuffix(tok, ","))
	neg := false
	if strings.HasPrefix(tok, "-") {
		neg = true
		tok = tok[1:]
	}
	var v int64
	var err error
	if strings.HasPrefix(tok, "0x") || strings.HasPrefix(tok, "0X") {
		v, err = strconv.ParseInt(tok[2:], 16, 64)
	} else {
		v, err = strconv.ParseInt(tok, 10, 64)
	}
	if err != nil {
		return 0, false
	}
	if neg {
		v = -v
	}
	return v, true
}

// --- the pass -------------------------------------------------------------

const vmDescriptor = "Lshield/rt/VM;"

// passVirtualize compiles eligible methods of owned classes to VM bytecode. It
// returns the number of methods virtualized and, if any, the interpreter class
// to inject (the caller appends it so the mutation is explicit).
func passVirtualize(classes []*smali.Class, includePrefixes []string, seed int64, base string) (int, *smali.Class) {
	wire := vmPermutation(seed)
	count := 0
	for _, c := range classes {
		if !isOwned(c.Descriptor, includePrefixes) {
			continue
		}
		forEachMethod(c, func(bk []string) []string {
			code, ok := compileMethod(bk, wire)
			if !ok {
				return bk
			}
			nargs, _ := countIntParams(vmMethodDeclRE.FindStringSubmatch(bk[0])[3])
			count++
			return virtualizedBody(bk[0], nargs, code)
		})
	}
	if count == 0 {
		return 0, nil
	}
	return count, VMClass(base, wire)
}
