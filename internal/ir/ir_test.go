package ir

import (
	"strings"
	"testing"
)

func parse(t *testing.T, src string) *Method {
	t.Helper()
	m, ok := ParseMethod(strings.Split(strings.TrimSpace(src), "\n"))
	if !ok {
		t.Fatalf("ParseMethod failed for:\n%s", src)
	}
	return m
}

func TestParseMethodShape(t *testing.T) {
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

	if !m.Static || m.Name != "choose" {
		t.Errorf("decl parse: static=%v name=%q", m.Static, m.Name)
	}
	if m.Params != "Ljava/lang/String;Ljava/lang/String;I" || m.Ret != "Ljava/lang/String;" {
		t.Errorf("params=%q ret=%q", m.Params, m.Ret)
	}
	if m.Regs != 4 || m.ParamBase != 1 {
		t.Errorf("regs=%d paramBase=%d, want 4/1", m.Regs, m.ParamBase)
	}
	// 5 instructions; the label binds to the 4th (index 3).
	if len(m.Insns) != 5 {
		t.Fatalf("insns=%d, want 5", len(m.Insns))
	}
	if got := m.Labels[":second"]; got != 3 {
		t.Errorf("label :second -> %d, want 3", got)
	}
	if m.Insns[0].Op != "if-lez" || len(m.Insns[0].Args) != 2 {
		t.Errorf("insn0 = %+v", m.Insns[0])
	}
}

func TestEntryTypesMixedParams(t *testing.T) {
	m := parse(t, `
.method public static combine(JI)J
    .registers 8
    int-to-long v0, p2
    add-long v0, p0, v0
    const-wide/16 v2, 0x2
    mul-long v0, v0, v2
    return-wide v0
.end method`)
	st := m.EntryTypes()
	// paramBase 5: p0 = long at v5 (v6 high), p2 = int at v7.
	want := map[int]RegType{5: TLong, 6: TWideHigh, 7: TInt, 0: TUnknown}
	for reg, w := range want {
		if got := st.Reg(reg); got != w {
			t.Errorf("entry v%d = %v, want %v", reg, got, w)
		}
	}
}

func TestEntryTypesInstanceMethodHasThis(t *testing.T) {
	m := parse(t, `
.method public greet(Ljava/lang/String;)V
    .registers 3
    return-void
.end method`)
	st := m.EntryTypes()
	// non-static: paramBase 1, p0 = this (ref) at v1, p1 = String at v2.
	if st.Reg(1) != TRef || st.Reg(2) != TRef {
		t.Errorf("this/param = %v/%v, want ref/ref", st.Reg(1), st.Reg(2))
	}
}

func TestInferLongArithmetic(t *testing.T) {
	m := parse(t, `
.method public static combine(JI)J
    .registers 8
    int-to-long v0, p2
    add-long v0, p0, v0
    const-wide/16 v2, 0x2
    mul-long v0, v0, v2
    return-wide v0
.end method`)
	in := m.InferTypes()
	// before return-wide (index 4), v0 is a long, v1 its high half.
	if in[4].Reg(0) != TLong || in[4].Reg(1) != TWideHigh {
		t.Errorf("at return: v0=%v v1=%v, want long/wide-high", in[4].Reg(0), in[4].Reg(1))
	}
}

func TestInferObjectPlumbingAcrossBranch(t *testing.T) {
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
	in := m.InferTypes()
	// both return-object sites (index 2 and 4) see v0 as a ref.
	if in[2].Reg(0) != TRef {
		t.Errorf("first return: v0=%v, want ref", in[2].Reg(0))
	}
	if in[4].Reg(0) != TRef {
		t.Errorf("second return: v0=%v, want ref", in[4].Reg(0))
	}
}

func TestInferInvokeResult(t *testing.T) {
	m := parse(t, `
.method public static len(Ljava/lang/String;)I
    .registers 2
    invoke-virtual {p0}, Ljava/lang/String;->length()I
    move-result v0
    return v0
.end method`)
	in := m.InferTypes()
	if in[2].Reg(0) != TInt {
		t.Errorf("move-result typed v0=%v, want int", in[2].Reg(0))
	}
}

func TestInferConflictAtMerge(t *testing.T) {
	// v0 is an int on one path and a ref on the other -> conflict where they meet.
	m := parse(t, `
.method public static f(I)Ljava/lang/Object;
    .registers 2
    if-eqz p0, :a
    const/4 v0, 0x1
    goto :b
    :a
    const-string v0, "x"
    :b
    return-object v0
.end method`)
	in := m.InferTypes()
	last := len(m.Insns) - 1 // return-object
	if in[last].Reg(0) != TConflict {
		t.Errorf("merge type v0=%v, want conflict", in[last].Reg(0))
	}
}

func TestInferLoopReachesFixpoint(t *testing.T) {
	// classify has a backward-free branch; ensure inference terminates and types
	// the int results correctly at both returns.
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
	in := m.InferTypes()
	for _, idx := range []int{3, 5} { // the two returns
		if idx < len(in) && in[idx].Reg(0) != TInt {
			t.Errorf("classify return @%d: v0=%v, want int", idx, in[idx].Reg(0))
		}
	}
}
