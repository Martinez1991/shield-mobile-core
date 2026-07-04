package nativesvc

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParsePasses(t *testing.T) {
	got, err := ParsePasses([]string{"flatten", "mba", "opaque", "strings"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0] != PassFlatten || got[3] != PassStrings {
		t.Errorf("ParsePasses = %v", got)
	}
	if _, err := ParsePasses([]string{"flatten", "bogus"}); err == nil {
		t.Error("expected error for an unknown pass")
	}
}

func TestLocateUnavailable(t *testing.T) {
	// Force every discovery source to miss: explicit path, env, and PATH.
	t.Setenv("SHIELD_NATIVE_SVC", "")
	t.Setenv("PATH", t.TempDir())
	if _, err := Locate(Config{Exec: "definitely-not-a-real-binary-xyz"}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
	if _, err := New(Config{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("New err = %v, want ErrUnavailable", err)
	}
}

func TestApplyContract(t *testing.T) {
	// A fake runner asserts the subprocess contract (ADR 0004) and echoes a
	// transformed object, so Apply is exercised without the real toolchain.
	var gotExec string
	var gotArgs []string
	var gotStdin []byte
	fake := func(exec string, args []string, stdin []byte) ([]byte, error) {
		gotExec, gotArgs, gotStdin = exec, args, stdin
		return append([]byte("XF:"), stdin...), nil
	}
	s := newWithRunner(Config{Passes: []Pass{PassFlatten, PassMBA}, Seed: 42}, "/opt/native-svc", fake)

	out, err := s.Apply("arm64-v8a", []byte("OBJECT"))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "XF:OBJECT" {
		t.Errorf("out = %q", out)
	}
	if gotExec != "/opt/native-svc" || string(gotStdin) != "OBJECT" {
		t.Errorf("exec=%q stdin=%q", gotExec, gotStdin)
	}
	want := []string{"transform", "--arch", "arm64-v8a", "--seed", "42", "--pass", "flatten", "--pass", "mba"}
	if fmt.Sprint(gotArgs) != fmt.Sprint(want) {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
}

func TestApplyErrors(t *testing.T) {
	// No passes configured.
	s := newWithRunner(Config{}, "svc", func(string, []string, []byte) ([]byte, error) { return nil, nil })
	if _, err := s.Apply("arm64", []byte("x")); err == nil {
		t.Error("expected error with no passes")
	}
	// Runner failure propagates.
	boom := newWithRunner(Config{Passes: []Pass{PassFlatten}}, "svc",
		func(string, []string, []byte) ([]byte, error) { return nil, errors.New("miscompile") })
	if _, err := boom.Apply("arm64", []byte("x")); err == nil {
		t.Error("expected error when native-svc fails")
	}
	// Empty output is rejected.
	empty := newWithRunner(Config{Passes: []Pass{PassFlatten}}, "svc",
		func(string, []string, []byte) ([]byte, error) { return nil, nil })
	if _, err := empty.Apply("arm64", []byte("x")); err == nil {
		t.Error("expected error on empty object")
	}
}

func TestPlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.apk")
	f, _ := os.Create(path)
	zw := zip.NewWriter(f)
	add := func(name string, data []byte) { w, _ := zw.Create(name); _, _ = w.Write(data) }
	add("lib/arm64-v8a/libfoo.so", buildELF())
	add("lib/x86_64/libbar.so", buildELF())
	add("classes.dex", []byte("dex"))
	zw.Close()
	f.Close()

	cands, err := Plan(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates = %d, want 2: %+v", len(cands), cands)
	}
	// SortedLibs orders entries; arm64 lib sorts before x86_64.
	if cands[0].Entry != "lib/arm64-v8a/libfoo.so" || cands[0].ABI != "arm64-v8a" || cands[0].Arch != "arm64" {
		t.Errorf("candidate[0] = %+v", cands[0])
	}
}

// buildELF is a minimal valid ELF64 arm64 ET_DYN (no .rodata needed here).
func buildELF() []byte {
	shstr := append([]byte{0}, []byte(".shstrtab\x00")...)
	const ehSize, shEnt = 64, 64
	shstrOff := uint64(ehSize)
	shoff := shstrOff + uint64(len(shstr))
	le := binary.LittleEndian
	var b bytes.Buffer
	w16 := func(v uint16) { _ = binary.Write(&b, le, v) }
	w32 := func(v uint32) { _ = binary.Write(&b, le, v) }
	w64 := func(v uint64) { _ = binary.Write(&b, le, v) }
	b.Write([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	w16(3)   // ET_DYN
	w16(183) // EM_AARCH64
	w32(1)
	w64(0)
	w64(0)
	w64(shoff)
	w32(0)
	w16(ehSize)
	w16(0)
	w16(0)
	w16(shEnt)
	w16(2) // e_shnum
	w16(1) // e_shstrndx
	b.Write(shstr)
	sh := func(name, typ uint32, off, size uint64) {
		w32(name)
		w32(typ)
		w64(0)
		w64(0)
		w64(off)
		w64(size)
		w32(0)
		w32(0)
		w64(1)
		w64(0)
	}
	sh(0, 0, 0, 0)                         // SHT_NULL
	sh(1, 3, shstrOff, uint64(len(shstr))) // .shstrtab
	return b.Bytes()
}
