package ios

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"fmt"
	"io"

	"github.com/Martinez1991/shield-mobile-core/internal/analyze"
)

// Mach-O inspection (issue #75). Read-only analysis over the standard library's
// debug/macho — segments, sections, symbols, architecture, and sensitive-string
// density in __TEXT/__cstring — the model the iOS transforms (metadata strip,
// string obfuscation) will build on. Stdlib only, so it stays zero-dependency.

// MachOInfo is the inspection result for one architecture slice.
type MachOInfo struct {
	Arch          string        `json:"arch"`
	Type          string        `json:"type"`
	Segments      []string      `json:"segments"`
	Sections      []SectionInfo `json:"sections"`
	NumSymbols    int           `json:"numSymbols"`
	SecretStrings int           `json:"secretStrings"` // secret-looking __cstring literals
}

// SectionInfo is one section's identity and size.
type SectionInfo struct {
	Name    string `json:"name"`
	Segment string `json:"segment"`
	Size    uint64 `json:"size"`
}

// Inspect parses a Mach-O binary (thin or fat/universal) and returns one
// MachOInfo per architecture. Malformed input yields an error, never a panic.
func Inspect(bin []byte) ([]MachOInfo, error) {
	r := bytes.NewReader(bin)
	if ff, err := macho.NewFatFile(r); err == nil {
		out := make([]MachOInfo, 0, len(ff.Arches))
		for _, a := range ff.Arches {
			out = append(out, inspectFile(a.File))
		}
		return out, nil
	}
	f, err := macho.NewFile(r)
	if err != nil {
		return nil, fmt.Errorf("not a Mach-O: %w", err)
	}
	return []MachOInfo{inspectFile(f)}, nil
}

// InspectIPA opens an IPA, locates its app binary and embedded frameworks, and
// inspects each Mach-O. Keys are zip entry names; unreadable/non-Mach-O entries
// are skipped.
func InspectIPA(path string) (map[string][]MachOInfo, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	b, ok := FindBundle(&zr.Reader)
	if !ok {
		return nil, fmt.Errorf("no app bundle in %s", path)
	}
	out := map[string][]MachOInfo{}
	for _, name := range append([]string{b.MainBinary}, b.Frameworks...) {
		data, err := readEntry(&zr.Reader, name)
		if err != nil {
			continue
		}
		if info, err := Inspect(data); err == nil {
			out[name] = info
		}
	}
	return out, nil
}

func readEntry(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("entry %q not found", name)
}

func inspectFile(f *macho.File) MachOInfo {
	mi := MachOInfo{Arch: cpuName(f.Cpu), Type: typeName(f.Type)}
	for _, l := range f.Loads {
		if s, ok := l.(*macho.Segment); ok {
			mi.Segments = append(mi.Segments, s.Name)
		}
	}
	for _, sec := range f.Sections {
		mi.Sections = append(mi.Sections, SectionInfo{Name: sec.Name, Segment: sec.Seg, Size: sec.Size})
		if sec.Seg == "__TEXT" && sec.Name == "__cstring" {
			if data, err := sec.Data(); err == nil {
				for _, lit := range splitCStrings(data) {
					if analyze.LooksSecret(lit) {
						mi.SecretStrings++
					}
				}
			}
		}
	}
	if f.Symtab != nil {
		mi.NumSymbols = len(f.Symtab.Syms)
	}
	return mi
}

// splitCStrings splits a __cstring blob (NUL-terminated literals) into strings,
// dropping tiny fragments.
func splitCStrings(data []byte) []string {
	var out []string
	for _, part := range bytes.Split(data, []byte{0}) {
		if len(part) >= 4 {
			out = append(out, string(part))
		}
	}
	return out
}

func cpuName(c macho.Cpu) string {
	switch c {
	case macho.Cpu386:
		return "i386"
	case macho.CpuAmd64:
		return "x86_64"
	case macho.CpuArm:
		return "arm"
	case macho.CpuArm64:
		return "arm64"
	case macho.CpuPpc:
		return "ppc"
	case macho.CpuPpc64:
		return "ppc64"
	}
	return fmt.Sprintf("cpu(%#x)", uint32(c))
}

func typeName(t macho.Type) string {
	switch t {
	case macho.TypeObj:
		return "object"
	case macho.TypeExec:
		return "executable"
	case macho.TypeDylib:
		return "dylib"
	case macho.TypeBundle:
		return "bundle"
	}
	return fmt.Sprintf("type(%#x)", uint32(t))
}
