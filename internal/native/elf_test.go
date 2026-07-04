package native

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildELF synthesizes a minimal but valid ELF64 (arm64, ET_DYN) with a .rodata
// section holding the given NUL-terminated strings, plus a .shstrtab.
func buildELF(strs []string) []byte {
	var rodata []byte
	for _, s := range strs {
		rodata = append(rodata, []byte(s)...)
		rodata = append(rodata, 0)
	}
	// section header string table: "\0.rodata\0.shstrtab\0"
	shstr := append([]byte{0}, []byte(".rodata\x00.shstrtab\x00")...)
	const (
		ehSize   = 64
		shEnt    = 64
		nameRoda = 1 // offset of ".rodata" in shstr
		nameShst = 9 // offset of ".shstrtab"
	)
	rodataOff := uint64(ehSize)
	shstrOff := rodataOff + uint64(len(rodata))
	shoff := shstrOff + uint64(len(shstr))

	le := binary.LittleEndian
	var b bytes.Buffer
	w16 := func(v uint16) { _ = binary.Write(&b, le, v) }
	w32 := func(v uint32) { _ = binary.Write(&b, le, v) }
	w64 := func(v uint64) { _ = binary.Write(&b, le, v) }

	// ELF header
	b.Write([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // e_ident
	w16(3)                                                                   // e_type: ET_DYN
	w16(183)                                                                 // e_machine: EM_AARCH64
	w32(1)                                                                   // e_version
	w64(0)                                                                   // e_entry
	w64(0)                                                                   // e_phoff
	w64(shoff)                                                               // e_shoff
	w32(0)                                                                   // e_flags
	w16(ehSize)                                                              // e_ehsize
	w16(0)                                                                   // e_phentsize
	w16(0)                                                                   // e_phnum
	w16(shEnt)                                                               // e_shentsize
	w16(3)                                                                   // e_shnum
	w16(2)                                                                   // e_shstrndx

	b.Write(rodata)
	b.Write(shstr)

	sh := func(name, typ uint32, flags, off, size uint64) {
		w32(name)
		w32(typ)
		w64(flags)
		w64(0) // sh_addr
		w64(off)
		w64(size)
		w32(0) // sh_link
		w32(0) // sh_info
		w64(1) // sh_addralign
		w64(0) // sh_entsize
	}
	sh(0, 0, 0, 0, 0)                                  // [0] SHT_NULL
	sh(nameRoda, 1, 2, rodataOff, uint64(len(rodata))) // [1] .rodata SHT_PROGBITS SHF_ALLOC
	sh(nameShst, 3, 0, shstrOff, uint64(len(shstr)))   // [2] .shstrtab SHT_STRTAB
	return b.Bytes()
}

func TestInspectELF(t *testing.T) {
	bin := buildELF([]string{"AKIAIOSFODNN7EXAMPLE", "just a message"})
	info, err := Inspect(bin)
	if err != nil {
		t.Fatal(err)
	}
	if info.Machine != "arm64" {
		t.Errorf("Machine = %q, want arm64", info.Machine)
	}
	if info.Type != "shared" {
		t.Errorf("Type = %q, want shared", info.Type)
	}
	var roda *SectionInfo
	for i := range info.Sections {
		if info.Sections[i].Name == ".rodata" {
			roda = &info.Sections[i]
		}
	}
	if roda == nil {
		t.Fatalf(".rodata not found: %+v", info.Sections)
	}
	// one secret (the AWS access key), not the plain message.
	if info.SecretStrings != 1 {
		t.Errorf("SecretStrings = %d, want 1", info.SecretStrings)
	}
}

func TestInspectELFMalformedNoPanic(t *testing.T) {
	for _, in := range [][]byte{nil, []byte("garbage"), {0x7f, 'E', 'L', 'F'}} {
		if _, err := Inspect(in); err == nil {
			t.Errorf("expected error for %d-byte non-ELF", len(in))
		}
	}
}

func TestNativeLib(t *testing.T) {
	cases := []struct {
		name, abi string
		ok        bool
	}{
		{"lib/arm64-v8a/libfoo.so", "arm64-v8a", true},
		{"base/lib/x86_64/libbar.so", "x86_64", true},
		{"./lib/armeabi-v7a/libx.so", "armeabi-v7a", true},
		{"classes.dex", "", false},
		{"lib/arm64-v8a/notlib.txt", "", false},
		{"res/lib.xml", "", false},
	}
	for _, c := range cases {
		abi, ok := NativeLib(c.name)
		if ok != c.ok || abi != c.abi {
			t.Errorf("NativeLib(%q) = (%q,%v), want (%q,%v)", c.name, abi, ok, c.abi, c.ok)
		}
	}
}

func TestInspectArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.apk")
	f, _ := os.Create(path)
	zw := zip.NewWriter(f)
	add := func(name string, data []byte) {
		w, _ := zw.Create(name)
		_, _ = w.Write(data)
	}
	add("lib/arm64-v8a/libfoo.so", buildELF([]string{"AKIAIOSFODNN7EXAMPLE"}))
	add("classes.dex", []byte("dex"))
	zw.Close()
	f.Close()

	got, err := InspectArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	libs := SortedLibs(got)
	if len(libs) != 1 || libs[0] != "lib/arm64-v8a/libfoo.so" {
		t.Fatalf("libs = %v, want the one .so", libs)
	}
	if got[libs[0]].Machine != "arm64" || got[libs[0]].SecretStrings != 1 {
		t.Errorf("inspected lib = %+v", got[libs[0]])
	}
}
