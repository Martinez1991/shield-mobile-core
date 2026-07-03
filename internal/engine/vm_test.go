package engine

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

const vmSum2 = `.method public static sum2(II)I
    .registers 3
    add-int v0, p0, p1
    return v0
.end method`

const vmPoly = `.method public static poly(II)I
    .registers 4
    mul-int v0, p0, p0
    add-int v0, v0, p1
    add-int/lit8 v0, v0, 0x5
    return v0
.end method`

func compileStr(t *testing.T, src string, wire []byte) []byte {
	t.Helper()
	code, ok := compileMethod(strings.Split(src, "\n"), wire)
	if !ok {
		t.Fatalf("method not virtualizable:\n%s", src)
	}
	return code
}

func TestVMExecMatchesFormula(t *testing.T) {
	wire := vmPermutation(0x5117e1d)
	sum := compileStr(t, vmSum2, wire)
	poly := compileStr(t, vmPoly, wire)

	// deterministic pseudo-inputs (no rand: keep tests reproducible)
	for _, tc := range []struct{ a, b int32 }{{0, 0}, {1, 2}, {-3, 7}, {123, -456}, {1000, 1000}} {
		if got := vmExec(sum, []int64{int64(tc.a), int64(tc.b)}, wire); got != tc.a+tc.b {
			t.Errorf("sum2(%d,%d) = %d, want %d", tc.a, tc.b, got, tc.a+tc.b)
		}
		want := tc.a*tc.a + tc.b + 5
		if got := vmExec(poly, []int64{int64(tc.a), int64(tc.b)}, wire); got != want {
			t.Errorf("poly(%d,%d) = %d, want %d", tc.a, tc.b, got, want)
		}
	}
}

const vmSumLoop = `.method public static sum(I)I
    .registers 4
    const/4 v0, 0x0
    const/4 v1, 0x0
    :loop
    if-ge v1, p0, :done
    add-int/2addr v0, v1
    add-int/lit8 v1, v1, 0x1
    goto :loop
    :done
    return v0
.end method`

const vmClassify = `.method public static classify(I)I
    .registers 2
    const/16 v0, 0xa
    if-lt p0, v0, :small
    const/16 v0, 0x64
    return v0
    :small
    const/4 v0, 0x1
    return v0
.end method`

func TestVMBranches(t *testing.T) {
	wire := vmPermutation(0x5117e1d)
	sum := compileStr(t, vmSumLoop, wire)
	classify := compileStr(t, vmClassify, wire)

	sumRef := func(n int32) int32 {
		var s int32
		for i := int32(0); i < n; i++ {
			s += i
		}
		return s
	}
	classifyRef := func(n int32) int32 {
		if n < 10 {
			return 1
		}
		return 100
	}
	for _, n := range []int32{0, 1, 5, 10, 20, 50} {
		if got := vmExec(sum, []int64{int64(n)}, wire); got != sumRef(n) {
			t.Errorf("sum(%d) = %d, want %d", n, got, sumRef(n))
		}
		if got := vmExec(classify, []int64{int64(n)}, wire); got != classifyRef(n) {
			t.Errorf("classify(%d) = %d, want %d", n, got, classifyRef(n))
		}
	}
}

const vmBits = `.method public static bits(II)I
    .registers 6
    shl-int v0, p0, p1
    shr-int v1, p0, p1
    ushr-int v2, p0, p1
    add-int v0, v0, v1
    add-int v0, v0, v2
    div-int/lit8 v0, v0, 0x2
    rem-int/lit8 v3, v0, 0x5
    xor-int v0, v0, v3
    neg-int v0, v0
    not-int v0, v0
    rsub-int/lit8 v0, v0, 0x14
    and-int/lit8 v0, v0, 0xf
    return v0
.end method`

func TestVMExtendedIntOps(t *testing.T) {
	wire := vmPermutation(0x5117e1d)
	bits := compileStr(t, vmBits, wire)

	ref := func(a, b int32) int32 {
		sh := uint(b) & 31
		v0 := a << sh
		v0 += a >> sh
		v0 += int32(uint32(a) >> sh)
		v0 = jdiv(v0, 2)
		v0 ^= jrem(v0, 5)
		v0 = -v0
		v0 = ^v0
		v0 = 20 - v0
		return v0 & 0xf
	}
	for _, tc := range [][2]int32{{6, 2}, {13, 1}, {255, 3}, {1, 4}, {100, 5}} {
		got := vmExec(bits, []int64{int64(tc[0]), int64(tc[1])}, wire)
		want := ref(tc[0], tc[1])
		if got != want {
			t.Errorf("bits(%d,%d) = %d, want %d", tc[0], tc[1], got, want)
		}
	}
	// spot-check the golden value used by the ART gate.
	if vmExec(bits, []int64{6, 2}, wire) != 7 {
		t.Errorf("bits(6,2) = %d, want 7", vmExec(bits, []int64{6, 2}, wire))
	}
}

