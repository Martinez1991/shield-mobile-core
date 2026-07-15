package engine

import (
	"fmt"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// virtualizedBody replaces a compiled method's body with a call to the VM. It
// marshals the args into two arrays — a long[] of primitives (int widened, long
// direct) and an Object[] of references — builds a String[] of the method's
// const-string literals, embeds the bytecode as a byte[] payload, and invokes
// Lshield/rt/VM;->run, which returns an Object (primitives boxed as Long). The
// result is unboxed to match the declared return type. Because virtualization
// runs before string encryption, the pool literals emitted here are themselves
// encrypted by the string pass afterwards (defense in depth).
//
// Register map (temps v0..v6, then the params): v0=[J prims, v1=[Object refs,
// v2=[String pool, v3=[B bytecode, v4=index, v5:v6=wide box / string temp.
func virtualizedBody(decl string, pinfos []paramInfo, code []byte, pool []string) []string {
	m := vmMethodDeclRE.FindStringSubmatch(decl)
	ret := m[4]
	retKind, _ := returnKind(ret)
	regWidth := 0
	for _, pi := range pinfos {
		if pi.kind == 'J' {
			regWidth = pi.regOff + 2
		} else {
			regWidth = pi.regOff + 1
		}
	}
	nPrim, nObj := primObjCounts(pinfos)

	var b []string
	b = append(b, decl)
	b = append(b, fmt.Sprintf("    .registers %d", 7+regWidth))
	b = append(b, fmt.Sprintf("    const/16 v0, 0x%x", nPrim))
	b = append(b, "    new-array v0, v0, [J")
	b = append(b, fmt.Sprintf("    const/16 v1, 0x%x", nObj))
	b = append(b, "    new-array v1, v1, [Ljava/lang/Object;")
	for _, pi := range pinfos {
		b = append(b, fmt.Sprintf("    const/16 v4, 0x%x", pi.argIdx))
		switch pi.kind {
		case 'J':
			b = append(b, fmt.Sprintf("    aput-wide p%d, v0, v4", pi.regOff))
		case 'L':
			b = append(b, fmt.Sprintf("    aput-object p%d, v1, v4", pi.regOff))
		default: // int: widen into the long[] slot
			b = append(b, fmt.Sprintf("    int-to-long v5, p%d", pi.regOff))
			b = append(b, "    aput-wide v5, v0, v4")
		}
	}
	// v2 = String[] constant pool.
	b = append(b, fmt.Sprintf("    const/16 v2, 0x%x", len(pool)))
	b = append(b, "    new-array v2, v2, [Ljava/lang/String;")
	for i, lit := range pool {
		b = append(b, fmt.Sprintf("    const/16 v4, 0x%x", i))
		b = append(b, fmt.Sprintf("    const-string v5, \"%s\"", lit))
		b = append(b, "    aput-object v5, v2, v4")
	}
	b = append(b, fmt.Sprintf("    const/16 v3, 0x%x", len(code)))
	b = append(b, "    new-array v3, v3, [B")
	b = append(b, "    fill-array-data v3, :vmbc")
	b = append(b, "    invoke-static {v3, v0, v1, v2}, Lshield/rt/VM;->run([B[J[Ljava/lang/Object;[Ljava/lang/String;)Ljava/lang/Object;")
	b = append(b, "    move-result-object v0")
	switch retKind {
	case 'L':
		b = append(b, fmt.Sprintf("    check-cast v0, %s", ret))
		b = append(b, "    return-object v0")
	case 'J':
		b = append(b, "    check-cast v0, Ljava/lang/Long;")
		b = append(b, "    invoke-virtual {v0}, Ljava/lang/Long;->longValue()J")
		b = append(b, "    move-result-wide v0")
		b = append(b, "    return-wide v0")
	default: // int
		b = append(b, "    check-cast v0, Ljava/lang/Long;")
		b = append(b, "    invoke-virtual {v0}, Ljava/lang/Long;->longValue()J")
		b = append(b, "    move-result-wide v0")
		b = append(b, "    long-to-int v0, v0")
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
	// run: the fetch/decode/dispatch loop. p1 is the long[] of primitive args,
	// p2 the Object[] of reference args. Returns Object: RET/RET_WIDE box the
	// primitive as Long, RET_OBJ returns the reference. r=int registers (v2),
	// v10=rw long registers, v18=ro object registers.
	p(".method public static run([B[J[Ljava/lang/Object;[Ljava/lang/String;)Ljava/lang/Object;")
	p("    .locals 20")
	// params p0/p1/p2 sit at v20/v21/v22 (locals+params), out of reach of
	// aget/invoke (formats 23x/35c cap at v15). Relocate the bytecode to the free
	// low register v11 via move-object/16 (format 32x reaches high regs); the arg
	// arrays are copied on demand inside the LOADARG handlers. The object register
	// file lives in the high local v18, likewise reached via move-object/16.
	p("    move-object/16 v11, p0")
	p("    const/4 v0, 0x0")
	p("    aget-byte v1, v11, v0")
	p("    and-int/lit16 v1, v1, 0xff")
	p("    new-array v2, v1, [I")
	p("    new-array v10, v1, [J")
	p("    new-array v9, v1, [Ljava/lang/Object;")
	p("    move-object/16 v18, v9")
	// v19 holds the pending invoke result; seed it with a valid ref so the
	// verifier sees it definitely-assigned (move-result only runs after an
	// invoke, which overwrites it).
	p("    move-object/16 v19, v9")
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

	// LOADARG_OBJ dest, argIdx (object): copy from the Object[] args (p2) into ro
	{
		lo := label()
		p("    const/16 v5, 0x%x", w(opLoadArgObj))
		p("    if-ne v4, v5, %s", lo)
		readByte("v6")
		readByte("v7")
		p("    move-object/16 v8, p2")
		p("    aget-object v9, v8, v7")
		p("    move-object/16 v8, v18")
		p("    aput-object v9, v8, v6")
		p("    goto :loop")
		p("    %s", lo)
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

	// MOVE_OBJ dest, src (move-object on the ro object register file)
	{
		lm := label()
		p("    const/16 v5, 0x%x", w(opMoveObj))
		p("    if-ne v4, v5, %s", lm)
		readByte("v6")
		readByte("v7")
		p("    move-object/16 v8, v18")
		p("    aget-object v9, v8, v7")
		p("    aput-object v9, v8, v6")
		p("    goto :loop")
		p("    %s", lm)
	}

	// CONST_STR dest, poolIdx: load a pooled string literal (p3) into ro
	{
		lc := label()
		p("    const/16 v5, 0x%x", w(opConstStr))
		p("    if-ne v4, v5, %s", lc)
		readByte("v6")
		readByte("v7")
		p("    move-object/16 v8, p3")
		p("    aget-object v9, v8, v7")
		p("    move-object/16 v8, v18")
		p("    aput-object v9, v8, v6")
		p("    goto :loop")
		p("    %s", lc)
	}

	// INVOKE_STATIC ownerIdx, nameIdx, argCount, argReg...: resolve an external
	// static method by reflection (owner/name from the string pool p3, all-int
	// param types) and call it; the boxed result is held in v19 for MOVE_RESULT.
	{
		li := label()
		p("    const/16 v5, 0x%x", w(opInvokeStatic))
		p("    if-ne v4, v5, %s", li)
		readByte("v6") // ownerIdx
		readByte("v7") // nameIdx
		readByte("v9") // descIdx (Go model only; interpreter ignores)
		// receiver register (0xff = static/null); resolve the object into v16.
		readByte("v12")
		nullRecv, recvDone := label(), label()
		p("    const/16 v13, 0xff")
		p("    if-eq v12, v13, %s", nullRecv)
		p("    move-object/16 v13, v18") // ro
		p("    aget-object v13, v13, v12")
		p("    move-object/16 v16, v13") // receiver (survives)
		p("    goto %s", recvDone)
		p("    %s", nullRecv)
		p("    const/4 v13, 0x0")
		p("    move-object/16 v16, v13") // null receiver
		p("    %s", recvDone)
		readByte("v8") // argCount
		p("    move-object/16 v13, p3")
		p("    aget-object v0, v13, v6")
		p("    invoke-static {v0}, Ljava/lang/Class;->forName(Ljava/lang/String;)Ljava/lang/Class;")
		p("    move-result-object v0")                  // owner Class (survives)
		p("    aget-object v15, v13, v7")               // method name String (survives)
		p("    new-array v1, v8, [Ljava/lang/Class;")   // paramTypes (survives)
		p("    new-array v14, v8, [Ljava/lang/Object;") // args (survives)
		p("    const/4 v5, 0x0")

		loop, done := label(), label()
		notInt, notLong, next := label(), label(), label()
		p("    %s", loop)
		p("    if-ge v5, v8, %s", done)
		readByte("v6") // kind
		readByte("v7") // reg
		readByte("v9") // typeIdx
		// int arg
		p("    const/4 v13, 0x%x", invKindInt)
		p("    if-ne v6, v13, %s", notInt)
		p("    sget-object v12, Ljava/lang/Integer;->TYPE:Ljava/lang/Class;")
		p("    aput-object v12, v1, v5")
		p("    aget v13, v2, v7")
		p("    invoke-static {v13}, Ljava/lang/Integer;->valueOf(I)Ljava/lang/Integer;")
		p("    move-result-object v12")
		p("    aput-object v12, v14, v5")
		p("    goto %s", next)
		p("    %s", notInt)
		// long arg
		p("    const/4 v13, 0x%x", invKindLong)
		p("    if-ne v6, v13, %s", notLong)
		p("    sget-object v12, Ljava/lang/Long;->TYPE:Ljava/lang/Class;")
		p("    aput-object v12, v1, v5")
		p("    aget-wide v12, v10, v7")
		p("    invoke-static {v12, v13}, Ljava/lang/Long;->valueOf(J)Ljava/lang/Long;")
		p("    move-result-object v12")
		p("    aput-object v12, v14, v5")
		p("    goto %s", next)
		p("    %s", notLong)
		// object arg: paramType = Class.forName(pool[typeIdx]); value = ro[reg]
		p("    move-object/16 v13, p3")
		p("    aget-object v12, v13, v9")
		p("    invoke-static {v12}, Ljava/lang/Class;->forName(Ljava/lang/String;)Ljava/lang/Class;")
		p("    move-result-object v12")
		p("    aput-object v12, v1, v5")
		p("    move-object/16 v13, v18")
		p("    aget-object v12, v13, v7")
		p("    aput-object v12, v14, v5")
		p("    %s", next)
		p("    add-int/lit8 v5, v5, 0x1")
		p("    goto %s", loop)
		p("    %s", done)
		// resolve and invoke
		p("    invoke-virtual {v0, v15, v1}, Ljava/lang/Class;->getMethod(Ljava/lang/String;[Ljava/lang/Class;)Ljava/lang/reflect/Method;")
		p("    move-result-object v0")
		p("    move-object/16 v13, v16") // receiver (null for static)
		p("    invoke-virtual {v0, v13, v14}, Ljava/lang/reflect/Method;->invoke(Ljava/lang/Object;[Ljava/lang/Object;)Ljava/lang/Object;")
		p("    move-result-object v19") // pending result (boxed)
		p("    goto :loop")
		p("    %s", li)
	}

	// MOVE_RESULT dest: unbox the pending Integer into an int register.
	{
		lm := label()
		p("    const/16 v5, 0x%x", w(opMoveResult))
		p("    if-ne v4, v5, %s", lm)
		readByte("v6")
		p("    move-object/16 v0, v19")
		p("    check-cast v0, Ljava/lang/Integer;")
		p("    invoke-virtual {v0}, Ljava/lang/Integer;->intValue()I")
		p("    move-result v7")
		p("    aput v7, v2, v6")
		p("    goto :loop")
		p("    %s", lm)
	}

	// MOVE_RESULT_WIDE dest: unbox the pending Long into a wide register.
	{
		lw := label()
		p("    const/16 v5, 0x%x", w(opMoveResultWide))
		p("    if-ne v4, v5, %s", lw)
		readByte("v6")
		p("    move-object/16 v0, v19")
		p("    check-cast v0, Ljava/lang/Long;")
		p("    invoke-virtual {v0}, Ljava/lang/Long;->longValue()J")
		p("    move-result-wide v12")
		p("    aput-wide v12, v10, v6")
		p("    goto :loop")
		p("    %s", lw)
	}

	// MOVE_RESULT_OBJ dest: store the pending object into an object register.
	{
		lo := label()
		p("    const/16 v5, 0x%x", w(opMoveResultObj))
		p("    if-ne v4, v5, %s", lo)
		readByte("v6")
		p("    move-object/16 v13, v18")
		p("    move-object/16 v0, v19")
		p("    aput-object v0, v13, v6")
		p("    goto :loop")
		p("    %s", lo)
	}

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

	// RET_WIDE src: box the long as Long and return it (run returns Object).
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opRetWide))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		p("    aget-wide v12, v10, v6")
		p("    invoke-static {v12, v13}, Ljava/lang/Long;->valueOf(J)Ljava/lang/Long;")
		p("    move-result-object v0")
		p("    return-object v0")
		p("    %s", l)
	}

	// RET_OBJ src: return the object register directly.
	{
		l := label()
		p("    const/16 v5, 0x%x", w(opRetObj))
		p("    if-ne v4, v5, %s", l)
		readByte("v6")
		p("    move-object/16 v8, v18")
		p("    aget-object v0, v8, v6")
		p("    return-object v0")
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

	// RET src (int): sign-extend to long, box as Long, return (run returns Object)
	l = label()
	p("    const/16 v5, 0x%x", w(opRet))
	p("    if-ne v4, v5, %s", l)
	readByte("v6")
	p("    aget v0, v2, v6")
	p("    int-to-long v12, v0")
	p("    invoke-static {v12, v13}, Ljava/lang/Long;->valueOf(J)Ljava/lang/Long;")
	p("    move-result-object v0")
	p("    return-object v0")
	p("    %s", l)

	// unknown opcode -> null
	p("    const/4 v0, 0x0")
	p("    return-object v0")
	p(".end method")

	return &smali.Class{
		Base:       base,
		Descriptor: vmDescriptor,
		Lines:      strings.Split(strings.TrimRight(s.String(), "\n"), "\n"),
	}
}
