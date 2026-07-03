package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProj(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestKeyDeterministicAndSensitive(t *testing.T) {
	a := t.TempDir()
	writeProj(t, a, map[string]string{"smali/A.smali": "x", "smali/B.smali": "y"})

	k1, err := Key(a, []byte(`{"name":"p"}`), "1")
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := Key(a, []byte(`{"name":"p"}`), "1")
	if k1 != k2 {
		t.Fatal("key must be deterministic for identical input")
	}
	// different policy => different key
	if k3, _ := Key(a, []byte(`{"name":"q"}`), "1"); k3 == k1 {
		t.Error("policy change must change the key")
	}
	// different engine version => different key
	if k4, _ := Key(a, []byte(`{"name":"p"}`), "2"); k4 == k1 {
		t.Error("engine version change must change the key")
	}
	// different content => different key
	b := t.TempDir()
	writeProj(t, b, map[string]string{"smali/A.smali": "X", "smali/B.smali": "y"})
	if kb, _ := Key(b, []byte(`{"name":"p"}`), "1"); kb == k1 {
		t.Error("content change must change the key")
	}
}

func TestPutGetRoundTrip(t *testing.T) {
	src := t.TempDir()
	writeProj(t, src, map[string]string{"smali/A.smali": "protected-A", "mapping.txt": "m"})
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := "deadbeef"

	if _, ok := store.Get(key); ok {
		t.Fatal("expected miss before Put")
	}
	if err := store.Put(key, src, []byte(`{"policy":"p"}`)); err != nil {
		t.Fatal(err)
	}
	entry, ok := store.Get(key)
	if !ok {
		t.Fatal("expected hit after Put")
	}
	_ = entry

	// restored tree matches
	dst := t.TempDir()
	if err := CopyTree(store.OutDir(key), dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dst, "smali", "A.smali"))
	if err != nil || string(b) != "protected-A" {
		t.Fatalf("restored file wrong: %q err=%v", b, err)
	}
	rep, err := store.Report(key)
	if err != nil || string(rep) != `{"policy":"p"}` {
		t.Fatalf("report wrong: %q err=%v", rep, err)
	}
}
