package engine

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// Type descriptors are the only tokens we rewrite for renaming: every class,
// field and method reference in smali names its owning type as `L...;`.
var descTokenRE = regexp.MustCompile(`L[A-Za-z0-9_$/]+;`)

// neverRename are runtime/library namespaces we must not touch.
var neverRename = []string{
	"Landroid/", "Landroidx/", "Ljava/", "Ljavax/", "Lkotlin/",
	"Lkotlinx/", "Ldalvik/", "Lorg/", "Lcom/google/", "Lshield/rt/",
}

// passRename renames app-owned classes to short opaque names (section 3.1) and
// rewrites every reference. Returns old->new descriptor map (for the mapping file).
// Reachability-aware: only descriptors under includePrefixes and not in keep are
// renamed, protecting entry points, reflection targets and library code.
func passRename(classes []*smali.Class, includePrefixes, keep []string) map[string]string {
	keepSet := make(map[string]bool)
	for _, k := range keep {
		keepSet[normalizeKeep(k)] = true
	}

	var owned []string
	seen := make(map[string]bool)
	for _, c := range classes {
		d := c.Descriptor
		if d == "" || seen[d] {
			continue
		}
		if isOwned(d, includePrefixes) && !keepSet[d] {
			owned = append(owned, d)
			seen[d] = true
		}
	}
	sort.Strings(owned) // determinism (P2)

	m := make(map[string]string, len(owned))
	for i, d := range owned {
		m[d] = "Lo/" + shortName(i) + ";"
	}
	if len(m) == 0 {
		return m
	}

	rewriteRefs(classes, m)
	for _, c := range classes {
		if nd, ok := m[c.Descriptor]; ok {
			c.Descriptor = nd
		}
	}
	return m
}

func rewriteRefs(classes []*smali.Class, m map[string]string) {
	repl := func(tok string) string {
		if nd, ok := m[tok]; ok {
			return nd
		}
		return tok
	}
	for _, c := range classes {
		for i, ln := range c.Lines {
			if strings.Contains(ln, "L") {
				c.Lines[i] = descTokenRE.ReplaceAllStringFunc(ln, repl)
			}
		}
	}
}

func isOwned(desc string, includePrefixes []string) bool {
	for _, p := range neverRename {
		if strings.HasPrefix(desc, p) {
			return false
		}
	}
	internal := smali.InternalName(desc) // e.g. com/bank/pay/Foo
	for _, pre := range includePrefixes {
		pre = strings.TrimSpace(strings.ReplaceAll(pre, ".", "/"))
		if pre == "" {
			continue
		}
		if internal == pre || strings.HasPrefix(internal, pre+"/") {
			return true
		}
	}
	return false
}

// normalizeKeep accepts either "Lcom/foo/Bar;" or "com.foo.Bar" and returns the
// descriptor form.
func normalizeKeep(k string) string {
	k = strings.TrimSpace(k)
	if strings.HasPrefix(k, "L") && strings.HasSuffix(k, ";") {
		return k
	}
	return "L" + strings.ReplaceAll(k, ".", "/") + ";"
}

// shortName maps 0,1,2,... to a,b,...,z,aa,ab,... (base-26, lowercase).
func shortName(i int) string {
	const alpha = "abcdefghijklmnopqrstuvwxyz"
	var out []byte
	i++ // 1-based so 0 -> "a"
	for i > 0 {
		i--
		out = append([]byte{alpha[i%26]}, out...)
		i /= 26
	}
	return string(out)
}
