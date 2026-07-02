// Package smali is SHIELD's editable intermediate representation.
//
// A full DEX obfuscator lifts Dalvik bytecode into an IR (SHIELD-IR, see
// shield-platform.md sections 2.2 and 4). To stay toolchain-light and fully
// testable offline, this MVP treats the *smali* text emitted by baksmali/apktool
// as that IR: each .smali file is one class, transformations rewrite the lines.
package smali

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Class is one loaded .smali file.
type Class struct {
	// Path is the absolute path on disk.
	Path string
	// Base is the smali source root the class lives under (smali/,
	// smali_classes2/, ... or the project root). Descriptor path is relative to it.
	Base string
	// Descriptor is the Dalvik type descriptor, e.g. "Lcom/bank/pay/CardValidator;".
	Descriptor string
	// Lines is the file content split by \n (no trailing newlines kept per line).
	Lines []string
}

var classDirRE = regexp.MustCompile(`^smali(_classes\d+)?$`)

// classLineRE matches `.class <access flags> Lcom/foo/Bar;`.
var classLineRE = regexp.MustCompile(`^\s*\.class\b.*\s(L[^;]+;)\s*$`)

// LoadProject discovers every smali source root under root and loads all classes.
// A "source root" is a directory named smali or smali_classesN (apktool layout);
// if none exist, root itself is treated as a single source root (raw smali dir).
func LoadProject(root string) ([]*Class, error) {
	bases := discoverBases(root)
	var classes []*Class
	for _, base := range bases {
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".smali") {
				return nil
			}
			c, err := loadClass(path, base)
			if err != nil {
				return err
			}
			classes = append(classes, c)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return classes, nil
}

func discoverBases(root string) []string {
	var bases []string
	entries, err := os.ReadDir(root)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && classDirRE.MatchString(e.Name()) {
				bases = append(bases, filepath.Join(root, e.Name()))
			}
		}
	}
	if len(bases) == 0 {
		bases = []string{root}
	}
	return bases
}

func loadClass(path, base string) (*Class, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	desc := ""
	for _, ln := range lines {
		if m := classLineRE.FindStringSubmatch(ln); m != nil {
			desc = m[1]
			break
		}
	}
	return &Class{Path: path, Base: base, Descriptor: desc, Lines: lines}, nil
}

// Save writes the class back to disk at the given absolute path (creating dirs).
func (c *Class) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := strings.Join(c.Lines, "\n")
	return os.WriteFile(path, []byte(body), 0o644)
}

// ---- Descriptor helpers -------------------------------------------------

// DescToDotted converts "Lcom/foo/Bar;" -> "com.foo.Bar".
func DescToDotted(desc string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(desc, "L"), ";")
	return strings.ReplaceAll(inner, "/", ".")
}

// DescToRelPath converts "Lcom/foo/Bar;" -> "com/foo/Bar.smali" (OS-specific sep).
func DescToRelPath(desc string) string {
	inner := strings.TrimSuffix(strings.TrimPrefix(desc, "L"), ";")
	return filepath.FromSlash(inner) + ".smali"
}

// InternalName converts "Lcom/foo/Bar;" -> "com/foo/Bar".
func InternalName(desc string) string {
	return strings.TrimSuffix(strings.TrimPrefix(desc, "L"), ";")
}
