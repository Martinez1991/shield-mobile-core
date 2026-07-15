package engine

import (
	"sort"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/risk"
	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// riskKey is a stable identity for a method — its owner descriptor plus
// name+signature — which survives the VM/flatten passes (they keep the decl).
func riskKey(classDesc string, block []string) string {
	if len(block) == 0 {
		return classDesc
	}
	if m := vmMethodDeclRE.FindStringSubmatch(block[0]); m != nil {
		return classDesc + "->" + m[2] + "(" + m[3] + ")" + m[4]
	}
	return classDesc + "|" + strings.TrimSpace(block[0])
}

// assessMethods scores every owned method before the passes run (the score must
// reflect the original body, not a VM wrapper). Keyed by riskKey.
func assessMethods(classes []*smali.Class, includePrefixes []string) map[string]risk.Assessment {
	out := map[string]risk.Assessment{}
	for _, c := range classes {
		if !isOwned(c.Descriptor, includePrefixes) {
			continue
		}
		forEachMethod(c, func(bk []string) []string {
			out[riskKey(c.Descriptor, bk)] = risk.Assess(bk)
			return bk
		})
	}
	return out
}

// finalizeRiskMap matches each pre-pass assessment to the (now-mutated) method
// body to record whether/how it was protected, and returns the audit list sorted
// by risk score (most-at-risk first).
func finalizeRiskMap(classes []*smali.Class, includePrefixes []string, assessed map[string]risk.Assessment) []RiskEntry {
	var out []RiskEntry
	for _, c := range classes {
		if !isOwned(c.Descriptor, includePrefixes) {
			continue
		}
		forEachMethod(c, func(bk []string) []string {
			a, ok := assessed[riskKey(c.Descriptor, bk)]
			if !ok {
				return bk
			}
			e := RiskEntry{Method: riskKey(c.Descriptor, bk), Score: a.Score, Reasons: a.Reasons}
			body := strings.Join(bk, "\n")
			switch {
			case strings.Contains(body, vmDescriptor+"->run("):
				e.Protected, e.Technique = true, "vm"
			case strings.Contains(body, ".packed-switch"):
				e.Protected, e.Technique = true, "flatten"
			}
			out = append(out, e)
			return bk
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
