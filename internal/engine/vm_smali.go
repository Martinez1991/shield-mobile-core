package engine

import (
	"fmt"
	"strings"

	"shield/internal/smali"
)

// virtualizedBody replaces a compiled method's body with a call to the VM: it
// packs the args into a long[] (one slot per param; int params widened, long
// params stored directly), embeds the bytecode as a byte[] payload and invokes
// Lshield/rt/VM;->run (which returns a long). wideRet marks a long-returning
// method (return-wide); otherwise the long is narrowed to int.
func virtualizedBody(decl string, pinfos []paramInfo, code []byte, wideRet bool) []string {
	regWidth := 0
	for _, pi := range pinfos {
		if pi.isLong {
			regWidth = pi.regOff + 2
		} else {
			regWidth = pi.regOff + 1
		}
	}
	var b []string
	b = append(b, decl)
	// locals: v0 args, v1 index/bc, v2:v3 wide temp + run result.
	b = append(b, fmt.Sprintf("    .registers %d", 4+regWidth))
	b = append(b, fmt.Sprintf("    const/16 v0, 0x%x", len(pinfos)))
	b = append(b, "    new-array v0, v0, [J")
	for i, pi := range pinfos {
		b = append(b, fmt.Sprintf("    const/16 v1, 0x%x", i))
		if pi.isLong {
			b = append(b, fmt.Sprintf("    aput-wide p%d, v0, v1", pi.regOff))
		} else {
			b = append(b, fmt.Sprintf("    int-to-long v2, p%d", pi.regOff))
			b = append(b, "    aput-wide v2, v0, v1")
		}
	}
	b = append(b, fmt.Sprintf("    const/16 v1, 0x%x", len(code)))
	b = append(b, "    new-array v1, v1, [B")
	b = append(b, "    fill-array-data v1, :vmbc")
	b = append(b, "    invoke-static {v1, v0}, Lshield/rt/VM;->run([B[J)J")
	b = append(b, "    move-result-wide v2")
	if wideRet {
		b = append(b, "    return-wide v2")
	} else {
		b = append(b, "    long-to-int v0, v2")
		b = append(b, "    return v0")
	}
	b = append(b, "    :vmbc")
	b = append(b, "    .array-data 1")
	for _, by := range code {
		b = append(b, fmt.Sprintf("        0x%02xt", by))
	}
	b = append(b, "    .end array-data")
	b = append(b, ".end method")
	return b
}

