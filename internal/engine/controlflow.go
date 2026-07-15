package engine

import (
	"fmt"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// passControlFlow inserts an always-true opaque predicate guarding a dead junk
// block at each method's entry (shield-platform.md section 3.2: opaque
// predicates + fake branches). This breaks naive linear disassembly and pollutes
// the CFG without a central dispatcher.
//
// Verifier-safety: it reuses v0, which is a *local* register (never a parameter)
// and is provably not-yet-live at method entry — the Dalvik verifier requires
// every local to be assigned before use, so clobbering it before the real body
// is harmless. No register-count change, no renumbering. Methods with zero free
// locals, or with no body (abstract/native), are skipped.
//
// Emitted shape (id unique per injection):
//
//	const/16 v0, <positive const>
//	mul-int/2addr v0, v0          # square is always >= 0 (const < 2^15, no overflow)
//	if-gez v0, :shield_<id>       # always taken
//	const/4 v0, 0x0               # dead: reachable to the verifier, never at runtime
//	:shield_<id>
//	<original body>
func passControlFlow(classes []*smali.Class, seed int64) int {
	id := 0
	injected := 0
	for _, c := range classes {
		forEachMethod(c, func(block []string) []string {
			idx := firstBodyIndex(block)
			if idx < 0 {
				return block
			}
			locals, ok := localsAtEntry(block)
			if !ok || locals < 1 {
				return block // no free local register at entry; skip to stay safe
			}
			k := int((uint64(seed)>>3+uint64(id))&0x3FFF) | 1 // positive, k*k < 2^28
			label := fmt.Sprintf(":shield_%d", id)
			id++
			injected++
			pred := []string{
				fmt.Sprintf("    const/16 v0, 0x%x", k),
				"    mul-int/2addr v0, v0",
				"    if-gez v0, " + label,
				"    const/4 v0, 0x0",
				"    " + label,
			}
			out := make([]string, 0, len(block)+len(pred))
			out = append(out, block[:idx]...)
			out = append(out, pred...)
			out = append(out, block[idx:]...)
			return out
		})
	}
	return injected
}
