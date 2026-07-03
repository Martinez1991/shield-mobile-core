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
	// control flow (issue #14): targets are absolute 2-byte bytecode offsets.
	opGoto // target
	opIfEq // a, b, target
	opIfNe
	opIfLt
	opIfGe
	opIfGt
	opIfLe
	opIfEqz // a, target
	opIfNez
	opIfLtz
	opIfGez
	opIfGtz
	opIfLez
	// extended integer ALU (issue #14): 3-addr binops
	opDiv // dest, a, b
	opRem
	opShl
	opShr
	opUshr
	// unary
	opNeg // dest, src
	opNot
	// literal forms (dest, src, imm32)
	opAndLit
	opOrLit
	opXorLit
	opShlLit
	opShrLit
	opUshrLit
	opDivLit
	opRemLit
	opRsubLit // dest = imm - src
	// narrowing conversions (dest, src)
	opI2B // (byte)  sign-extend low 8
	opI2S // (short) sign-extend low 16
	opI2C // (char)  zero-extend low 16
	// 64-bit long ops (issue #14). Wide values live in a parallel long register
	// file indexed by the low register of the smali pair.
	opConstWide // dest, imm64 (8 bytes)
	opMoveWide  // dest, src
	opAddL      // dest, a, b
	opSubL
	opMulL
	opDivL
	opRemL
	opAndL
	opOrL
	opXorL
	opShlL // dest, a, b(int reg)
	opShrL
	opUshrL
	opNegL // dest, src
	opNotL
	opI2L         // destWide, srcInt
	opL2I         // destInt, srcWide
	opCmpLong     // destInt, a, b (both wide)
	opRetWide     // src
	opLoadArgWide // destWide, argIdx (long param)
	// reference/object support (issue #14, objects). Object values live in a
	// third parallel register file (ro) indexed like the int/wide files. Object
	// args arrive in a separate [Ljava/lang/Object; array; the return ABI is
	// unified to Object (primitives boxed as Long).
	opLoadArgObj // destReg, argIdx (object param, from the [Object args)
	opMoveObj    // dest, src (move-object)
	opRetObj     // src (return-object)
	opCount
)

// branch2/branchZ map smali comparison mnemonics to logical opcodes.
var branch2 = map[string]int{
	"if-eq": opIfEq, "if-ne": opIfNe, "if-lt": opIfLt, "if-ge": opIfGe, "if-gt": opIfGt, "if-le": opIfLe,
}
var branchZ = map[string]int{
	"if-eqz": opIfEqz, "if-nez": opIfNez, "if-ltz": opIfLtz, "if-gez": opIfGez, "if-gtz": opIfGtz, "if-lez": opIfLez,
}

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

// vop is one compiled VM instruction. For jumps, bytes holds opcode+reg operands
// and a 2-byte target is appended at emit time once labels are resolved.
type vop struct {
	bytes  []byte
	isJump bool
	label  string
}

func imm4(v int64) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(int32(v)))
	return b[:]
}

func imm8(v int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	return b[:]
}