// VMClass builds the injected interpreter Lshield/rt/VM;. The wire opcode values
// are baked in from the per-build permutation (section 8.1 polymorphism), so the
// dispatch constants differ every build. Validated to assemble and to interop
// with the JVM (see the Java harness in scripts/).
func VMClass(base string, wire []byte) *smali.Class {
	w := func(op int) int { return int(wire[op]) }
	var s strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&s, format+"\n", a...) }

	p(".class public Lshield/rt/VM;")
	p(".super Ljava/lang/Object;")
	p("")
	p("# SHIELD code-virtualization interpreter (generated). Polymorphic opcodes.")
	p("")
	// i4: read a big-endian signed int32 from bc at offset.
	p(".method public static i4([BI)I")
	p("    .locals 3")
	p("    aget-byte v0, p0, p1")
	p("    shl-int/lit8 v0, v0, 0x18")
	p("    add-int/lit8 v1, p1, 0x1")
	p("    aget-byte v2, p0, v1")
	p("    and-int/lit16 v2, v2, 0xff")
	p("    shl-int/lit8 v2, v2, 0x10")
	p("    or-int/2addr v0, v2")
	p("    add-int/lit8 v1, p1, 0x2")
	p("    aget-byte v2, p0, v1")
	p("    and-int/lit16 v2, v2, 0xff")
	p("    shl-int/lit8 v2, v2, 0x8")
	p("    or-int/2addr v0, v2")
	p("    add-int/lit8 v1, p1, 0x3")
	p("    aget-byte v2, p0, v1")
	p("    and-int/lit16 v2, v2, 0xff")
	p("    or-int/2addr v0, v2")
	p("    return v0")
	p(".end method")
	p("")
	// i2: read a big-endian unsigned int16 (jump target) from bc at offset.
	p(".method public static i2([BI)I")
	p("    .locals 3")
	p("    aget-byte v0, p0, p1")
	p("    and-int/lit16 v0, v0, 0xff")
	p("    shl-int/lit8 v0, v0, 0x8")
	p("    add-int/lit8 v1, p1, 0x1")
	p("    aget-byte v2, p0, v1")
	p("    and-int/lit16 v2, v2, 0xff")
	p("    or-int/2addr v0, v2")
	p("    return v0")
	p(".end method")
	p("")
	// i8: read a big-endian signed int64 (wide constant) from bc at offset.
	p(".method public static i8([BI)J")
	p("    .locals 8")
	p("    invoke-static {p0, p1}, Lshield/rt/VM;->i4([BI)I")
	p("    move-result v0")
	p("    int-to-long v0, v0")
	p("    const/16 v2, 0x20")
	p("    shl-long v0, v0, v2")
	p("    add-int/lit8 v3, p1, 0x4")
	p("    invoke-static {p0, v3}, Lshield/rt/VM;->i4([BI)I")
	p("    move-result v3")
	p("    int-to-long v4, v3")
	p("    const/16 v3, 0x20")
	p("    shl-long v4, v4, v3")
	p("    ushr-long v4, v4, v3")
	p("    or-long v0, v0, v4")
	p("    return-wide v0")
	p(".end method")
	p("")
	// run: the fetch/decode/dispatch loop. args (p1) is a long[] (one slot per
	// param). Returns long: RET (int) sign-extends, RET_WIDE returns the full
	// long. r=int registers, v10=rw long registers.
	p(".method public static run([B[J)J")
	p("    .locals 18")
	// p0 ([B bytecode) and p1 ([J args) sit at v18/v19 (locals+params), out of
	// reach of aget/invoke (formats 23x/35c cap at v15). Relocate the bytecode to
	// the free low register v11 via move-object/16 (format 32x reaches v18); the
	// args array is copied on demand inside the two LOADARG handlers.
	p("    move-object/16 v11, p0")
	p("    const/4 v0, 0x0")
	p("    aget-byte v1, v11, v0")
	p("    and-int/lit16 v1, v1, 0xff")
	p("    new-array v2, v1, [I")
	p("    new-array v10, v1, [J")
	p("    const/4 v3, 0x1")
	p("    :loop")
	p("    aget-byte v4, v11, v3")
	p("    and-int/lit16 v4, v4, 0xff")
	p("    add-int/lit8 v3, v3, 0x1")

	// helper emitters
	readByte := func(dest string) {
		p("    aget-byte %s, v11, v3", dest)
		p("    and-int/lit16 %s, %s, 0xff", dest, dest)
		p("    add-int/lit8 v3, v3, 0x1")
	}

	next := 0
	label := func() string { next++; return fmt.Sprintf(":h%d", next) }

	// LOADARG dest, argIdx (int): read the long slot and narrow to int
	l := label()
	p("    const/16 v5, 0x%x", w(opLoadArg))
	p("    if-ne v4, v5, %s", l)
	readByte("v6")
	readByte("v7")
	p("    move-object/16 v9, p1")
	p("    aget-wide v12, v9, v7")
	p("    long-to-int v8, v12")
	p("    aput v8, v2, v6")
	p("    goto :loop")
	p("    %s", l)

	// LOADARG_WIDE dest, argIdx (long): read the long slot into rw
	{
		lw := label()
		p("    const/16 v5, 0x%x", w(opLoadArgWide))
		p("    if-ne v4, v5, %s", lw)
		readByte("v6")
		readByte("v7")
		p("    move-object/16 v9, p1")
		p("    aget-wide v12, v9, v7")
		p("    aput-wide v12, v10, v6")
		p("    goto :loop")
		p("    %s", lw)
	}

	// CONST dest, imm32
	l = label()
	p("    const/16 v5, 0x%x", w(opConst))
	p("    if-ne v4, v5, %s", l)
	readByte("v6")
	p("    invoke-static {v11, v3}, Lshield/rt/VM;->i4([BI)I")
	p("    move-result v7")
	p("    add-int/lit8 v3, v3, 0x4")
	p("    aput v7, v2, v6")
	p("    goto :loop")
	p("    %s", l)

	// MOVE dest, src
	l = label()
	p("    const/16 v5, 0x%x", w(opMove))
	p("    if-ne v4, v5, %s", l)
	readByte("v6")
	readByte("v7")
	p("    aget v8, v2, v7")
	p("    aput v8, v2, v6")
	p("    goto :loop")
	p("    %s", l)

	// binary ops
	binop := func(op int, instr string) {
		l := label()
		p("    const/16 v5, 0x%x", w(op))
		p("    if-ne v4, v5, %s", l)
		readByte("v6") // dest
		readByte("v7") // a
		readByte("v8") // b
		p("    aget v9, v2, v7")
		p("    aget v5, v2, v8")
		p("    %s v9, v9, v5", instr)
		p("    aput v9, v2, v6")
		p("    goto :loop")
		p("    %s", l)
	}
	binop(opAdd, "add-int")
	binop(opSub, "sub-int")
	binop(opMul, "mul-int")
	binop(opAnd, "and-int")
	binop(opOr, "or-int")
	binop(opXor, "xor-int")
	binop(opDiv, "div-int")
	binop(opRem, "rem-int")
	binop(opShl, "shl-int")
	binop(opShr, "shr-int")
	binop(opUshr, "ushr-int")

	// unary ops: dest, src
	unop := func(op int, instr string) {
		l := label()
		p("    const/16 v5, 0x%x", w(op))
		p("    if-ne v4, v5, %s", l)
		readByte("v6") // dest
		readByte("v7") // src
		p("    aget v9, v2, v7")
		p("    %s v9, v9", instr)
		p("    aput v9, v2, v6")
		p("    goto :loop")
		p("    %s", l)
	}
	unop(opNeg, "neg-int")
	unop(opNot, "not-int")
	unop(opI2B, "int-to-byte")
	unop(opI2S, "int-to-short")
	unop(opI2C, "int-to-char")

	// --- 64-bit long handlers (rw = v10; wide temps v12:v13, v14:v15, v16:v17) ---
	wideBin := func(op int, instr string) {
		l := label()
		p("    const/16 v5, 0x%x", w(op))
		p("    if-ne v4, v5, %s", l)
		readByte("v6") // dest
		readByte("v7") // a
		readByte("v8") // b
		p("    aget-wide v12, v10, v7")
		p("    aget-wide v14, v10, v8")
		p("    %s v16, v12, v14", instr)
		p("    aput-wide v16, v10, v6")
		p("    goto :loop")
		p("    %s", l)
	}
	wideBin(opAddL, "add-long")
	wideBin(opSubL, "sub-long")
	wideBin(opMulL, "mul-long")
	wideBin(opDivL, "div-long")
	wideBin(opRemL, "rem-long")
	wideBin(opAndL, "and-long")
	wideBin(opOrL, "or-long")
	wideBin(opXorL, "xor-long")

	// long shifts: dest(wide), a(wide), b(int register)
	wideShift := func(op int, instr string) {
		l := label()
		p("    const/16 v5, 0x%x", w(op))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		readByte("v7")
		readByte("v8")
		p("    aget-wide v12, v10, v7")
		p("    aget v9, v2, v8")
		p("    %s v16, v12, v9", instr)
		p("    aput-wide v16, v10, v6")
		p("    goto :loop")
		p("    %s", l)
	}
	wideShift(opShlL, "shl-long")
	wideShift(opShrL, "shr-long")
	wideShift(opUshrL, "ushr-long")

	wideUn := func(op int, instr string) {
		l := label()
		p("    const/16 v5, 0x%x", w(op))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		readByte("v7")
		p("    aget-wide v12, v10, v7")
		p("    %s v14, v12", instr)
		p("    aput-wide v14, v10, v6")
		p("    goto :loop")
		p("    %s", l)
	}
	wideUn(opNegL, "neg-long")
	wideUn(opNotL, "not-long")

	// CONST_WIDE dest, imm64
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opConstWide))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		p("    invoke-static {v11, v3}, Lshield/rt/VM;->i8([BI)J")
		p("    move-result-wide v12")
		p("    add-int/lit8 v3, v3, 0x8")
		p("    aput-wide v12, v10, v6")
		p("    goto :loop")
		p("    %s", l)
	}

	// MOVE_WIDE dest, src
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opMoveWide))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		readByte("v7")
		p("    aget-wide v12, v10, v7")
		p("    aput-wide v12, v10, v6")
		p("    goto :loop")
		p("    %s", l)
	}

	// I2L destWide, srcInt
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opI2L))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		readByte("v7")
		p("    aget v9, v2, v7")
		p("    int-to-long v12, v9")
		p("    aput-wide v12, v10, v6")
		p("    goto :loop")
		p("    %s", l)
	}

	// L2I destInt, srcWide
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opL2I))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		readByte("v7")
		p("    aget-wide v12, v10, v7")
		p("    long-to-int v9, v12")
		p("    aput v9, v2, v6")
		p("    goto :loop")
		p("    %s", l)
	}

	// CMP_LONG destInt, a, b
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opCmpLong))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		readByte("v7")
		readByte("v8")
		p("    aget-wide v12, v10, v7")
		p("    aget-wide v14, v10, v8")
		p("    cmp-long v9, v12, v14")
		p("    aput v9, v2, v6")
		p("    goto :loop")
		p("    %s", l)
	}

	// RET_WIDE src
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opRetWide))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		p("    aget-wide v12, v10, v6")
		p("    return-wide v12")
		p("    %s", l)
	}

	// lit ops: dest, src, imm32
	litop := func(op int, instr string) {
		l := label()
		p("    const/16 v5, 0x%x", w(op))
		p("    if-ne v4, v5, %s", l)
		readByte("v6") // dest
		readByte("v7") // src
		p("    invoke-static {v11, v3}, Lshield/rt/VM;->i4([BI)I")
		p("    move-result v8")
		p("    add-int/lit8 v3, v3, 0x4")
		p("    aget v9, v2, v7")
		p("    %s v9, v9, v8", instr)
		p("    aput v9, v2, v6")
		p("    goto :loop")
		p("    %s", l)
	}
	litop(opAddLit, "add-int")
	litop(opMulLit, "mul-int")
	litop(opAndLit, "and-int")
	litop(opOrLit, "or-int")
	litop(opXorLit, "xor-int")
	litop(opShlLit, "shl-int")
	litop(opShrLit, "shr-int")
	litop(opUshrLit, "ushr-int")
	litop(opDivLit, "div-int")
	litop(opRemLit, "rem-int")

	// RSUB imm, src: dest = imm - src
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opRsubLit))
		p("    if-ne v4, v5, %s", l)
		readByte("v6") // dest
		readByte("v7") // src
		p("    invoke-static {v11, v3}, Lshield/rt/VM;->i4([BI)I")
		p("    move-result v8") // imm
		p("    add-int/lit8 v3, v3, 0x4")
		p("    aget v9, v2, v7")
		p("    sub-int v9, v8, v9")
		p("    aput v9, v2, v6")
		p("    goto :loop")
		p("    %s", l)
	}

	// GOTO target: pc <- target (opcode already consumed; v3 points at target)
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opGoto))
		p("    if-ne v4, v5, %s", l)
		p("    invoke-static {v11, v3}, Lshield/rt/VM;->i2([BI)I")
		p("    move-result v3")
		p("    goto :loop")
		p("    %s", l)
	}

	// IFCMP a, b, target: branch if r[a] <cmp> r[b]
	ifcmp := func(op int, instr string) {
		l := label()
		p("    const/16 v5, 0x%x", w(op))
		p("    if-ne v4, v5, %s", l)
		readByte("v6") // a
		readByte("v7") // b
		p("    invoke-static {v11, v3}, Lshield/rt/VM;->i2([BI)I")
		p("    move-result v8") // target
		p("    add-int/lit8 v3, v3, 0x2")
		p("    aget v9, v2, v6")
		p("    aget v5, v2, v7")
		br := label()
		p("    %s v9, v5, %s", instr, br)
		p("    goto :loop")
		p("    %s", br)
		p("    move v3, v8")
		p("    goto :loop")
		p("    %s", l)
	}
	ifcmp(opIfEq, "if-eq")
	ifcmp(opIfNe, "if-ne")
	ifcmp(opIfLt, "if-lt")
	ifcmp(opIfGe, "if-ge")
	ifcmp(opIfGt, "if-gt")
	ifcmp(opIfLe, "if-le")

	// IFZ a, target: branch if r[a] <cmp> 0
	ifz := func(op int, instr string) {
		l := label()
		p("    const/16 v5, 0x%x", w(op))
		p("    if-ne v4, v5, %s", l)
		readByte("v6") // a
		p("    invoke-static {v11, v3}, Lshield/rt/VM;->i2([BI)I")
		p("    move-result v8") // target
		p("    add-int/lit8 v3, v3, 0x2")
		p("    aget v9, v2, v6")
		br := label()
		p("    %s v9, %s", instr, br)
		p("    goto :loop")
		p("    %s", br)
		p("    move v3, v8")
		p("    goto :loop")
		p("    %s", l)
	}
	ifz(opIfEqz, "if-eqz")
	ifz(opIfNez, "if-nez")
	ifz(opIfLtz, "if-ltz")
	ifz(opIfGez, "if-gez")
	ifz(opIfGtz, "if-gtz")
	ifz(opIfLez, "if-lez")

	// RET src (int): sign-extend to long and return-wide (run returns J)
	l = label()
	p("    const/16 v5, 0x%x", w(opRet))
	p("    if-ne v4, v5, %s", l)
	readByte("v6")
	p("    aget v0, v2, v6")
	p("    int-to-long v12, v0")
	p("    return-wide v12")
	p("    %s", l)

	// unknown opcode
	p("    const-wide/16 v0, -0x1")
	p("    return-wide v0")
	p(".end method")

	return &smali.Class{
		Base:       base,
		Descriptor: vmDescriptor,
		Lines:      strings.Split(strings.TrimRight(s.String(), "\n"), "\n"),
	}
}
