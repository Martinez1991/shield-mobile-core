package engine

import (
	"strings"

	"shield/internal/smali"
)

// stripDirectives are debug-only smali directives that carry no runtime
// semantics: removing them deletes line tables, local-variable names and the
// original source file name (shield-platform.md section 3.5 Metadata Removal).
var stripDirectives = []string{
	".line ",
	".local ",
	".end local ",
	".restart local ",
	".prologue",
	".source ",
}

// passMetadata removes debug metadata from every class. Returns the number of
// directive lines removed. Semantics-safe: none of these affect execution.
func passMetadata(classes []*smali.Class) int {
	removed := 0
	for _, c := range classes {
		out := c.Lines[:0:0] // fresh slice, don't alias
		for _, ln := range c.Lines {
			if isDebugDirective(ln) {
				removed++
				continue
			}
			out = append(out, ln)
		}
		c.Lines = out
	}
	return removed
}

func isDebugDirective(line string) bool {
	t := strings.TrimSpace(line)
	for _, d := range stripDirectives {
		// ".source" / ".prologue" have no trailing space when bare.
		if t == strings.TrimSpace(d) || strings.HasPrefix(t, d) {
			return true
		}
	}
	return false
}