// compileMethod returns the bytecode for a virtualizable method, or ok=false.
// Supports straight-line integer ops plus branches/labels (issue #14) via a
// two-pass layout that resolves label targets to absolute bytecode offsets.
func compileMethod(block []string, wire []byte) ([]byte, bool) {
	decl := block[0]
	m := vmMethodDeclRE.FindStringSubmatch(decl)
	if m == nil {
		return nil, false
	}
	flags, params, ret := m[1], m[3], m[4]
	if !strings.Contains(" "+flags, " static ") {
		return nil, false
	}
	if _, ok := returnKind(ret); !ok {
		return nil, false
	}
	pinfos, regWidth, ok := parseParams(params)
	if !ok || len(pinfos) == 0 {
		return nil, false
	}
	// The virtualized wrapper marshals each param via aput-wide/aput-object on
	// its param register, which (formats 23x/3rc) must be <= v15. Wrapper temps
	// occupy v0..v5, so bail if the params would spill past v15.
	if 5+regWidth > 15 {
		return nil, false
	}
	regKind, regCount, ok := findRegs(block)
	if !ok {
		return nil, false
	}
	numRegs := regCount
	paramBase := regCount - regWidth
	if regKind == "locals" {
		numRegs = regCount + regWidth
		paramBase = regCount
	}
	if numRegs <= 0 || numRegs > 255 || paramBase < 0 {
		return nil, false
	}
	reg := func(tok string) (byte, bool) {
		idx, ok := parseReg(tok, paramBase)
		if !ok || idx < 0 || idx >= numRegs {
			return 0, false
		}
		return byte(idx), true
	}

	var ops []vop
	labelToOp := map[string]int{}
	var pending []string
	push := func(v vop) {
		for _, l := range pending {
			labelToOp[l] = len(ops)
		}
		pending = nil
		ops = append(ops, v)
	}

	// pass 1a: arg loaders. Primitives read from the [J array (int narrows, long
	// direct); objects read from the [Object array. argIdx indexes the right one.
	for _, pi := range pinfos {
		dest := byte(paramBase + pi.regOff)
		switch pi.kind {
		case 'J':
			push(vop{bytes: []byte{wire[opLoadArgWide], dest, byte(pi.argIdx)}})
		case 'L':
			push(vop{bytes: []byte{wire[opLoadArgObj], dest, byte(pi.argIdx)}})
		default: // 'I'
			push(vop{bytes: []byte{wire[opLoadArg], dest, byte(pi.argIdx)}})
		}
	}

	body := methodBody(block)
	for _, ln := range body {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, ":") {
			pending = append(pending, t) // label; binds to the next emitted op
			continue
		}
		fields := splitOperands(t)
		op := fields[0]
		switch {
		// const/high16's smali operand is already the full 32-bit value (low 16
		// bits zero), so it loads exactly like const — no shift.
		case op == "const/4" || op == "const/16" || op == "const" || op == "const/high16":
			d, ok1 := reg(fields[1])
			v, ok2 := parseLit(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: append([]byte{wire[opConst], d}, imm4(v)...)})
		case op == "move" || op == "move/16" || op == "move/from16":
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[opMove], d, s}})
		case op == "move-object" || op == "move-object/16" || op == "move-object/from16":
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[opMoveObj], d, s}})
		case bin3[op] != 0:
			d, ok1 := reg(fields[1])
			a, ok2 := reg(fields[2])
			b, ok3 := reg(fields[3])
			if !ok1 || !ok2 || !ok3 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[bin3[op]-1], d, a, b}})
		case bin2[op] != 0:
			d, ok1 := reg(fields[1])
			b, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[bin2[op]-1], d, d, b}})
		case litMap[op] != 0:
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			v, ok3 := parseLit(fields[3])
			if !ok1 || !ok2 || !ok3 {
				return nil, false
			}
			push(vop{bytes: append([]byte{wire[litMap[op]-1], d, s}, imm4(v)...)})
		case unary[op] != 0:
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[unary[op]-1], d, s}})
		// --- 64-bit long ops ---
		case op == "const-wide/high16":
			d, ok1 := reg(fields[1])
			v, ok2 := parseLit(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: append([]byte{wire[opConstWide], d}, imm8(v<<48)...)})
		case op == "const-wide" || op == "const-wide/16" || op == "const-wide/32":
			d, ok1 := reg(fields[1])
			v, ok2 := parseLit(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: append([]byte{wire[opConstWide], d}, imm8(v)...)})
		case op == "move-wide" || op == "move-wide/from16" || op == "move-wide/16":
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[opMoveWide], d, s}})
		case binL3[op] != 0:
			d, ok1 := reg(fields[1])
			a, ok2 := reg(fields[2])
			b, ok3 := reg(fields[3])
			if !ok1 || !ok2 || !ok3 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[binL3[op]-1], d, a, b}})
		case binL2[op] != 0:
			d, ok1 := reg(fields[1])
			b, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[binL2[op]-1], d, d, b}})
		case shiftL3[op] != 0:
			d, ok1 := reg(fields[1])
			a, ok2 := reg(fields[2])
			b, ok3 := reg(fields[3]) // b is an INT register
			if !ok1 || !ok2 || !ok3 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[shiftL3[op]-1], d, a, b}})
		case shiftL2[op] != 0:
			d, ok1 := reg(fields[1])
			b, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[shiftL2[op]-1], d, d, b}})
		case unaryL[op] != 0:
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[unaryL[op]-1], d, s}})
		case op == "int-to-long":
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[opI2L], d, s}})
		case op == "long-to-int":
			d, ok1 := reg(fields[1])
			s, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[opL2I], d, s}})
		case op == "cmp-long":
			d, ok1 := reg(fields[1])
			a, ok2 := reg(fields[2])
			b, ok3 := reg(fields[3])
			if !ok1 || !ok2 || !ok3 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[opCmpLong], d, a, b}})
		case op == "return-wide":
			s, ok := reg(fields[1])
			if !ok {
				return nil, false
			}
			push(vop{bytes: []byte{wire[opRetWide], s}})
		case op == "return":
			s, ok := reg(fields[1])
			if !ok {
				return nil, false
			}
			push(vop{bytes: []byte{wire[opRet], s}})
		case op == "return-object":
			s, ok := reg(fields[1])
			if !ok {
				return nil, false
			}
			push(vop{bytes: []byte{wire[opRetObj], s}})
		case op == "goto" || op == "goto/16" || op == "goto/32":
			push(vop{bytes: []byte{wire[opGoto]}, isJump: true, label: fields[1]})
		case branch2[op] != 0:
			a, ok1 := reg(fields[1])
			b, ok2 := reg(fields[2])
			if !ok1 || !ok2 {
				return nil, false
			}
			push(vop{bytes: []byte{wire[branch2[op]], a, b}, isJump: true, label: fields[3]})
		case branchZ[op] != 0:
			a, ok := reg(fields[1])
			if !ok {
				return nil, false
			}
			push(vop{bytes: []byte{wire[branchZ[op]], a}, isJump: true, label: fields[2]})
		case op == "nop":
			// skip (labels carry to the next real op)
		default:
			return nil, false
		}
	}
	// labels at the very end bind past the last op.
	for _, l := range pending {
		labelToOp[l] = len(ops)
	}

	// pass 2: compute offsets (header byte occupies offset 0) then resolve.
	offset := make([]int, len(ops)+1)
	offset[0] = 1
	for i, o := range ops {
		sz := len(o.bytes)
		if o.isJump {
			sz += 2
		}
		offset[i+1] = offset[i] + sz
	}
	labelOffset := func(l string) (int, bool) {
		idx, ok := labelToOp[l]
		if !ok {
			return 0, false
		}
		return offset[idx], true
	}
	code := []byte{byte(numRegs)}
	for _, o := range ops {
		code = append(code, o.bytes...)
		if o.isJump {
			t, ok := labelOffset(o.label)
			if !ok || t > 0xFFFF {
				return nil, false // undefined/oversized jump target
			}
			code = append(code, byte(t>>8), byte(t))
		}
	}
	return code, true
}

