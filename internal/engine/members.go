package engine

import (
	"regexp"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// Member (method/field) renaming (shield-platform.md section 3.1). Scope is
// deliberately conservative to guarantee semantic correctness (section 20 gate):
//
//   - methods: only `private` or `static` ones (never virtual/vtable-dispatched,
//     so overrides and framework callbacks like onCreate are untouched); never
//     <init>/<clinit>, native (JNI), or synthetic/bridge helpers.
//   - fields: only `private` ones (never externally visible).
//   - enum classes are skipped entirely (values()/valueOf reflection).
//
// References carry the full owner+name+signature, so cross-class static calls
// and field access are rewritten exactly. Runs BEFORE class renaming, so it can
// reason about original, scoped descriptors.

var (
	methodDeclRE = regexp.MustCompile(`^(\s*\.method\s+)((?:[\w-]+\s+)*)([\w$<>]+)(\(.*)$`)
	fieldDeclRE  = regexp.MustCompile(`^(\s*\.field\s+)((?:[\w-]+\s+)*)([\w$]+):([\[\w$/;]+)(.*)$`)
	memberRefRE  = regexp.MustCompile(`(L[\w$/]+;->)([\w$<>]+)(\([^)]*\)\[*[\w$/;]+|:\[*[\w$/;]+)`)
)

func passRenameMembers(classes []*smali.Class, includePrefixes, keep []string) int {
	keepSet := make(map[string]bool)
	for _, k := range keep {
		keepSet[normalizeKeep(k)] = true
	}

	// refMap: full member reference token -> new name.
	refMap := make(map[string]string)
	for _, c := range classes {
		if !ownedForMembers(c, includePrefixes) || keepSet[c.Descriptor] {
			continue
		}
		used := collectMemberSigs(c)
		counter := 0
		for _, ln := range c.Lines {
			if m := methodDeclRE.FindStringSubmatch(ln); m != nil {
				flags, name, sig := m[2], m[3], m[4]
				if !methodEligible(flags, name) {
					continue
				}
				nn := genMemberName(&counter, used, name+sig)
				refMap[c.Descriptor+"->"+name+sig] = nn
			} else if m := fieldDeclRE.FindStringSubmatch(ln); m != nil {
				flags, name, typ := m[2], m[3], m[4]
				if !fieldEligible(flags) {
					continue
				}
				nn := genMemberName(&counter, used, name+":"+typ)
				refMap[c.Descriptor+"->"+name+":"+typ] = nn
			}
		}
	}
	if len(refMap) == 0 {
		return 0
	}

	// 1) Rewrite every reference (invoke-*, field ops) across all classes.
	for _, c := range classes {
		for i, ln := range c.Lines {
			if !strings.Contains(ln, ";->") {
				continue
			}
			c.Lines[i] = memberRefRE.ReplaceAllStringFunc(ln, func(tok string) string {
				mm := memberRefRE.FindStringSubmatch(tok)
				if nn, ok := refMap[mm[1]+mm[2]+mm[3]]; ok {
					return mm[1] + nn + mm[3]
				}
				return tok
			})
		}
	}

	// 2) Rewrite declarations in the owning classes.
	for _, c := range classes {
		if !ownedForMembers(c, includePrefixes) || keepSet[c.Descriptor] {
			continue
		}
		for i, ln := range c.Lines {
			if m := methodDeclRE.FindStringSubmatch(ln); m != nil {
				if nn, ok := refMap[c.Descriptor+"->"+m[3]+m[4]]; ok {
					c.Lines[i] = m[1] + m[2] + nn + m[4]
				}
			} else if m := fieldDeclRE.FindStringSubmatch(ln); m != nil {
				if nn, ok := refMap[c.Descriptor+"->"+m[3]+":"+m[4]]; ok {
					c.Lines[i] = m[1] + m[2] + nn + ":" + m[4] + m[5]
				}
			}
		}
	}
	return len(refMap)
}

// ownedForMembers reports whether a class is app-owned and eligible for member
// renaming (not the injected runtime, not an enum).
func ownedForMembers(c *smali.Class, includePrefixes []string) bool {
	if c.Descriptor == decryptorDescriptor || !isOwned(c.Descriptor, includePrefixes) {
		return false
	}
	for _, ln := range c.Lines {
		if strings.TrimSpace(ln) == ".super Ljava/lang/Enum;" {
			return false
		}
	}
	return true
}

func collectMemberSigs(c *smali.Class) map[string]bool {
	set := make(map[string]bool)
	for _, ln := range c.Lines {
		if m := methodDeclRE.FindStringSubmatch(ln); m != nil {
			set[m[3]+m[4]] = true
		} else if m := fieldDeclRE.FindStringSubmatch(ln); m != nil {
			set[m[3]+":"+m[4]] = true
		}
	}
	return set
}

func methodEligible(flags, name string) bool {
	if name == "<init>" || name == "<clinit>" {
		return false
	}
	f := flagSet(flags)
	if f["native"] || f["bridge"] || f["synthetic"] {
		return false
	}
	return f["private"] || f["static"]
}

func fieldEligible(flags string) bool {
	f := flagSet(flags)
	if f["synthetic"] {
		return false
	}
	return f["private"]
}

func flagSet(flags string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(flags) {
		set[w] = true
	}
	return set
}

// genMemberName returns a short name whose name+signature doesn't collide with
// any member already present (or generated) in the class.
func genMemberName(counter *int, used map[string]bool, nameAndSig string) string {
	tail := sigTail(nameAndSig)
	for {
		name := shortName(*counter)
		*counter++
		key := name + tail
		if !used[key] {
			used[key] = true
			return name
		}
	}
}

// sigTail strips the original member name off a "name+sig" / "name:type" string,
// returning just the signature portion so a new name can be prepended.
func sigTail(nameAndSig string) string {
	if i := strings.IndexAny(nameAndSig, "(:"); i >= 0 {
		return nameAndSig[i:]
	}
	return nameAndSig
}
