package engine

import "shield/internal/smali"

// passJunk inserts N verifier-safe `nop` instructions at the head of every
// method body (a conservative slice of section 3.2). It never changes registers
// or reachability, so semantic correctness (the section 20 gate) is preserved.
// Returns the number of methods padded.
func passJunk(classes []*smali.Class, nops int) int {
	if nops <= 0 {
		return 0
	}
	pad := make([]string, nops)
	for i := range pad {
		pad[i] = "    nop"
	}
	touched := 0
	for _, c := range classes {
		forEachMethod(c, func(block []string) []string {
			idx := firstBodyIndex(block)
			if idx < 0 {
				return block
			}
			touched++
			out := make([]string, 0, len(block)+nops)
			out = append(out, block[:idx]...)
			out = append(out, pad...)
			out = append(out, block[idx:]...)
			return out
		})
	}
	return touched
}
