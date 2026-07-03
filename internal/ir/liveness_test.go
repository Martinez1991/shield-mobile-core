package ir

import "testing"

func TestLivenessParamsAtEntry(t *testing.T) {
	// choose reads all three params; each must be live on entry (v1,v2 on the two
	// branches, v3 in the selector), and v0 (a local) must not.
	m := parse(t, `
.method public static choose(Ljava/lang/String;Ljava/lang/String;I)Ljava/lang/String;
    .registers 4
    if-lez p2, :second
    move-object v0, p0
    return-object v0
    :second
    move-object v0, p1
    return-object v0
.end method`)
	lv := m.Liveness()
	for _, r := range []int{1, 2, 3} { // p0, p1, p2
		if !lv.LiveInAt(0, r) {
			t.Errorf("param v%d should be live at entry", r)
		}
	}
	if lv.LiveInAt(0, 0) {
		t.Error("local v0 should not be live at entry")
	}
}

func TestLivenessDefKillsUpstream(t *testing.T) {
	// classify: entry needs only p0 (v1); v0 is defined before use on every path,
	// so it is not live at entry.
	m := parse(t, `
.method public static classify(I)I
    .registers 2
    const/16 v0, 0xa
    if-lt p0, v0, :small
    const/16 v0, 0x64
    return v0
    :small
    const/4 v0, 0x1
    return v0
.end method`)
	lv := m.Liveness()
	if !lv.LiveInAt(0, 1) {
		t.Error("p0 (v1) must be live at entry")
	}
	if lv.LiveInAt(0, 0) {
		t.Error("v0 is defined before use -> not live at entry")
	}
	// v0 (the comparand) is live across the branch at the if-lt (index 1).
	if !lv.LiveInAt(1, 0) || !lv.LiveInAt(1, 1) {
		t.Error("if-lt reads v0 and p0 -> both live going in")
	}
	// after the if-lt, neither is needed (each arm loads its own const).
	if lv.Out(1)[0] || lv.Out(1)[1] {
		t.Errorf("nothing live after the branch, got %v", lv.Out(1))
	}
}

func TestLivenessAcrossWideAndInvoke(t *testing.T) {
	// A long accumulate then a call: the long (v0) is live until the mul, and the
	// object arg (p0) is live until the invoke.
	m := parse(t, `
.method public static f(Ljava/lang/String;J)J
    .registers 6
    invoke-virtual {p0}, Ljava/lang/String;->length()I
    move-result v5
    int-to-long v0, v5
    add-long v0, v0, p1
    return-wide v0
.end method`)
	lv := m.Liveness()
	// p0 (v3) live at entry, consumed by the invoke (index 0), dead after.
	if !lv.LiveInAt(0, 3) {
		t.Error("p0 (v3) must be live at entry")
	}
	if lv.Out(0)[3] {
		t.Error("p0 is dead after the invoke that consumes it")
	}
	// p1 (the long param, low reg v4) is live until add-long (index 3).
	if !lv.LiveInAt(3, 4) {
		t.Error("long param p1 (v4) must be live going into add-long")
	}
}

func TestLivenessRangeInvokeExpandsOperands(t *testing.T) {
	// invoke/range uses every register in the range; all must read as live.
	m := parse(t, `
.method public static g(III)I
    .registers 3
    invoke-static/range {v0 .. v2}, Lx/Y;->h(III)I
    move-result v0
    return v0
.end method`)
	lv := m.Liveness()
	for _, r := range []int{0, 1, 2} {
		if !lv.LiveInAt(0, r) {
			t.Errorf("range operand v%d should be live at the invoke", r)
		}
	}
}
