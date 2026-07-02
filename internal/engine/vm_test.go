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
	code := compileStr(t, vmPoly, wire)
	var sb strings.Builder
	sb.WriteString(hexBytes(wire) + "\n")
	sb.WriteString(hexBytes(code) + "\n")
	for _, tc := range [][2]int32{{3, 4}, {-5, 10}, {100, -7}} {
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
