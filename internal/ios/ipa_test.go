package ios

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeZip(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func readZip(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = b
	}
	return out
}

func sampleIPA(t *testing.T, path string) {
	writeZip(t, path, map[string][]byte{
		"Payload/Foo.app/Foo":                          []byte("MACH-O-MAIN"),
		"Payload/Foo.app/Info.plist":                   []byte("bplist00..."),
		"Payload/Foo.app/Frameworks/Bar.framework/Bar": []byte("MACH-O-FW"),
		"Payload/Foo.app/Assets.car":                   []byte("assets"),
	})
}

func TestIsIPA(t *testing.T) {
	dir := t.TempDir()

	ipa := filepath.Join(dir, "app.ipa")
	sampleIPA(t, ipa)
	if !IsIPA(ipa) {
		t.Error(".ipa extension should be detected")
	}

	// A .zip with a Payload/<App>.app/ bundle is an IPA.
	bundled := filepath.Join(dir, "bundle.zip")
	sampleIPA(t, bundled)
	if !IsIPA(bundled) {
		t.Error("Payload/*.app bundle should be detected")
	}

	plain := filepath.Join(dir, "plain.zip")
	writeZip(t, plain, map[string][]byte{"foo.txt": {1}})
	if IsIPA(plain) {
		t.Error("plain zip must not be detected as IPA")
	}
}

func TestFindBundle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.ipa")
	sampleIPA(t, path)
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	b, ok := FindBundle(&zr.Reader)
	if !ok {
		t.Fatal("bundle not found")
	}
	if b.AppName != "Foo" {
		t.Errorf("AppName = %q, want Foo", b.AppName)
	}
	if b.MainBinary != "Payload/Foo.app/Foo" {
		t.Errorf("MainBinary = %q", b.MainBinary)
	}
	if len(b.Frameworks) != 1 || b.Frameworks[0] != "Payload/Foo.app/Frameworks/Bar.framework/Bar" {
		t.Errorf("Frameworks = %v", b.Frameworks)
	}
}

func TestFindBundleNoMainBinary(t *testing.T) {
	// a .app dir with no executable at the conventional path -> not a valid bundle.
	path := filepath.Join(t.TempDir(), "broken.ipa")
	writeZip(t, path, map[string][]byte{"Payload/Foo.app/Info.plist": []byte("x")})
	zr, _ := zip.OpenReader(path)
	defer zr.Close()
	if _, ok := FindBundle(&zr.Reader); ok {
		t.Error("bundle without a main binary must not be found")
	}
}

func TestRewriteIPAPreservesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.ipa")
	out := filepath.Join(dir, "out.ipa")
	sampleIPA(t, in)

	newBin := []byte("PROTECTED-MACH-O")
	if err := RewriteIPA(in, out, map[string][]byte{"Payload/Foo.app/Foo": newBin}); err != nil {
		t.Fatal(err)
	}
	got := readZip(t, out)
	if !bytes.Equal(got["Payload/Foo.app/Foo"], newBin) {
		t.Error("main binary was not replaced")
	}
	// everything else byte-identical.
	if !bytes.Equal(got["Payload/Foo.app/Info.plist"], []byte("bplist00...")) {
		t.Error("Info.plist was altered")
	}
	if !bytes.Equal(got["Payload/Foo.app/Assets.car"], []byte("assets")) {
		t.Error("Assets.car was altered")
	}
	if !bytes.Equal(got["Payload/Foo.app/Frameworks/Bar.framework/Bar"], []byte("MACH-O-FW")) {
		t.Error("framework binary was altered")
	}
}
