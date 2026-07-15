package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Martinez1991/shield-mobile-core/internal/policy"
	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// genClasses builds n synthetic app-owned classes with a realistic mix of
// const-strings, integer arithmetic (virtualizable), branches (reorderable) and
// plain methods, for benchmarking the passes at scale.
func genClasses(n int) []*smali.Class {
	out := make([]*smali.Class, n)
	for i := 0; i < n; i++ {
		var b strings.Builder
		fmt.Fprintf(&b, ".class public Lcom/bench/C%d;\n.super Ljava/lang/Object;\n\n", i)
		// method with strings
		fmt.Fprintf(&b, ".method public s%d()Ljava/lang/String;\n    .registers 2\n", i)
		fmt.Fprintf(&b, "    const-string v0, \"secret-value-%d-abcdefghij\"\n    return-object v0\n.end method\n\n", i)
		// virtualizable int arithmetic
		fmt.Fprintf(&b, ".method public static m%d(II)I\n    .registers 4\n", i)
		b.WriteString("    mul-int v0, p0, p0\n    add-int v0, v0, p1\n    add-int/lit8 v0, v0, 0x7\n    return v0\n.end method\n\n")
		// branchy method
		fmt.Fprintf(&b, ".method public static b%d(I)I\n    .registers 4\n", i)
		b.WriteString("    const/4 v0, 0x0\n    const/4 v1, 0x0\n    :loop\n    if-ge v1, p0, :done\n    add-int/2addr v0, v1\n    add-int/lit8 v1, v1, 0x1\n    goto :loop\n    :done\n    if-lez v0, :neg\n    return v0\n    :neg\n    const/4 v0, 0x0\n    return v0\n.end method\n")
		out[i] = &smali.Class{
			Descriptor: fmt.Sprintf("Lcom/bench/C%d;", i),
			Base:       "smali",
			Lines:      strings.Split(b.String(), "\n"),
		}
	}
	return out
}

func clone(cs []*smali.Class) []*smali.Class {
	out := make([]*smali.Class, len(cs))
	for i, c := range cs {
		lines := make([]string, len(c.Lines))
		copy(lines, c.Lines)
		out[i] = &smali.Class{Descriptor: c.Descriptor, Base: c.Base, Path: c.Path, Lines: lines}
	}
	return out
}

const benchN = 500

func benchPass(b *testing.B, fn func(cs []*smali.Class)) {
	src := genClasses(benchN)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		cs := clone(src)
		b.StartTimer()
		fn(cs)
	}
}

func BenchmarkPassStrings(b *testing.B) {
	benchPass(b, func(cs []*smali.Class) { passStrings(cs, 2, "aes", 7) })
}
func BenchmarkPassRenameMembers(b *testing.B) {
	benchPass(b, func(cs []*smali.Class) { passRenameMembers(cs, []string{"com/bench"}, nil) })
}
func BenchmarkPassRenameClasses(b *testing.B) {
	benchPass(b, func(cs []*smali.Class) { passRename(cs, []string{"com/bench"}, nil) })
}
func BenchmarkPassReorder(b *testing.B) {
	benchPass(b, func(cs []*smali.Class) { passReorder(cs, 7) })
}
func BenchmarkPassControlFlow(b *testing.B) {
	benchPass(b, func(cs []*smali.Class) { passControlFlow(cs, 7) })
}
func BenchmarkPassVirtualize(b *testing.B) {
	benchPass(b, func(cs []*smali.Class) { passVirtualize(cs, []string{"com/bench"}, 7, "smali", 0) })
}
func BenchmarkEstimateMethodRefs(b *testing.B) {
	src := genClasses(benchN)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		estimateMethodRefs(src)
	}
}

// BenchmarkRunPipeline measures the full plan on disk (I/O included).
func BenchmarkRunPipeline(b *testing.B) {
	p := policy.Preset("prod-high")
	p.Rename.IncludePrefixes = []string{"com/bench"}
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		root := b.TempDir()
		writeGenProject(b, root, 200)
		b.StartTimer()
		if _, err := Run(root, p); err != nil {
			b.Fatal(err)
		}
	}
}

func writeGenProject(b *testing.B, root string, n int) {
	b.Helper()
	for _, c := range genClasses(n) {
		path := smali.DescToRelPath(c.Descriptor)
		full := root + "/smali/" + path
		c.Path = full
		if err := c.Save(full); err != nil {
			b.Fatal(err)
		}
	}
}