const vmNarrow = `.method public static narrow(I)I
    .registers 4
    const/high16 v0, 0x12340000
    or-int v0, v0, p0
    int-to-short v1, v0
    int-to-char v2, v0
    int-to-byte v0, v0
    add-int v0, v0, v1
    add-int v0, v0, v2
    return v0
.end method`

func TestVMNarrowingAndHigh16(t *testing.T) {
	wire := vmPermutation(0x5117e1d)
	narrow := compileStr(t, vmNarrow, wire)
	ref := func(a int32) int32 {
		v0 := int32(0x12340000) // const/high16 operand is the full value
		v0 |= a
		v1 := int32(int16(v0))
		v2 := int32(uint16(v0))
		v0 = int32(int8(v0))
		return v0 + v1 + v2
	}
	for _, a := range []int32{0xabcd, 0x1234, 0x7f, 0x80, -1, 0} {
		if got := vmExec(narrow, []int64{int64(a)}, wire); got != ref(a) {
			t.Errorf("narrow(%#x) = %d, want %d", a, got, ref(a))
		}
	}
	if vmExec(narrow, []int64{0xabcd}, wire) != 22375 {
		t.Errorf("narrow(0xABCD) = %d, want 22375", vmExec(narrow, []int64{0xabcd}, wire))
	}
}

const vmPoly64 = `.method public static poly64(II)J
    .registers 8
    int-to-long v0, p0
    int-to-long v2, p1
    const-wide/16 v4, 0x1f
    mul-long v0, v0, v4
    add-long v0, v0, v2
    return-wide v0
.end method`

const vmBits64 = `.method public static bits64(II)J
    .registers 12
    int-to-long v0, p0
    int-to-long v2, p1
    const-wide/16 v4, 0x64
    mul-long v0, v0, v4
    add-long v0, v0, v2
    const-wide/16 v4, 0x7
    div-long v6, v0, v4
    rem-long v8, v0, v4
    add-long v6, v6, v8
    shl-long v6, v6, p1
    return-wide v6
.end method`

func TestVMLong(t *testing.T) {
	wire := vmPermutation(0x5117e1d)
	poly := compileStr(t, vmPoly64, wire)
	bits := compileStr(t, vmBits64, wire)

	polyRef := func(a, b int32) int64 { return int64(a)*31 + int64(b) }
	bitsRef := func(a, b int32) int64 {
		x := int64(a)*100 + int64(b)
		s := jdivL(x, 7) + jremL(x, 7)
		return s << (uint(b) & 63)
	}
	for _, tc := range [][2]int32{{3, 4}, {-5, 10}, {100000, 7}, {0, 0}, {-1, 3}} {
		if got := vmRun(poly, []int64{int64(tc[0]), int64(tc[1])}, wire); got != polyRef(tc[0], tc[1]) {
			t.Errorf("poly64(%d,%d) = %d, want %d", tc[0], tc[1], got, polyRef(tc[0], tc[1]))
		}
		if got := vmRun(bits, []int64{int64(tc[0]), int64(tc[1])}, wire); got != bitsRef(tc[0], tc[1]) {
			t.Errorf("bits64(%d,%d) = %d, want %d", tc[0], tc[1], got, bitsRef(tc[0], tc[1]))
		}
	}
	// overflow: 100000*31 fits in long but not int -> proves 64-bit width.
	if got := vmRun(poly, []int64{100000, 0}, wire); got != 3100000 {
		t.Errorf("poly64(100000,0) = %d, want 3100000", got)
	}
}

const vmWide = `.method public static wide(II)J
    .registers 12
    int-to-long v0, p0
    int-to-long v2, p1
    mul-long v4, v0, v2
    and-long v6, v0, v2
    add-long v4, v4, v6
    or-long v6, v0, v2
    add-long v4, v4, v6
    xor-long v6, v0, v2
    add-long v4, v4, v6
    not-long v6, v2
    add-long v4, v4, v6
    move-wide v8, v4
    neg-long v8, v8
    sub-long v4, v4, v8
    return-wide v4
.end method`

