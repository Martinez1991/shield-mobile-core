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
		if got := vmExec(sum, []int32{tc.a, tc.b}, wire); got != tc.a+tc.b {
			t.Errorf("sum2(%d,%d) = %d, want %d", tc.a, tc.b, got, tc.a+tc.b)
		}
		want := tc.a*tc.a + tc.b + 5
		if got := vmExec(poly, []int32{tc.a, tc.b}, wire); got != want {
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
		if got := vmExec(sum, []int32{n}, wire); got != sumRef(n) {
			t.Errorf("sum(%d) = %d, want %d", n, got, sumRef(n))
		}
		if got := vmExec(classify, []int32{n}, wire); got != classifyRef(n) {
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
		got := vmExec(bits, []int32{tc[0], tc[1]}, wire)
		want := ref(tc[0], tc[1])
		if got != want {
			t.Errorf("bits(%d,%d) = %d, want %d", tc[0], tc[1], got, want)
		}
	}
	// spot-check the golden value used by the ART gate.
	if vmExec(bits, []int32{6, 2}, wire) != 7 {
		t.Errorf("bits(6,2) = %d, want 7", vmExec(bits, []int32{6, 2}, wire))
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
			strconv.Itoa(int(vmExec(code, []int32{tc[0], tc[1]}, wire))) + "\n")
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
