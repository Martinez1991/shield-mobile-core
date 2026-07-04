// Package ios handles the iOS artifact format — the .ipa (issue #63/#74). An IPA
// is a plain zip whose app bundle lives under Payload/<App>.app/, with the main
// Mach-O executable and any embedded frameworks inside. This package locates
// those binaries and repacks the bundle, copying every other entry byte-for-byte
// — the same approach as the AAB round-trip. The Mach-O protection itself is a
// follow-up (#75+); this is the packaging foundation.
//
// Everything here is pure Go / stdlib, so it stays deterministic and
// offline-testable.
package ios

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Bundle describes the located parts of an IPA's app bundle. Names are zip entry
// paths.
type Bundle struct {
	AppName    string   // e.g. "Foo" (from Payload/Foo.app)
	MainBinary string   // "Payload/Foo.app/Foo"
	Frameworks []string // "Payload/Foo.app/Frameworks/Bar.framework/Bar"
}

// IsIPA reports whether path is an iOS App Store package: a .ipa by extension, or
// a zip that contains a Payload/<App>.app/ bundle.
func IsIPA(path string) bool {
	if strings.EqualFold(filepath.Ext(path), ".ipa") {
		return true
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer zr.Close()
	for _, f := range zr.File {
		if _, ok := appName(f.Name); ok {
			return true
		}
	}
	return false
}

// appName extracts the app bundle name from a "Payload/<App>.app/..." entry.
func appName(name string) (string, bool) {
	name = strings.TrimPrefix(name, "./")
	parts := strings.Split(name, "/")
	if len(parts) >= 2 && parts[0] == "Payload" && strings.HasSuffix(parts[1], ".app") {
		return strings.TrimSuffix(parts[1], ".app"), true
	}
	return "", false
}

// FindBundle locates the main app binary (by the CFBundleExecutable convention
// Payload/<App>.app/<App>) and any embedded framework binaries.
func FindBundle(zr *zip.Reader) (Bundle, bool) {
	app := ""
	present := map[string]bool{}
	for _, f := range zr.File {
		present[strings.TrimPrefix(f.Name, "./")] = true
		if a, ok := appName(f.Name); ok && app == "" {
			app = a
		}
	}
	if app == "" {
		return Bundle{}, false
	}
	b := Bundle{AppName: app, MainBinary: "Payload/" + app + ".app/" + app}
	if !present[b.MainBinary] {
		return Bundle{}, false // no main executable at the conventional path
	}

	fwPrefix := "Payload/" + app + ".app/Frameworks/"
	for name := range present {
		rest, ok := strings.CutPrefix(name, fwPrefix)
		if !ok {
			continue
		}
		// Frameworks/<Fw>.framework/<Fw> — the framework's own Mach-O.
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && strings.HasSuffix(parts[0], ".framework") &&
			parts[1] == strings.TrimSuffix(parts[0], ".framework") {
			b.Frameworks = append(b.Frameworks, name)
		}
	}
	sort.Strings(b.Frameworks)
	return b, true
}

// RewriteIPA copies inPath to outPath, replacing entries named in sub with the
// given bytes (nil drops the entry). Untouched entries are copied raw, preserving
// their compression and metadata exactly.
func RewriteIPA(inPath, outPath string, sub map[string][]byte) error {
	zr, err := zip.OpenReader(inPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)

	for _, f := range zr.File {
		if content, replaced := sub[f.Name]; replaced {
			if content == nil {
				continue
			}
			hdr := f.FileHeader
			hdr.Method = zip.Deflate
			hdr.CompressedSize64, hdr.CRC32 = 0, 0
			w, err := zw.CreateHeader(&hdr)
			if err != nil {
				zw.Close()
				return err
			}
			if _, err := w.Write(content); err != nil {
				zw.Close()
				return err
			}
			continue
		}
		w, err := zw.CreateRaw(&f.FileHeader)
		if err != nil {
			zw.Close()
			return err
		}
		rc, err := f.OpenRaw()
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			zw.Close()
			return err
		}
	}
	return zw.Close()
}
