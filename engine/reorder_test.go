package engine

import (
	"sort"
	"strings"
	"testing"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

const branchy = `.class public Lcom/x/Y;
.super Ljava/lang/Object;

.method public static sum(I)I
    .registers 4
    const/4 v0, 0x0
    const/4 v1, 0x0
    :loop
    if-ge v1, p0, :done
    add-int/2addr v0, v1
    add-int/lit8 v1, v1, 0x1
    goto :loop
    :done
    if-lez v0, :neg
    return v0
    :neg
    const/4 v0, 0x0
    return v0
.end method
`

func TestReorderPreservesInstructionsAndEntry(t *testing.T) {
	c := &smali.Class{Descriptor: "Lcom/x/Y;", Lines: strings.Split(branchy, "\n")}
	orig := allInstrs(c.Lines)

	mid := 0
	n := passReorder([]*smali.Class{c}, 0x5117e1d)
	if n != 1 {
		t.Fatalf("reordered = %d, want 1", n)
	}
	_ = mid

	got := allInstrs(c.Lines)
	// Every original instruction must still be present (multiset equality); the
	// only additions allowed are `goto` reconnections.
	origNoGoto := withoutGotos(orig)
	gotNoGoto := withoutGotos(got)
	if !multisetEqual(origNoGoto, gotNoGoto) {
		t.Errorf("non-goto instructions changed.\norig=%v\ngot =%v", origNoGoto, gotNoGoto)
	}

	// Entry instruction must remain the first executable line.
	first := firstInstr(c.Lines)
	if first != "const/4 v0, 0x0" {
		t.Errorf("entry instruction changed to %q", first)
	}
}

func TestReorderBailsOnTryCatch(t *testing.T) {
	src := `.class public Lcom/x/Y;
.super Ljava/lang/Object;

.method public static f()V
    .registers 1
    :try_start_0
    invoke-static {}, Lcom/x/Y;->f()V
    :try_end_0
    .catch Ljava/lang/Exception; {:try_start_0 .. :try_end_0} :h
    return-void
    :h
    move-exception v0
    return-void
.end method
`
	c := &smali.Class{Descriptor: "Lcom/x/Y;", Lines: strings.Split(src, "\n")}
	before := strings.Join(c.Lines, "\n")
	passReorder([]*smali.Class{c}, 1)
	if strings.Join(c.Lines, "\n") != before {
		t.Error("reorder must bail (leave unchanged) on try/catch methods")
	}
}

func TestReorderIsDeterministic(t *testing.T) {
	a := &smali.Class{Descriptor: "Lcom/x/Y;", Lines: strings.Split(branchy, "\n")}
	b := &smali.Class{Descriptor: "Lcom/x/Y;", Lines: strings.Split(branchy, "\n")}
	passReorder([]*smali.Class{a}, 99)
	passReorder([]*smali.Class{b}, 99)
	if strings.Join(a.Lines, "\n") != strings.Join(b.Lines, "\n") {
		t.Error("reorder must be deterministic for a fixed seed (P2)")
	}
}

func allInstrs(lines []string) []string {
	var out []string
	in := false
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(t, ".method "):
			in = true
		case t == ".end method":
			in = false
		case in && t != "" && !strings.HasPrefix(t, ".") && !strings.HasPrefix(t, ":") && !strings.HasPrefix(t, "#"):
			out = append(out, t)
		}
	}
	return out
}

func withoutGotos(in []string) []string {
	var out []string
	for _, s := range in {
		if !strings.HasPrefix(s, "goto") {
			out = append(out, s)
		}
	}
	return out
}

func multisetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func firstInstr(lines []string) string {
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, ".method ") {
			continue
		}
		if t != "" && !strings.HasPrefix(t, ".") && !strings.HasPrefix(t, ":") && !strings.HasPrefix(t, "#") {
			return t
		}
	}
	return ""
}
