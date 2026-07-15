// Package inspect exposes the binary-analysis foundation (issue #87) as a single
// user-facing report over the three artifact formats: IPA (Mach-O, #75) and
// APK/AAB (ELF .so, #81). It only orchestrates internal/ios and internal/native,
// both of which are stdlib-only (debug/macho, debug/elf) — so this stays
// zero-dependency, deterministic and offline-testable. Read-only: no transforms.
package inspect

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/apk"
	"github.com/Martinez1991/shield-mobile-core/internal/ios"
	"github.com/Martinez1991/shield-mobile-core/internal/native"
)

// Binary is one inspected binary (a Mach-O arch slice or an ELF .so), flattened
// to a format-neutral shape so the CLI can render every artifact uniformly.
type Binary struct {
	Entry         string `json:"entry"`         // zip entry / bundle path
	Arch          string `json:"arch"`          // arm64, x86_64, ...
	Type          string `json:"type"`          // shared, executable, ...
	Sections      int    `json:"sections"`      //
	Symbols       int    `json:"symbols"`       //
	SecretStrings int    `json:"secretStrings"` // secret-looking __cstring/.rodata literals
}

// Report is the whole-artifact binary inventory.
type Report struct {
	Path     string   `json:"path"`
	Kind     string   `json:"kind"` // "ipa" | "apk" | "aab"
	Binaries []Binary `json:"binaries"`
}

// Analyze detects the artifact format and inspects every binary it contains.
// IPA → each Mach-O arch slice of the app binary and embedded frameworks;
// APK/AAB → each lib/<abi>/*.so. Unsupported or unreadable input errors.
func Analyze(path string) (*Report, error) {
	switch {
	case ios.IsIPA(path):
		return analyzeIPA(path)
	case apk.IsAAB(path):
		return analyzeArchive(path, "aab")
	case isAPK(path):
		return analyzeArchive(path, "apk")
	default:
		return nil, fmt.Errorf("unsupported artifact %q: expected an .ipa, .apk or .aab", path)
	}
}

func analyzeIPA(path string) (*Report, error) {
	inspected, err := ios.InspectIPA(path)
	if err != nil {
		return nil, err
	}
	rep := &Report{Path: path, Kind: "ipa"}
	for _, entry := range sortedKeys(inspected) {
		for _, m := range inspected[entry] {
			rep.Binaries = append(rep.Binaries, Binary{
				Entry:         entry,
				Arch:          m.Arch,
				Type:          m.Type,
				Sections:      len(m.Sections),
				Symbols:       m.NumSymbols,
				SecretStrings: m.SecretStrings,
			})
		}
	}
	return rep, nil
}

func analyzeArchive(path, kind string) (*Report, error) {
	inspected, err := native.InspectArchive(path)
	if err != nil {
		return nil, err
	}
	rep := &Report{Path: path, Kind: kind}
	for _, entry := range native.SortedLibs(inspected) {
		e := inspected[entry]
		rep.Binaries = append(rep.Binaries, Binary{
			Entry:         entry,
			Arch:          e.Machine,
			Type:          e.Type,
			Sections:      len(e.Sections),
			Symbols:       e.NumSymbols,
			SecretStrings: e.SecretStrings,
		})
	}
	return rep, nil
}

// isAPK recognizes an APK by extension (content is confirmed downstream by the
// ELF inspector, which simply skips non-native entries).
func isAPK(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".apk")
}

func sortedKeys(m map[string][]ios.MachOInfo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