// bin3[op] = logical+1 for 3-address ALU ops (0 means not present).
var bin3 = map[string]int{
	"add-int": opAdd + 1, "sub-int": opSub + 1, "mul-int": opMul + 1,
	"and-int": opAnd + 1, "or-int": opOr + 1, "xor-int": opXor + 1,
	"div-int": opDiv + 1, "rem-int": opRem + 1,
	"shl-int": opShl + 1, "shr-int": opShr + 1, "ushr-int": opUshr + 1,
}

// bin2[op] = logical+1 for /2addr ALU ops.
var bin2 = map[string]int{
	"add-int/2addr": opAdd + 1, "sub-int/2addr": opSub + 1, "mul-int/2addr": opMul + 1,
	"and-int/2addr": opAnd + 1, "or-int/2addr": opOr + 1, "xor-int/2addr": opXor + 1,
	"div-int/2addr": opDiv + 1, "rem-int/2addr": opRem + 1,
	"shl-int/2addr": opShl + 1, "shr-int/2addr": opShr + 1, "ushr-int/2addr": opUshr + 1,
}

// unary[op] = logical+1 for unary ALU ops (dest, src).
var unary = map[string]int{
	"neg-int": opNeg + 1, "not-int": opNot + 1,
	"int-to-byte": opI2B + 1, "int-to-short": opI2S + 1, "int-to-char": opI2C + 1,
}

// long binops (3-addr and /2addr) on the wide register file.
var binL3 = map[string]int{
	"add-long": opAddL + 1, "sub-long": opSubL + 1, "mul-long": opMulL + 1,
	"div-long": opDivL + 1, "rem-long": opRemL + 1,
	"and-long": opAndL + 1, "or-long": opOrL + 1, "xor-long": opXorL + 1,
}
var binL2 = map[string]int{
	"add-long/2addr": opAddL + 1, "sub-long/2addr": opSubL + 1, "mul-long/2addr": opMulL + 1,
	"div-long/2addr": opDivL + 1, "rem-long/2addr": opRemL + 1,
	"and-long/2addr": opAndL + 1, "or-long/2addr": opOrL + 1, "xor-long/2addr": opXorL + 1,
}

