package ios

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeIPA(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.ipa")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for n, d := range entries {
		w, _ := zw.Create(n)
		_, _ = w.Write(d)
	}
	zw.Close()
	f.Close()
	return path
}

func TestStripIPA(t *testing.T) {
	macho := buildMachO([]string{"hello"})
	in := writeIPA(t, map[string][]byte{
		"Payload/Foo.app/Foo":                          macho,
		"Payload/Foo.app/Frameworks/Bar.framework/Bar": macho,
		"Payload/Foo.app/Info.plist":                   []byte("PLIST"),
		"Payload/Foo.app/assets/logo.png":              []byte("PNGDATA"),
	})

	// Fakes: strip prepends a marker, sign appends one — both mutate the file.
	strip := func(p string) error {
		b, _ := os.ReadFile(p)
		return os.WriteFile(p, append([]byte("STRIP:"), b...), 0o600)
	}
	sign := func(p string) error {
		b, _ := os.ReadFile(p)
		return os.WriteFile(p, append(b, []byte(":SIGNED")...), 0o600)
	}

	out := filepath.Join(t.TempDir(), "out.ipa")
	res, err := StripIPA(in, out, strip, sign)
	if err != nil {
		t.Fatal(err)
	}

	// main binary + framework binary stripped (sorted).
	want := []string{"Payload/Foo.app/Foo", "Payload/Foo.app/Frameworks/Bar.framework/Bar"}
	if len(res.Stripped) != 2 || res.Stripped[0] != want[0] || res.Stripped[1] != want[1] {
		t.Fatalf("Stripped = %v, want %v", res.Stripped, want)
	}

	got := readIPA(t, out)
	// strip ran, then sign ran (STRIP:<macho>:SIGNED).
	exp := append(append([]byte("STRIP:"), macho...), []byte(":SIGNED")...)
	if string(got["Payload/Foo.app/Foo"]) != string(exp) {
		t.Errorf("main binary not strip+sign transformed")
	}
	// non-Mach-O entries preserved.
	if string(got["Payload/Foo.app/Info.plist"]) != "PLIST" || string(got["Payload/Foo.app/assets/logo.png"]) != "PNGDATA" {
		t.Error("non-binary entries not preserved")
	}
}

func TestStripIPAUnsigned(t *testing.T) {
	// nil Signer leaves the stripped binary as-is (unsigned).
	in := writeIPA(t, map[string][]byte{"Payload/Foo.app/Foo": buildMachO(nil)})
	strip := func(p string) error {
		b, _ := os.ReadFile(p)
		return os.WriteFile(p, append([]byte("S:"), b...), 0o600)
	}
	out := filepath.Join(t.TempDir(), "out.ipa")
	if _, err := StripIPA(in, out, strip, nil); err != nil {
		t.Fatal(err)
	}
	if string(readIPA(t, out)["Payload/Foo.app/Foo"][:2]) != "S:" {
		t.Error("strip not applied for unsigned flow")
	}
}

func TestStripIPARequiresStripper(t *testing.T) {
	in := writeIPA(t, map[string][]byte{"Payload/Foo.app/Foo": buildMachO(nil)})
	if _, err := StripIPA(in, filepath.Join(t.TempDir(), "o.ipa"), nil, nil); err == nil {
		t.Error("expected error with a nil Stripper")
	}
}

func readIPA(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		b, err := readEntry(&zr.Reader, f.Name)
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = b
	}
	return out
}