func TestVMWideAccumulate(t *testing.T) {
	wire := vmPermutation(0x5117e1d)
	code := compileStr(t, vmWide, wire)
	ref := func(a, b int32) int64 {
		A, B := int64(a), int64(b)
		r := A*B + (A & B) + (A | B) + (A ^ B) + ^B
		return r - (-r)
	}
	for _, tc := range [][2]int32{{100000, 100000}, {3, 4}, {-7, 11}, {0, 0}} {
		if got := vmRun(code, []int64{int64(tc[0]), int64(tc[1])}, wire); got != ref(tc[0], tc[1]) {
			t.Errorf("wide(%d,%d) = %d, want %d", tc[0], tc[1], got, ref(tc[0], tc[1]))
		}
	}
	if got := vmRun(code, []int64{100000, 100000}, wire); got != 20000199998 {
		t.Errorf("wide(100000,100000) = %d, want 20000199998 (64-bit)", got)
	}
}

const vmCombine = `.method public static combine(JI)J
    .registers 8
    int-to-long v0, p2
    add-long v0, p0, v0
    const-wide/16 v2, 0x2
    mul-long v0, v0, v2
    return-wide v0
.end method`

func TestVMLongParam(t *testing.T) {
	wire := vmPermutation(0x5117e1d)
	code := compileStr(t, vmCombine, wire)
	ref := func(a int64, b int32) int64 { return (a + int64(b)) * 2 }
	cases := []struct {
		a int64
		b int32
	}{{5000000000, 5}, {0, 0}, {-3, 7}, {1 << 40, -1}}
	for _, c := range cases {
		// args: long param in slot 0, int param in slot 1 (one slot per param).
		if got := vmRun(code, []int64{c.a, int64(c.b)}, wire); got != ref(c.a, c.b) {
			t.Errorf("combine(%d,%d) = %d, want %d", c.a, c.b, got, ref(c.a, c.b))
		}
	}
	if got := vmRun(code, []int64{5000000000, 5}, wire); got != 10000000010 {
		t.Errorf("combine(5e9,5) = %d, want 10000000010", got)
	}
}

func TestVMPermutationIsBijection(t *testing.T) {
	for _, seed := range []int64{0, 1, 42, 0x5117e1d, -99} {
		wire := vmPermutation(seed)
		if len(wire) != opCount {
			t.Fatalf("wire len %d", len(wire))
		}
		seen := make(map[byte]bool)
		for _, w := range wire {
			if int(w) >= opCount || seen[w] {
				t.Fatalf("seed %d: not a bijection: %v", seed, wire)
			}
			seen[w] = true
		}
	}
}

func TestVMPolymorphicAcrossSeeds(t *testing.T) {
	a := compileStr(t, vmPoly, vmPermutation(1))
	b := compileStr(t, vmPoly, vmPermutation(2))
	if string(a) == string(b) {
		t.Error("different seeds should yield different bytecode (polymorphism)")
	}
}

// TestVMDumpVector writes a test vector (wire permutation, bytecode, and
// expected results) to SHIELD_VM_VEC so a JVM harness can validate that the
// injected smali interpreter algorithm is correct end-to-end. Skipped unless set.
func TestVMDumpVector(t *testing.T) {
	path := os.Getenv("SHIELD_VM_VEC")
	if path == "" {
		t.Skip("set SHIELD_VM_VEC to emit the VM validation vector")
	}
	wire := vmPermutation(0x5117e1d)
	// Use bits() so the JVM harness validates the extended integer ALU (#14).
	code := compileStr(t, vmBits, wire)
	var sb strings.Builder
	sb.WriteString(hexBytes(wire) + "\n")
	sb.WriteString(hexBytes(code) + "\n")
	for _, tc := range [][2]int32{{6, 2}, {13, 1}, {255, 3}, {1, 4}} {
		sb.WriteString(strconv.Itoa(int(tc[0])) + " " + strconv.Itoa(int(tc[1])) + " " +
			strconv.Itoa(int(vmExec(code, []int64{int64(tc[0]), int64(tc[1])}, wire))) + "\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hexBytes(b []byte) string {
	const h = "0123456789abcdef"
	var s strings.Builder
	for _, x := range b {
		s.WriteByte(h[x>>4])
		s.WriteByte(h[x&0xf])
	}
	return s.String()
}

func TestVMBailsOnUnsupported(t *testing.T) {
	// contains an invoke -> not virtualizable
	src := `.method public static f(I)I
    .registers 2
    invoke-static {}, Lx/Y;->g()V
    return p0
.end method`
	if _, ok := compileMethod(strings.Split(src, "\n"), vmPermutation(1)); ok {
		t.Error("expected bail on method with invoke")
	}
	// non-int param -> bail
	src2 := `.method public static f(Ljava/lang/String;)I
    .registers 2
    const/4 v0, 0x1
    return v0
.end method`
	if _, ok := compileMethod(strings.Split(src2, "\n"), vmPermutation(1)); ok {
		t.Error("expected bail on non-int param")
	}
}