// long shifts: dest(wide), a(wide), b(INT). 3-addr and /2addr.
var shiftL3 = map[string]int{"shl-long": opShlL + 1, "shr-long": opShrL + 1, "ushr-long": opUshrL + 1}
var shiftL2 = map[string]int{"shl-long/2addr": opShlL + 1, "shr-long/2addr": opShrL + 1, "ushr-long/2addr": opUshrL + 1}

// long unary (dest, src) both wide.
var unaryL = map[string]int{"neg-long": opNegL + 1, "not-long": opNotL + 1}

// litMap[op] = logical+1 for literal ALU ops (dest, src, imm).
var litMap = map[string]int{
	"add-int/lit8": opAddLit + 1, "add-int/lit16": opAddLit + 1,
	"mul-int/lit8": opMulLit + 1, "mul-int/lit16": opMulLit + 1,
	"and-int/lit8": opAndLit + 1, "and-int/lit16": opAndLit + 1,
	"or-int/lit8": opOrLit + 1, "or-int/lit16": opOrLit + 1,
	"xor-int/lit8": opXorLit + 1, "xor-int/lit16": opXorLit + 1,
	"shl-int/lit8": opShlLit + 1, "shr-int/lit8": opShrLit + 1, "ushr-int/lit8": opUshrLit + 1,
	"div-int/lit8": opDivLit + 1, "div-int/lit16": opDivLit + 1,
	"rem-int/lit8": opRemLit + 1, "rem-int/lit16": opRemLit + 1,
	"rsub-int": opRsubLit + 1, "rsub-int/lit8": opRsubLit + 1,
}

const minInt32 = -2147483648

// jdiv/jrem mirror Java/Dalvik int division: MIN/-1 wraps (no overflow trap),
// and /0 is guarded to 0 (the real VM throws; the golden tests never divide by 0).
func jdiv(a, b int32) int32 {
	if b == 0 {
		return 0
	}
	if a == minInt32 && b == -1 {
		return a
	}
	return a / b
}
func jrem(a, b int32) int32 {
	if b == 0 || (a == minInt32 && b == -1) {
		return 0
	}
	return a % b
}

const minInt64 = -9223372036854775808

