// Package native inspects native shared libraries (issue #64/#81). On Android
// these are ELF .so files under lib/<abi>/ in the APK/AAB; here we parse them
// read-only with the standard library's debug/elf — sections, symbols,
// architecture, and sensitive-string density in .rodata — the model the native
// transforms (string obfuscation, LLVM passes) will build on. Stdlib only, so it
// stays zero-dependency and offline-testable, mirroring the Mach-O inspection.
package native

import (
	"archive/zip"
	"bytes"
	"debug/elf"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/analyze"
)

// ELFInfo is the inspection result for one ELF shared object.
type ELFInfo struct {
	Machine       string        `json:"machine"`
	Type          string        `json:"type"`
	Sections      []SectionInfo `json:"sections"`
	NumSymbols    int           `json:"numSymbols"`
	SecretStrings int           `json:"secretStrings"` // secret-looking .rodata literals
}

// SectionInfo is one section's name and size.
type SectionInfo struct {
	Name string `json:"name"`
	Size uint64 `json:"size"`
}

// Inspect parses an ELF shared object and reports its structure and the number
// of secret-looking .rodata strings. Malformed input errors, never panics.
func Inspect(bin []byte) (ELFInfo, error) {
	f, err := elf.NewFile(bytes.NewReader(bin))
	if err != nil {
		return ELFInfo{}, fmt.Errorf("not an ELF: %w", err)
	}
	defer f.Close()

	info := ELFInfo{Machine: machineName(f.Machine), Type: typeName(f.Type)}
	for _, s := range f.Sections {
		info.Sections = append(info.Sections, SectionInfo{Name: s.Name, Size: s.Size})
		if s.Name == ".rodata" {
			if data, err := s.Data(); err == nil {
				for _, lit := range splitStrings(data) {
					if analyze.LooksSecret(lit) {
						info.SecretStrings++
					}
				}
			}
		}
	}
	if syms, err := f.Symbols(); err == nil {
		info.NumSymbols += len(syms)
	}
	if dyn, err := f.DynamicSymbols(); err == nil {
		info.NumSymbols += len(dyn)
	}
	return info, nil
}

// NativeLib reports whether a zip entry is a native shared object and returns its
// ABI. Matches "lib/<abi>/x.so" (APK) and "<module>/lib/<abi>/x.so" (AAB).
func NativeLib(name string) (abi string, ok bool) {
	name = strings.TrimPrefix(name, "./")
	if !strings.HasSuffix(name, ".so") {
		return "", false
	}
	parts := strings.Split(name, "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "lib" {
			return parts[i+1], true // <abi>
		}
	}
	return "", false
}

// InspectArchive opens an APK/AAB zip and inspects every native .so it contains.
// Keys are zip entry names; unreadable/non-ELF entries are skipped.
func InspectArchive(path string) (map[string]ELFInfo, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	out := map[string]ELFInfo{}
	for _, f := range zr.File {
		if _, ok := NativeLib(f.Name); !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		if info, err := Inspect(data); err == nil {
			out[f.Name] = info
		}
	}
	return out, nil
}

// SortedLibs returns the native-lib entry names of a parsed archive, sorted.
func SortedLibs(inspected map[string]ELFInfo) []string {
	names := make([]string, 0, len(inspected))
	for n := range inspected {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func splitStrings(data []byte) []string {
	var out []string
	for _, part := range bytes.Split(data, []byte{0}) {
		if len(part) >= 4 {
			out = append(out, string(part))
		}
	}
	return out
}

func machineName(m elf.Machine) string {
	switch m {
	case elf.EM_AARCH64:
		return "arm64"
	case elf.EM_ARM:
		return "arm"
	case elf.EM_X86_64:
		return "x86_64"
	case elf.EM_386:
		return "x86"
	case elf.EM_RISCV:
		return "riscv"
	}
	return m.String()
}

func typeName(t elf.Type) string {
	switch t {
	case elf.ET_DYN:
		return "shared"
	case elf.ET_EXEC:
		return "executable"
	case elf.ET_REL:
		return "relocatable"
	}
	return t.String()
}
