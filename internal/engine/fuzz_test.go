package engine

import (
	"strings"
	"testing"

	"shield/internal/smali"
)

// The parsers/passes operate on attacker-influenced smali (issue #10, risk RM2).
// These fuzz targets assert they never panic on malformed input. Run the seed
// corpus with `go test`; fuzz with e.g. `go test -run x -fuzz FuzzReorderMethod`.

var fuzzSeeds = []string{
	"",
	"\n",
	".method",
	".method public static f(I)I\n.end method\n",
	".method x\n.end method\ncode", // .end method before body
	".method public foo()V\n.registers\n:l\n.end method",
	".method a\n.registers 1\nconst-string v0, \"\\uZZZZ\"\nreturn-void\n.end method",
	":label\ngoto :label\nif-eqz v0, :x\n",
	".field private a:I = 0x1",
	"const-string v0, \"L\\u0041;\"",
	"invoke-static {}, Lx/Y;->m()V\n.method\n(\n",
}

func addSeeds(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
}

func FuzzUnescapeSmali(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = unescapeSmali(s) // must not panic
	})
}

func FuzzCompileMethod(f *testing.F) {
	addSeeds(f)
	wire := vmPermutation(1)
	f.Fuzz(func(t *testing.T, s string) {
		_, _, _ = compileMethod(strings.Split(s, "\n"), wire)
	})
}

func FuzzReorderMethod(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, s string) {
		mid := 0
		_, _ = reorderMethod(strings.Split(s, "\n"), 1, &mid)
	})
}

func FuzzSplitBlocks(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = splitBlocks(strings.Split(s, "\n"))
	})
}

func FuzzEstimateMethodRefs(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, s string) {
		c := &smali.Class{Descriptor: "Lx/Y;", Lines: strings.Split(s, "\n")}
		_ = estimateMethodRefs([]*smali.Class{c})
	})
}

func FuzzPassesOnClass(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, s string) {
		mk := func() []*smali.Class {
			return []*smali.Class{{Descriptor: "Lcom/x/Y;", Lines: strings.Split(s, "\n")}}
		}
		_ = passMetadata(mk())
		_ = passStrings(mk(), 1, "aes", 7)
		_ = passRenameMembers(mk(), []string{"com/x"}, nil)
		_ = passReorder(mk(), 7)
		_ = passControlFlow(mk(), 7)
		_ = passJunk(mk(), 2)
		_, _ = passVirtualize(mk(), []string{"com/x"}, 7, "base")
	})
}
