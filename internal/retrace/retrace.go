// Package retrace de-obfuscates stack traces using the mapping produced during
// renaming (shield-platform.md section 3.1; issue #15). The mapping file maps
// original -> obfuscated (dotted) class names; retrace reverses it so a trace
// captured in production reads with the original class names again.
package retrace

import (
	"bufio"
	"io"
	"regexp"
	"sort"
	"strings"
)

// ParseMapping reads a "original -> obfuscated" mapping (as written by
// engine.WriteMappingFile) and returns the reverse map obfuscated -> original.
func ParseMapping(r io.Reader) map[string]string {
	rev := make(map[string]string)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " -> ", 2)
		if len(parts) != 2 {
			continue
		}
		orig, obf := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if obf != "" && orig != "" {
			rev[obf] = orig
		}
	}
	return rev
}

// Apply rewrites every obfuscated class token in input back to its original.
// Tokens are matched at word boundaries so "o.a" does not match inside "o.ab".
func Apply(reverse map[string]string, input string) string {
	if len(reverse) == 0 {
		return input
	}
	// Replace longer obfuscated names first to avoid partial overlaps.
	obfs := make([]string, 0, len(reverse))
	for o := range reverse {
		obfs = append(obfs, o)
	}
	sort.Slice(obfs, func(i, j int) bool { return len(obfs[i]) > len(obfs[j]) })

	out := input
	for _, obf := range obfs {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(obf) + `\b`)
		out = re.ReplaceAllString(out, reverse[obf])
	}
	return out
}
