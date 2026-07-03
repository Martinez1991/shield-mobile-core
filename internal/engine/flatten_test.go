package engine

import (
	"strings"
	"testing"
)

func flatten(t *testing.T, src string) (string, bool) {
	t.Helper()
	fid := 0
	out, ok := flattenMethod(strings.Split(strings.TrimSpace(src), "\n"), &fid)
	return strings.Join(out, "\n"), ok
}

func TestFlattenIntMethod(t *testing.T) {
	out, ok := flatten(t, `
.method public static f(I)I
    .registers 3
    if-lez p0, :neg
    const/4 v0, 0x1
    return v0
    :neg
    const/4 v0, 0x0
    return v0
.end method`)
	if !ok {
		t.Fatal("int method with a branch must flatten")
	}
	// Dispatcher present, driven by the dead local v1.
	for _, want := range []string{"packed-switch v1,", ".packed-switch 0x0", ".end packed-switch", "const/16 v1, 0x0"} {
		if !strings.Contains(out, want) {
			t.Errorf("flattened body missing %q:\n%s", want, out)
		}
	}
	// Original computation is preserved (both arms), and the direct branch to the
	// original label is gone (rewritten through the dispatcher).
	if !strings.Contains(out, "const/4 v0, 0x1") || !strings.Contains(out, "const/4 v0, 0x0") {
		t.Error("original block instructions were lost")
	}
	if strings.Contains(out, "if-lez p0, :neg") {
		t.Error("conditional branch was not routed through the dispatcher")
	}
	if strings.Count(out, "return v0") != 2 {
		t.Errorf("expected both returns preserved, got:\n%s", out)
	}
}

func TestFlattenBailsOnReference(t *testing.T) {
	// a ref-typed register would risk a verifier type conflict at the dispatcher
	// join, so the typed-IR gate must refuse it.
	_, ok := flatten(t, `
.method public static g(I)Ljava/lang/String;
    .registers 2
    if-lez p0, :neg
    const-string v0, "pos"
    return-object v0
    :neg
    const-string v0, "neg"
    return-object v0
.end method`)
	if ok {
		t.Error("method with reference registers must not flatten (yet)")
	}
}

func TestFlattenBailsWithoutFreeRegister(t *testing.T) {
	// every local is live -> no register to carry the dispatch state.
	_, ok := flatten(t, `
.method public static h(II)I
    .registers 2
    if-ge p0, p1, :b
    return p0
    :b
    return p1
.end method`)
	if ok {
		t.Error("no free local -> must bail")
	}
}

func TestFlattenSkipsVMWrapper(t *testing.T) {
	_, ok := flatten(t, `
.method public static w(I)I
    .registers 8
    const/16 v0, 0x0
    new-array v0, v0, [J
    invoke-static {v0, v0, v0, v0}, Lshield/rt/VM;->run([B[J[Ljava/lang/Object;[Ljava/lang/String;)Ljava/lang/Object;
    move-result-object v0
    return v0
.end method`)
	if ok {
		t.Error("already-virtualized wrapper must be skipped")
	}
}
