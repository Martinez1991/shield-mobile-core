package apk

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// writeZip builds a zip file at path from name->content entries.
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
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = b
	}
	return out
}

func TestIsAAB(t *testing.T) {
	dir := t.TempDir()

	// .aab extension is enough.
	aab := filepath.Join(dir, "app.aab")
	writeZip(t, aab, map[string][]byte{"base/dex/classes.dex": {1}})
	if !IsAAB(aab) {
		t.Error(".aab extension should be detected")
	}

	// A .zip with the BundleConfig.pb marker is an AAB.
	marked := filepath.Join(dir, "bundle.zip")
	writeZip(t, marked, map[string][]byte{"BundleConfig.pb": {0}, "base/dex/classes.dex": {1}})
	if !IsAAB(marked) {
		t.Error("BundleConfig.pb marker should be detected")
	}

	// A plain zip without the marker is not an AAB.
	plain := filepath.Join(dir, "plain.zip")
	writeZip(t, plain, map[string][]byte{"foo.txt": {1}})
	if IsAAB(plain) {
		t.Error("plain zip must not be detected as AAB")
	}
}

func TestModuleOfDex(t *testing.T) {
	cases := []struct {
		name string
		mod  string
		ok   bool
	}{
		{"base/dex/classes.dex", "base", true},
		{"base/dex/classes2.dex", "base", true},
		{"feature1/dex/classes.dex", "feature1", true},
		{"./base/dex/classes.dex", "base", true},
		{"base/manifest/AndroidManifest.xml", "", false},
		{"base/dex/notclasses.dex", "", false},
		{"BundleConfig.pb", "", false},
		{"base/dex/sub/classes.dex", "", false},
	}
	for _, c := range cases {
		mod, ok := moduleOfDex(c.name)
		if ok != c.ok || mod != c.mod {
			t.Errorf("moduleOfDex(%q) = (%q,%v), want (%q,%v)", c.name, mod, ok, c.mod, c.ok)
		}
	}
}

func TestRewriteAABPreservesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.aab")
	out := filepath.Join(dir, "out.aab")

	manifest := []byte("protobuf-manifest-bytes\x00\x01\x02")
	config := []byte("bundle-config")
	origDex := []byte("OLD-DEX")
	origDex2 := []byte("OLD-DEX-2")
	writeZip(t, in, map[string][]byte{
		"BundleConfig.pb":                   config,
		"base/manifest/AndroidManifest.xml": manifest,
		"base/dex/classes.dex":              origDex,
		"base/dex/classes2.dex":             origDex2,
		"base/resources.pb":                 []byte("resources"),
	})

	newDex := []byte("NEW-PROTECTED-DEX")
	sub := map[string][]byte{
		"base/dex/classes.dex":  newDex, // replace
		"base/dex/classes2.dex": nil,    // drop (consolidated)
	}
	if err := rewriteAAB(in, out, sub); err != nil {
		t.Fatal(err)
	}

	got := readZip(t, out)
	// Untouched entries preserved byte-for-byte.
	if !bytes.Equal(got["BundleConfig.pb"], config) {
		t.Error("BundleConfig.pb was altered")
	}
	if !bytes.Equal(got["base/manifest/AndroidManifest.xml"], manifest) {
		t.Error("protobuf manifest was altered")
	}
	if !bytes.Equal(got["base/resources.pb"], []byte("resources")) {
		t.Error("resources.pb was altered")
	}
	// DEX replaced and the extra one dropped.
	if !bytes.Equal(got["base/dex/classes.dex"], newDex) {
		t.Errorf("classes.dex = %q, want the new dex", got["base/dex/classes.dex"])
	}
	if _, ok := got["base/dex/classes2.dex"]; ok {
		t.Error("classes2.dex should have been dropped")
	}
}
