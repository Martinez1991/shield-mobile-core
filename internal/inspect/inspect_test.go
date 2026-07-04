package inspect

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildMachO synthesizes a minimal thin arm64 Mach-O with a __TEXT/__cstring
// section holding the given NUL-terminated strings (mirrors internal/ios test).
func buildMachO(cstrings []string) []byte {
	var data []byte
	for _, s := range cstrings {
		data = append(data, []byte(s)...)
		data = append(data, 0)
	}
	var b bytes.Buffer
	le := binary.LittleEndian
	w32 := func(v uint32) { _ = binary.Write(&b, le, v) }
	w64 := func(v uint64) { _ = binary.Write(&b, le, v) }
	name16 := func(s string) { var n [16]byte; copy(n[:], s); b.Write(n[:]) }

	const (
		hdrLen  = 32
		segLen  = 72
		secLen  = 80
		cmdSize = segLen + secLen
	)
	dataOff := uint32(hdrLen + cmdSize)
	total := uint64(dataOff) + uint64(len(data))

	w32(0xfeedfacf) // magic
	w32(0x0100000c) // cputype arm64
	w32(0)          // cpusubtype
	w32(2)          // filetype MH_EXECUTE
	w32(1)          // ncmds
	w32(cmdSize)    // sizeofcmds
	w32(0)          // flags
	w32(0)          // reserved

	w32(0x19)    // LC_SEGMENT_64
	w32(cmdSize) //
	name16("__TEXT")
	w64(0)
	w64(total)
	w64(0)
	w64(total)
	w32(7)
	w32(5)
	w32(1) // nsects
	w32(0)

	name16("__cstring")
	name16("__TEXT")
	w64(0)
	w64(uint64(len(data)))
	w32(dataOff)
	w32(0)
	w32(0)
	w32(0)
	w32(2) // S_CSTRING_LITERALS
	w32(0)
	w32(0)
	w32(0)

	b.Write(data)
	return b.Bytes()
}

// buildELF synthesizes a minimal valid ELF64 arm64 ET_DYN with a .rodata section
// holding the given strings (mirrors internal/native test).
func buildELF(strs []string) []byte {
	var rodata []byte
	for _, s := range strs {
		rodata = append(rodata, []byte(s)...)
		rodata = append(rodata, 0)
	}
	shstr := append([]byte{0}, []byte(".rodata\x00.shstrtab\x00")...)
	const (
		ehSize   = 64
		shEnt    = 64
		nameRoda = 1
		nameShst = 9
	)
	rodataOff := uint64(ehSize)
	shstrOff := rodataOff + uint64(len(rodata))
	shoff := shstrOff + uint64(len(shstr))

	le := binary.LittleEndian
	var b bytes.Buffer
	w16 := func(v uint16) { _ = binary.Write(&b, le, v) }
	w32 := func(v uint32) { _ = binary.Write(&b, le, v) }
	w64 := func(v uint64) { _ = binary.Write(&b, le, v) }

	b.Write([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	w16(3)      // ET_DYN
	w16(183)    // EM_AARCH64
	w32(1)      //
	w64(0)      //
	w64(0)      //
	w64(shoff)  //
	w32(0)      //
	w16(ehSize) //
	w16(0)      //
	w16(0)      //
	w16(shEnt)  //
	w16(3)      // e_shnum
	w16(2)      // e_shstrndx

	b.Write(rodata)
	b.Write(shstr)

	sh := func(name, typ uint32, flags, off, size uint64) {
		w32(name)
		w32(typ)
		w64(flags)
		w64(0)
		w64(off)
		w64(size)
		w32(0)
		w32(0)
		w64(1)
		w64(0)
	}
	sh(0, 0, 0, 0, 0)
	sh(nameRoda, 1, 2, rodataOff, uint64(len(rodata)))
	sh(nameShst, 3, 0, shstrOff, uint64(len(shstr)))
	return b.Bytes()
}

func writeZip(t *testing.T, name string, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for n, data := range entries {
		w, _ := zw.Create(n)
		_, _ = w.Write(data)
	}
	zw.Close()
	f.Close()
	return path
}

func TestAnalyzeIPA(t *testing.T) {
	path := writeZip(t, "app.ipa", map[string][]byte{
		"Payload/Foo.app/Foo":        buildMachO([]string{"AKIAIOSFODNN7EXAMPLE", "hello"}),
		"Payload/Foo.app/Info.plist": []byte("plist"),
	})
	rep, err := Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Kind != "ipa" {
		t.Errorf("Kind = %q, want ipa", rep.Kind)
	}
	if len(rep.Binaries) != 1 {
		t.Fatalf("Binaries = %d, want 1", len(rep.Binaries))
	}
	b := rep.Binaries[0]
	if b.Entry != "Payload/Foo.app/Foo" || b.Arch != "arm64" {
		t.Errorf("binary = %+v", b)
	}
	if b.SecretStrings != 1 {
		t.Errorf("SecretStrings = %d, want 1 (the AWS key)", b.SecretStrings)
	}
}

func TestAnalyzeAPK(t *testing.T) {
	path := writeZip(t, "app.apk", map[string][]byte{
		"lib/arm64-v8a/libfoo.so": buildELF([]string{"AKIAIOSFODNN7EXAMPLE"}),
		"classes.dex":             []byte("dex"),
	})
	rep, err := Analyze(path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Kind != "apk" {
		t.Errorf("Kind = %q, want apk", rep.Kind)
	}
	if len(rep.Binaries) != 1 {
		t.Fatalf("Binaries = %d, want 1", len(rep.Binaries))
	}
	b := rep.Binaries[0]
	if b.Entry != "lib/arm64-v8a/libfoo.so" || b.Arch != "arm64" || b.Type != "shared" {
		t.Errorf("binary = %+v", b)
	}
	if b.SecretStrings != 1 {
		t.Errorf("SecretStrings = %d, want 1", b.SecretStrings)
	}
}

func TestAnalyzeUnsupported(t *testing.T) {
	path := writeZip(t, "thing.zip", map[string][]byte{"a.txt": []byte("x")})
	if _, err := Analyze(path); err == nil {
		t.Error("expected error for a non-artifact zip")
	}
}
