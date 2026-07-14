package nativesvc

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// makeAPK writes a zip with the given entries and returns its path.
func makeAPK(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.apk")
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

func readAPK(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		b, err := readZipEntry(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = b
	}
	return out
}

func TestProtectArchive(t *testing.T) {
	in := makeAPK(t, map[string][]byte{
		"lib/arm64-v8a/libsample.so.bc": []byte("BITCODE"),        // recompilable
		"lib/arm64-v8a/libsample.so":    []byte("OLD-SO"),         // stale, to be replaced
		"lib/x86_64/libother.so":        []byte("PLAIN-SO"),       // no sidecar -> skipped
		"classes.dex":                   []byte("DEX-DATA"),       // copied
		"res/values/strings.xml":        []byte("<resources/>\n"), // copied
	})

	var gotABI string
	var gotBitcode []byte
	run := func(exec string, args []string, stdin []byte) ([]byte, error) {
		return append([]byte("XF:"), stdin...), nil
	}
	s := newWithRunner(Config{Passes: []Pass{PassFlatten, PassStrings}, Seed: 1}, "svc", run)

	link := func(abi string, bc []byte) ([]byte, error) {
		gotABI, gotBitcode = abi, bc
		return append([]byte("SO<"), bc...), nil // synthetic linked object
	}

	out := filepath.Join(t.TempDir(), "out.apk")
	res, err := s.ProtectArchive(in, out, link, nil)
	if err != nil {
		t.Fatal(err)
	}

	// transform received the sidecar's ABI + bitcode.
	if gotABI != "arm64-v8a" || string(gotBitcode) != "XF:BITCODE" {
		t.Errorf("linker got abi=%q bc=%q", gotABI, gotBitcode)
	}
	// result classification.
	if len(res.Protected) != 1 || res.Protected[0] != "lib/arm64-v8a/libsample.so" {
		t.Errorf("Protected = %v", res.Protected)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "lib/x86_64/libother.so" {
		t.Errorf("Skipped = %v", res.Skipped)
	}

	got := readAPK(t, out)
	// .so replaced with linked(transformed(bitcode)); .bc dropped.
	if string(got["lib/arm64-v8a/libsample.so"]) != "SO<XF:BITCODE" {
		t.Errorf("protected .so = %q", got["lib/arm64-v8a/libsample.so"])
	}
	if _, present := got["lib/arm64-v8a/libsample.so.bc"]; present {
		t.Error("bitcode sidecar was not dropped")
	}
	// untouched entries preserved byte-for-byte.
	for _, name := range []string{"lib/x86_64/libother.so", "classes.dex", "res/values/strings.xml"} {
		if string(got[name]) != string(readAPK(t, in)[name]) {
			t.Errorf("entry %s not preserved", name)
		}
	}
}

func TestProtectArchiveTamperNeedsPatcher(t *testing.T) {
	in := makeAPK(t, map[string][]byte{"lib/arm64-v8a/x.so.bc": []byte("BC")})
	s := newWithRunner(Config{Passes: []Pass{PassTamper}}, "svc",
		func(string, []string, []byte) ([]byte, error) { return []byte("x"), nil })
	link := func(string, []byte) ([]byte, error) { return []byte("so"), nil }
	if _, err := s.ProtectArchive(in, filepath.Join(t.TempDir(), "o.apk"), link, nil); err == nil {
		t.Error("expected error: tamper pass without a Patcher")
	}
}

func TestProtectArchivePatchApplied(t *testing.T) {
	in := makeAPK(t, map[string][]byte{"lib/arm64-v8a/x.so.bc": []byte("BC")})
	s := newWithRunner(Config{Passes: []Pass{PassTamper}}, "svc",
		func(_ string, _ []string, in []byte) ([]byte, error) { return in, nil })
	link := func(_ string, bc []byte) ([]byte, error) { return append([]byte("SO:"), bc...), nil }
	patched := false
	patch := func(so []byte) ([]byte, error) { patched = true; return append(so, []byte(":PATCHED")...), nil }

	out := filepath.Join(t.TempDir(), "o.apk")
	if _, err := s.ProtectArchive(in, out, link, patch); err != nil {
		t.Fatal(err)
	}
	if !patched {
		t.Error("patcher not invoked for tamper pass")
	}
	if string(readAPK(t, out)["lib/arm64-v8a/x.so"]) != "SO:BC:PATCHED" {
		t.Errorf("patched .so = %q", readAPK(t, out)["lib/arm64-v8a/x.so"])
	}
}

func TestProtectArchiveNoBitcode(t *testing.T) {
	in := makeAPK(t, map[string][]byte{
		"lib/arm64-v8a/libplain.so": []byte("SO"),
		"classes.dex":               []byte("DEX"),
	})
	s := newWithRunner(Config{Passes: []Pass{PassFlatten}}, "svc",
		func(string, []string, []byte) ([]byte, error) { return []byte("x"), nil })
	out := filepath.Join(t.TempDir(), "o.apk")
	res, err := s.ProtectArchive(in, out, func(string, []byte) ([]byte, error) { return nil, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Protected) != 0 || len(res.Skipped) != 1 {
		t.Errorf("Protected=%v Skipped=%v", res.Protected, res.Skipped)
	}
	// output is a faithful copy.
	if string(readAPK(t, out)["classes.dex"]) != "DEX" {
		t.Error("copy-through failed")
	}
}
