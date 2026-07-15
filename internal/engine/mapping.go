package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// MapEntry records one original->obfuscated rename (dotted form), used to
// retrace stack traces in production (section 3.1).
type MapEntry struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func mappingFrom(m map[string]string) []MapEntry {
	entries := make([]MapEntry, 0, len(m))
	for old, nw := range m {
		entries = append(entries, MapEntry{
			From: smali.DescToDotted(old),
			To:   smali.DescToDotted(nw),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].From < entries[j].From })
	return entries
}

// WriteMappingFile writes a ProGuard-style mapping (`from -> to`).
func WriteMappingFile(path string, entries []MapEntry) error {
	var b strings.Builder
	b.WriteString("# SHIELD mapping file — keep secret; needed to retrace stack traces.\n")
	for _, e := range entries {
		b.WriteString(e.From)
		b.WriteString(" -> ")
		b.WriteString(e.To)
		b.WriteString("\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
