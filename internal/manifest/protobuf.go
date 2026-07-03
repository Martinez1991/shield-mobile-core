package manifest

import (
	"encoding/binary"
	"fmt"
)

// Protobuf AndroidManifest.xml keep-rules (issue #51). Inside an Android App
// Bundle the manifest is not text XML but a serialized aapt2 `XmlNode` protobuf
// (Resources.proto). To keep the engine stdlib-only, this decodes the tiny
// subset of the wire format we need — walking XmlNode -> XmlElement ->
// XmlAttribute — with a hand-rolled reader rather than the protobuf library.
//
// Relevant schema (field numbers):
//
//	XmlNode      { XmlElement element = 1; string text = 2; }
//	XmlElement   { string namespace_uri = 2; string name = 3;
//	               repeated XmlAttribute attribute = 4; repeated XmlNode child = 5; }
//	XmlAttribute { string namespace_uri = 1; string name = 2; string value = 3; }

type pbAttr struct{ ns, name, value string }

type pbElement struct {
	ns, name string
	attrs    []pbAttr
	children []*pbElement
}

// KeepClassesPB extracts component keep-rules from a protobuf-encoded
// AndroidManifest.xml (the base/feature module manifest of an AAB). It returns
// the fully qualified names of the Application and every declared component,
// mirroring the text-manifest KeepClasses. Malformed input yields an error, not
// a panic (the manifest is attacker-influenced).
func KeepClassesPB(pb []byte) ([]string, error) {
	root := parseNode(pb)
	if root == nil || root.name != "manifest" {
		return nil, fmt.Errorf("not a protobuf AndroidManifest (no <manifest> root)")
	}
	pkg := attrValue(root, "", "package")

	seen := make(map[string]bool)
	var out []string
	add := func(name string) {
		if fqcn := resolve(pkg, name); fqcn != "" && !seen[fqcn] {
			seen[fqcn] = true
			out = append(out, fqcn)
		}
	}

	app := childNamed(root, "application")
	if app != nil {
		add(attrValue(app, androidNS, "name"))
		for _, c := range app.children {
			switch c.name {
			case "activity", "service", "receiver", "provider":
				add(attrValue(c, androidNS, "name"))
			case "activity-alias":
				add(attrValue(c, androidNS, "name"))
				add(attrValue(c, androidNS, "targetActivity"))
			}
		}
	}
	return out, nil
}

func attrValue(e *pbElement, ns, name string) string {
	for _, a := range e.attrs {
		if a.name == name && (a.ns == ns || (ns == androidNS && a.ns == "")) {
			return a.value
		}
	}
	return ""
}

func childNamed(e *pbElement, name string) *pbElement {
	for _, c := range e.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

// parseNode decodes an XmlNode and returns its XmlElement (field 1), or nil.
func parseNode(b []byte) *pbElement {
	var el *pbElement
	walkFields(b, func(field, wire int, data []byte) {
		if field == 1 && wire == 2 {
			el = parseElement(data)
		}
	})
	return el
}

func parseElement(b []byte) *pbElement {
	el := &pbElement{}
	walkFields(b, func(field, wire int, data []byte) {
		if wire != 2 {
			return
		}
		switch field {
		case 2:
			el.ns = string(data)
		case 3:
			el.name = string(data)
		case 4:
			el.attrs = append(el.attrs, parseAttr(data))
		case 5:
			if c := parseNode(data); c != nil {
				el.children = append(el.children, c)
			}
		}
	})
	return el
}

func parseAttr(b []byte) pbAttr {
	var a pbAttr
	walkFields(b, func(field, wire int, data []byte) {
		if wire != 2 {
			return
		}
		switch field {
		case 1:
			a.ns = string(data)
		case 2:
			a.name = string(data)
		case 3:
			a.value = string(data)
		}
	})
	return a
}

// walkFields iterates the protobuf fields of b, invoking fn for each. Unknown
// wire types and truncated input stop iteration (best-effort, never panics).
func walkFields(b []byte, fn func(field, wire int, data []byte)) {
	pos := 0
	for pos < len(b) {
		tag, n := binary.Uvarint(b[pos:])
		if n <= 0 {
			return
		}
		pos += n
		field, wire := int(tag>>3), int(tag&7)
		switch wire {
		case 0: // varint
			_, n := binary.Uvarint(b[pos:])
			if n <= 0 {
				return
			}
			pos += n
		case 1: // 64-bit
			if pos+8 > len(b) {
				return
			}
			pos += 8
		case 2: // length-delimited
			l, n := binary.Uvarint(b[pos:])
			if n <= 0 {
				return
			}
			pos += n
			if l > uint64(len(b)-pos) {
				return
			}
			fn(field, wire, b[pos:pos+int(l)])
			pos += int(l)
		case 5: // 32-bit
			if pos+4 > len(b) {
				return
			}
			pos += 4
		default:
			return
		}
	}
}
