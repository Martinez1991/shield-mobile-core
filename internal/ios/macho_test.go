package ios

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"testing"
)

// buildMachO synthesizes a minimal but valid thin arm64 Mach-O: a mach_header_64
// + one LC_SEGMENT_64 (__TEXT) with one __cstring section whose data follows.
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
		cmdSize = segLen + secLen // 152
	)
	dataOff := uint32(hdrLen + cmdSize) // 184
	total := uint64(dataOff) + uint64(len(data))

	// mach_header_64
	w32(0xfeedfacf) // magic (64-bit LE)
	w32(0x0100000c) // cputype: arm64
	w32(0)          // cpusubtype
	w32(2)          // filetype: MH_EXECUTE
	w32(1)          // ncmds
	w32(cmdSize)    // sizeofcmds
	w32(0)          // flags
	w32(0)          // reserved

	// LC_SEGMENT_64 (__TEXT)
	w32(0x19)    // cmd: LC_SEGMENT_64
	w32(cmdSize) // cmdsize
	name16("__TEXT")
	w64(0)     // vmaddr
	w64(total) // vmsize
	w64(0)     // fileoff
	w64(total) // filesize
	w32(7)     // maxprot
	w32(5)     // initprot
	w32(1)     // nsects
	w32(0)     // flags

	// section_64 (__cstring)
	name16("__cstring")
	name16("__TEXT")
	w64(0)                 // addr
	w64(uint64(len(data))) // size
	w32(dataOff)           // offset
	w32(0)                 // align
	w32(0)                 // reloff
	w32(0)                 // nreloc
	w32(2)                 // flags: S_CSTRING_LITERALS
	w32(0)
	w32(0)
	w32(0) // reserved1..3

	b.Write(data)
	return b.Bytes()
}

func TestInspectMachO(t *testing.T) {
	bin := buildMachO([]string{"sk_live_abcdefghij", "hello world"})
	infos, err := Inspect(bin)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d arch slices, want 1 (thin)", len(infos))
	}
	mi := infos[0]
	if mi.Arch != "arm64" {
		t.Errorf("Arch = %q, want arm64", mi.Arch)
	}
	if mi.Type != "executable" {
		t.Errorf("Type = %q, want executable", mi.Type)
	}
	if !contains(mi.Segments, "__TEXT") {
		t.Errorf("Segments = %v, want __TEXT", mi.Segments)
	}
	var cstr *SectionInfo
	for i := range mi.Sections {
		if mi.Sections[i].Name == "__cstring" {
			cstr = &mi.Sections[i]
		}
	}
	if cstr == nil || cstr.Segment != "__TEXT" {
		t.Fatalf("__cstring section not found: %+v", mi.Sections)
	}
	// one secret-looking literal (the sk_live token), not the plain string.
	if mi.SecretStrings != 1 {
		t.Errorf("SecretStrings = %d, want 1", mi.SecretStrings)
	}
}

func TestInspectMalformedNoPanic(t *testing.T) {
	for _, in := range [][]byte{
		nil,
		[]byte("garbage"),
		{0xfe, 0xed, 0xfa, 0xcf}, // valid magic, truncated
	} {
		if _, err := Inspect(in); err == nil {
			t.Errorf("expected an error for %d-byte non-Mach-O input", len(in))
		}
	}
}

func TestInspectIPA(t *testing.T) {
	// an IPA whose app binary is the synthetic Mach-O ties #74 (IPA) to #75 (Mach-O).
	path := filepath.Join(t.TempDir(), "app.ipa")
	writeZip(t, path, map[string][]byte{
		"Payload/Foo.app/Foo":        buildMachO([]string{"sk_live_abcdefghij"}),
		"Payload/Foo.app/Info.plist": []byte("bplist00"),
	})
	got, err := InspectIPA(path)
	if err != nil {
		t.Fatal(err)
	}
	infos, ok := got["Payload/Foo.app/Foo"]
	if !ok || len(infos) != 1 {
		t.Fatalf("main binary not inspected: %v", got)
	}
	if infos[0].Arch != "arm64" || infos[0].SecretStrings != 1 {
		t.Errorf("main binary info = %+v", infos[0])
	}
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
