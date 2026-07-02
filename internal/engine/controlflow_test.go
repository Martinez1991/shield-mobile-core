package engine

import (
	"strings"
	"testing"

	"shield/internal/smali"
)

func mkClass(body string) *smali.Class {
	return &smali.Class{
		Descriptor: "Lcom/x/Y;",
		Lines:      strings.Split(body, "\n"),
	}
}

func TestControlFlowInjectsOpaquePredicate(t *testing.T) {
	c := mkClass(`.class public Lcom/x/Y;
.super Ljava/lang/Object;

.method public compute(I)I
    .registers 3
    .param p1, "n"
    add-int/lit8 v0, p1, 0x1
    return v0
.end method
`)
	n := passControlFlow([]*smali.Class{c}, 0x5117e1d)
	if n != 1 {
		t.Fatalf("injected = %d, want 1", n)
	}
	body := strings.Join(c.Lines, "\n")
	if !strings.Contains(body, "mul-int/2addr v0, v0") || !strings.Contains(body, "if-gez v0,") {
		t.Errorf("opaque predicate not injected:\n%s", body)
	}
	// Injection must be AFTER .param and BEFORE the first instruction.
	idxParam := strings.Index(body, ".param")
	idxPred := strings.Index(body, "const/16 v0")
	idxReal := strings.Index(body, "add-int/lit8")
	if !(idxParam < idxPred && idxPred < idxReal) {
		t.Errorf("bad ordering: param=%d pred=%d real=%d", idxParam, idxPred, idxReal)
	}
}

func TestControlFlowSkipsWhenNoFreeLocal(t *testing.T) {
	// .registers 1 on a static 1-arg method => 0 locals free at entry.
	c := mkClass(`.class public Lcom/x/Y;
.super Ljava/lang/Object;

.method public static id(I)I
    .registers 1
    return p0
.end method
`)
	if n := passControlFlow([]*smali.Class{c}, 1); n != 0 {
		t.Errorf("expected skip (no free local), got %d injections", n)
	}
}

func TestControlFlowSkipsAbstract(t *testing.T) {
	c := mkClass(`.class public abstract Lcom/x/Y;
.super Ljava/lang/Object;

.method public abstract foo()V
.end method
`)
	if n := passControlFlow([]*smali.Class{c}, 1); n != 0 {
		t.Errorf("expected skip (abstract), got %d", n)
	}
}

func TestParamRegisters(t *testing.T) {
	cases := []struct {
		decl string
		want int
	}{
		{".method public foo()V", 1},          // this
		{".method public static foo()V", 0},   // none
		{".method public static foo(I)V", 1},  // int
		{".method public foo(I)V", 2},         // this + int
		{".method public static foo(JD)V", 4}, // long+double
		{".method public static foo(Ljava/lang/String;I)V", 2},
		{".method public static foo([I[Ljava/lang/String;)V", 2}, // two arrays
	}
	for _, tc := range cases {
		if got := paramRegisters(tc.decl); got != tc.want {
			t.Errorf("paramRegisters(%q) = %d, want %d", tc.decl, got, tc.want)
		}
	}
}