func jdivL(a, b int64) int64 {
	if b == 0 {
		return 0
	}
	if a == minInt64 && b == -1 {
		return a
	}
	return a / b
}
func jremL(a, b int64) int64 {
	if b == 0 || (a == minInt64 && b == -1) {
		return 0
	}
	return a % b
}
func cmp64(a, b int64) int32 {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// vmResult is the Go reference interpreter's typed result: kind 'I'/'J' carry a
// primitive in i64 (int sign-extended), kind 'L' carries an object in obj.
type vmResult struct {
	kind byte
	i64  int64
	obj  any
}

// vmExec runs an int-returning method (mirrors the injected smali VM.run,
// narrowed to int).
func vmExec(code []byte, args []int64, wire []byte) int32 { return int32(vmRun(code, args, wire)) }

// vmRun is a convenience wrapper for primitive-returning methods (no objects).
func vmRun(code []byte, args []int64, wire []byte) int64 {
	return vmRunG(code, args, nil, wire).i64
}

// vmRunG is the Go reference interpreter. Primitive args are int64 (one slot per
// primitive param); object args are opaque values (one slot per object param).
// Wide values live in a parallel long register file rw and objects in ro, so the
// existing int handlers (using r) are unchanged. RET (int) sign-extends, RET_WIDE
// returns the full long, RET_OBJ returns the object.
func vmRunG(code []byte, args []int64, objArgs []any, wire []byte) vmResult {
	inv := make([]int, opCount)
	for logical, w := range wire {
		inv[w] = logical
	}
	numRegs := int(code[0])
	r := make([]int32, numRegs)
	rw := make([]int64, numRegs)
	ro := make([]any, numRegs)
	pc := 1
	rd := func() byte { b := code[pc]; pc++; return b }
	imm := func() int32 { v := int32(binary.BigEndian.Uint32(code[pc:])); pc += 4; return v }
	imm64 := func() int64 { v := int64(binary.BigEndian.Uint64(code[pc:])); pc += 8; return v }
	rd16 := func() int { t := int(code[pc])<<8 | int(code[pc+1]); pc += 2; return t }
	for pc < len(code) {
		switch inv[code[pc]] {
		case opLoadArg:
			pc++
			d, ai := rd(), rd()
			r[d] = int32(args[ai])
		case opLoadArgWide:
			pc++
			d, ai := rd(), rd()
			rw[d] = args[ai]
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
		case opDiv:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = jdiv(r[a], r[b])
		case opRem:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = jrem(r[a], r[b])
		case opShl:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = r[a] << (uint(r[b]) & 31)
		case opShr:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = r[a] >> (uint(r[b]) & 31)
		case opUshr:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = int32(uint32(r[a]) >> (uint(r[b]) & 31))
		case opNeg:
			pc++
			d, s := rd(), rd()
			r[d] = -r[s]
		case opNot:
			pc++
			d, s := rd(), rd()
			r[d] = ^r[s]
		case opAndLit:
			pc++
			d, s := rd(), rd()
			r[d] = r[s] & imm()
		case opOrLit:
			pc++
			d, s := rd(), rd()
			r[d] = r[s] | imm()
		case opXorLit:
			pc++
			d, s := rd(), rd()
			r[d] = r[s] ^ imm()
		case opShlLit:
			pc++
			d, s := rd(), rd()
			r[d] = r[s] << (uint(imm()) & 31)
		case opShrLit:
			pc++
			d, s := rd(), rd()
			r[d] = r[s] >> (uint(imm()) & 31)
		case opUshrLit:
			pc++
			d, s := rd(), rd()
			r[d] = int32(uint32(r[s]) >> (uint(imm()) & 31))
		case opDivLit:
			pc++
			d, s := rd(), rd()
			r[d] = jdiv(r[s], imm())
		case opRemLit:
			pc++
			d, s := rd(), rd()
			r[d] = jrem(r[s], imm())
		case opRsubLit:
			pc++
			d, s := rd(), rd()
			r[d] = imm() - r[s]
		case opI2B:
			pc++
			d, s := rd(), rd()
			r[d] = int32(int8(r[s]))
		case opI2S:
			pc++
			d, s := rd(), rd()
			r[d] = int32(int16(r[s]))
		case opI2C:
			pc++
			d, s := rd(), rd()
			r[d] = int32(uint16(r[s]))
		case opConstWide:
			pc++
			d := rd()
			rw[d] = imm64()
		case opMoveWide:
			pc++
			d, s := rd(), rd()
			rw[d] = rw[s]
		case opAddL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = rw[a] + rw[b]
		case opSubL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = rw[a] - rw[b]
		case opMulL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = rw[a] * rw[b]
		case opDivL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = jdivL(rw[a], rw[b])
		case opRemL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = jremL(rw[a], rw[b])
		case opAndL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = rw[a] & rw[b]
		case opOrL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = rw[a] | rw[b]
		case opXorL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = rw[a] ^ rw[b]
		case opShlL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = rw[a] << (uint(r[b]) & 63)
		case opShrL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = rw[a] >> (uint(r[b]) & 63)
		case opUshrL:
			pc++
			d, a, b := rd(), rd(), rd()
			rw[d] = int64(uint64(rw[a]) >> (uint(r[b]) & 63))
		case opNegL:
			pc++
			d, s := rd(), rd()
			rw[d] = -rw[s]
		case opNotL:
			pc++
			d, s := rd(), rd()
			rw[d] = ^rw[s]
		case opI2L:
			pc++
			d, s := rd(), rd()
			rw[d] = int64(r[s])
		case opL2I:
			pc++
			d, s := rd(), rd()
			r[d] = int32(rw[s])
		case opCmpLong:
			pc++
			d, a, b := rd(), rd(), rd()
			r[d] = cmp64(rw[a], rw[b])
		case opRetWide:
			pc++
			s := rd()
			return vmResult{kind: 'J', i64: rw[s]}
		case opRet:
			pc++
			s := rd()
			return vmResult{kind: 'I', i64: int64(r[s])}
		case opLoadArgObj:
			pc++
			d, ai := rd(), rd()
			ro[d] = objArgs[ai]
		case opMoveObj:
			pc++
			d, s := rd(), rd()
			ro[d] = ro[s]
		case opRetObj:
			pc++
			s := rd()
			return vmResult{kind: 'L', obj: ro[s]}
		case opGoto:
			pc++
			pc = rd16()
		case opIfEq:
			pc++
			a, b, t := rd(), rd(), rd16()
			if r[a] == r[b] {
				pc = t
			}
		case opIfNe:
			pc++
			a, b, t := rd(), rd(), rd16()
			if r[a] != r[b] {
				pc = t
			}
		case opIfLt:
			pc++
			a, b, t := rd(), rd(), rd16()
			if r[a] < r[b] {
				pc = t
			}
		case opIfGe:
			pc++
			a, b, t := rd(), rd(), rd16()
			if r[a] >= r[b] {
				pc = t
			}
		case opIfGt:
			pc++
			a, b, t := rd(), rd(), rd16()
			if r[a] > r[b] {
				pc = t
			}
		case opIfLe:
			pc++
			a, b, t := rd(), rd(), rd16()
			if r[a] <= r[b] {
				pc = t
			}
		case opIfEqz:
			pc++
			a, t := rd(), rd16()
			if r[a] == 0 {
				pc = t
			}
		case opIfNez:
			pc++
			a, t := rd(), rd16()
			if r[a] != 0 {
				pc = t
			}
		case opIfLtz:
			pc++
			a, t := rd(), rd16()
			if r[a] < 0 {
				pc = t
			}
		case opIfGez:
			pc++
			a, t := rd(), rd16()
			if r[a] >= 0 {
				pc = t
			}
		case opIfGtz:
			pc++
			a, t := rd(), rd16()
			if r[a] > 0 {
				pc = t
			}
		case opIfLez:
			pc++
			a, t := rd(), rd16()
			if r[a] <= 0 {
				pc = t
			}
		default:
			return vmResult{kind: 'I', i64: -1}
		}
	}
	return vmResult{kind: 'I', i64: -1}
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

// paramInfo describes one method parameter for the register/argument layout.
// kind is 'I' (int), 'J' (long) or 'L' (reference). regOff is the register
// offset from paramBase in the smali register file (int/ref = 1 slot, long = 2).
// argIdx is the slot index into the primitive arg array ([J, for I/J) or the
// object arg array ([Ljava/lang/Object;, for L) — the two are counted apart.
type paramInfo struct {
	kind   byte
	regOff int
	argIdx int
}

// tokenizeParams splits a bare parameter descriptor string into individual type
// descriptors, e.g. "ILjava/lang/String;[I" -> ["I","Ljava/lang/String;","[I"].
func tokenizeParams(params string) ([]string, bool) {
	var out []string
	for i := 0; i < len(params); {
		start := i
		for i < len(params) && params[i] == '[' { // array dimensions
			i++
		}
		if i >= len(params) {
			return nil, false
		}
		switch params[i] {
		case 'L':
			j := strings.IndexByte(params[i:], ';')
			if j < 0 {
				return nil, false
			}
			i += j + 1
		case 'I', 'J', 'Z', 'B', 'S', 'C', 'F', 'D':
			i++
		default:
			return nil, false
		}
		out = append(out, params[start:i])
	}
	return out, true
}

// parseParams classifies parameters into the wide/object-aware layout. Supports
// int (I), long (J) and reference types (L...;, arrays), which cover the common
// object-plumbing case; bails on the other primitives (Z/B/S/C/F/D) the VM does
// not yet model. Returns per-param info and the total register width.
func parseParams(params string) ([]paramInfo, int, bool) {
	toks, ok := tokenizeParams(params)
	if !ok {
		return nil, 0, false
	}
	var out []paramInfo
	regOff, primIdx, objIdx := 0, 0, 0
	for _, d := range toks {
		switch {
		case d == "I":
			out = append(out, paramInfo{'I', regOff, primIdx})
			regOff++
			primIdx++
		case d == "J":
			out = append(out, paramInfo{'J', regOff, primIdx})
			regOff += 2
			primIdx++
		case d[0] == 'L' || d[0] == '[':
			out = append(out, paramInfo{'L', regOff, objIdx})
			regOff++
			objIdx++
		default:
			return nil, 0, false
		}
	}
	return out, regOff, true
}

// returnKind classifies a return-type descriptor as int ('I'), long ('J') or
// reference ('L'). Other types are not virtualizable.
func returnKind(ret string) (byte, bool) {
	switch {
	case ret == "I":
		return 'I', true
	case ret == "J":
		return 'J', true
	case len(ret) > 0 && (ret[0] == 'L' || ret[0] == '['):
		return 'L', true
	}
	return 0, false
}

// primObjCounts returns how many primitive (I/J) and object (L) params exist.
func primObjCounts(pinfos []paramInfo) (prim, obj int) {
	for _, pi := range pinfos {
		if pi.kind == 'L' {
			obj++
		} else {
			prim++
		}
	}
	return
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
	if start >= end { // malformed input: no valid body range
		return nil
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
			m := vmMethodDeclRE.FindStringSubmatch(bk[0])
			pinfos, _, _ := parseParams(m[3])
			count++
			return virtualizedBody(bk[0], pinfos, code)
		})
	}
	if count == 0 {
		return 0, nil
	}
	return count, VMClass(base, wire)
}
