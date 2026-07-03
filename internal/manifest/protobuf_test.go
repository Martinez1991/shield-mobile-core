package manifest

import (
	"encoding/binary"
	"testing"
)

// --- minimal protobuf encoders (mirror the aapt XmlNode schema) ---

func pbField(field int, b []byte) []byte {
	out := binary.AppendUvarint(nil, uint64(field<<3|2))
	out = binary.AppendUvarint(out, uint64(len(b)))
	return append(out, b...)
}

func pbStr(field int, s string) []byte { return pbField(field, []byte(s)) }

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func pbAttrEnc(ns, name, value string) []byte {
	return cat(pbStr(1, ns), pbStr(2, name), pbStr(3, value))
}

// pbElem builds an XmlElement; attrs/children are already-encoded byte slices.
func pbElem(name string, attrs, children [][]byte) []byte {
	b := cat(pbStr(2, ""), pbStr(3, name))
	for _, a := range attrs {
		b = append(b, pbField(4, a)...)
	}
	for _, c := range children {
		b = append(b, pbField(5, c)...)
	}
	return b
}

// pbNode wraps an element as an XmlNode (field 1).
func pbNode(elem []byte) []byte { return pbField(1, elem) }

func TestKeepClassesPB(t *testing.T) {
	activity := pbNode(pbElem("activity", [][]byte{pbAttrEnc(androidNS, "name", ".Main")}, nil))
	service := pbNode(pbElem("service", [][]byte{pbAttrEnc(androidNS, "name", "com.x.svc.Sync")}, nil))
	alias := pbNode(pbElem("activity-alias", [][]byte{
		pbAttrEnc(androidNS, "name", ".Alias"),
		pbAttrEnc(androidNS, "targetActivity", ".Main"),
	}, nil))
	app := pbNode(pbElem("application",
		[][]byte{pbAttrEnc(androidNS, "name", ".App")},
		[][]byte{activity, service, alias}))
	manifest := pbNode(pbElem("manifest",
		[][]byte{pbAttrEnc("", "package", "com.x")},
		[][]byte{app}))

	keep, err := KeepClassesPB(manifest)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, k := range keep {
		got[k] = true
	}
	for _, want := range []string{"com.x.App", "com.x.Main", "com.x.svc.Sync", "com.x.Alias"} {
		if !got[want] {
			t.Errorf("missing keep %q; got %v", want, keep)
		}
	}
	// Main is declared twice (activity + alias target) but kept once.
	n := 0
	for _, k := range keep {
		if k == "com.x.Main" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("com.x.Main kept %d times, want 1 (deduped)", n)
	}
}

func TestKeepClassesPBRejectsNonManifest(t *testing.T) {
	other := pbNode(pbElem("resources", nil, nil))
	if _, err := KeepClassesPB(other); err == nil {
		t.Error("expected an error for a non-manifest root element")
	}
}

func TestKeepClassesPBMalformedNoPanic(t *testing.T) {
	// Truncated / garbage input must not panic (attacker-influenced).
	for _, in := range [][]byte{
		nil,
		{0xff, 0xff, 0xff},
		{0x0a, 0x05, 0x01, 0x02}, // field 1, len 5, but only 2 bytes follow
		pbNode([]byte{0x1a, 0x80}),
	} {
		_, _ = KeepClassesPB(in) // must return, not panic
	}
}
