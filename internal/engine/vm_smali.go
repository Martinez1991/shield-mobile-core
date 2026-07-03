package engine

import (
	"fmt"
	"strings"

	"shield/internal/smali"
)

// virtualizedBody replaces a compiled method's body with a call to the VM: it
// packs the int args into an int[], embeds the bytecode as a byte[] payload and
// invokes Lshield/rt/VM;->run.
func virtualizedBody(decl string, nargs int, code []byte) []string {
	var b []string
	b = append(b, decl)
	b = append(b, fmt.Sprintf("    .registers %d", nargs+3))
	b = append(b, fmt.Sprintf("    const/16 v0, 0x%x", nargs))
	b = append(b, "    new-array v0, v0, [I")
	for i := 0; i < nargs; i++ {
		b = append(b, fmt.Sprintf("    const/16 v1, 0x%x", i))
		b = append(b, fmt.Sprintf("    aput p%d, v0, v1", i))
	}
	b = append(b, fmt.Sprintf("    const/16 v1, 0x%x", len(code)))
	b = append(b, "    new-array v1, v1, [B")
	b = append(b, "    fill-array-data v1, :vmbc")
	b = append(b, "    invoke-static {v1, v0}, Lshield/rt/VM;->run([B[I)I")
	b = append(b, "    move-result v0")
	b = append(b, "    return v0")
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
	// run: the fetch/decode/dispatch loop.
	p(".method public static run([B[I)I")
	p("    .locals 10")
	p("    const/4 v0, 0x0")
	p("    aget-byte v1, p0, v0")
	p("    and-int/lit16 v1, v1, 0xff")
	p("    new-array v2, v1, [I")
	p("    const/4 v3, 0x1")
	p("    :loop")
	p("    aget-byte v4, p0, v3")
	p("    and-int/lit16 v4, v4, 0xff")
	p("    add-int/lit8 v3, v3, 0x1")

	// helper emitters
	readByte := func(dest string) {
		p("    aget-byte %s, p0, v3", dest)
		p("    and-int/lit16 %s, %s, 0xff", dest, dest)
		p("    add-int/lit8 v3, v3, 0x1")
	}

	next := 0
	label := func() string { next++; return fmt.Sprintf(":h%d", next) }

	// LOADARG dest, argIdx
	l := label()
	p("    const/16 v5, 0x%x", w(opLoadArg))
	p("    if-ne v4, v5, %s", l)
	readByte("v6")
	readByte("v7")
	p("    aget v8, p1, v7")
	p("    aput v8, v2, v6")
	p("    goto :loop")
	p("    %s", l)

	// CONST dest, imm32
	l = label()
	p("    const/16 v5, 0x%x", w(opConst))
	p("    if-ne v4, v5, %s", l)
	readByte("v6")
	p("    invoke-static {p0, v3}, Lshield/rt/VM;->i4([BI)I")
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

	// lit ops: dest, src, imm32
	litop := func(op int, instr string) {
		l := label()
		p("    const/16 v5, 0x%x", w(op))
		p("    if-ne v4, v5, %s", l)
		readByte("v6") // dest
		readByte("v7") // src
		p("    invoke-static {p0, v3}, Lshield/rt/VM;->i4([BI)I")
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

	// GOTO target: pc <- target (opcode already consumed; v3 points at target)
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opGoto))
		p("    if-ne v4, v5, %s", l)
		p("    invoke-static {p0, v3}, Lshield/rt/VM;->i2([BI)I")
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
		p("    invoke-static {p0, v3}, Lshield/rt/VM;->i2([BI)I")
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
		p("    invoke-static {p0, v3}, Lshield/rt/VM;->i2([BI)I")
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

	// RET src
	l = label()
	p("    const/16 v5, 0x%x", w(opRet))
	p("    if-ne v4, v5, %s", l)
	readByte("v6")
	p("    aget v0, v2, v6")
	p("    return v0")
	p("    %s", l)

	// unknown opcode
	p("    const/4 v0, -0x1")
	p("    return v0")
	p(".end method")

	return &smali.Class{
		Base:       base,
		Descriptor: vmDescriptor,
		Lines:      strings.Split(strings.TrimRight(s.String(), "\n"), "\n"),
	}
}
