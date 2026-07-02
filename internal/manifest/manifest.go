// Package manifest extracts reachability keep-rules from a decoded
// AndroidManifest.xml (shield-platform.md section 3.1). Components declared in
// the manifest (Application, Activity, Service, Receiver, Provider, alias) are
// referenced by their original class name by the framework, so renaming them
// breaks the app. These classes must be kept verbatim.
package manifest

import (
	"encoding/xml"
	"os"
	"strings"
)

const androidNS = "http://schemas.android.com/apk/res/android"

type named struct {
	Name string `xml:"http://schemas.android.com/apk/res/android name,attr"`
	// activity-alias also names a targetActivity that must be kept.
	Target string `xml:"http://schemas.android.com/apk/res/android targetActivity,attr"`
}

type manifestXML struct {
	Package string `xml:"package,attr"`
	App     struct {
		Name       string  `xml:"http://schemas.android.com/apk/res/android name,attr"`
		Activities []named `xml:"activity"`
		Aliases    []named `xml:"activity-alias"`
		Services   []named `xml:"service"`
		Receivers  []named `xml:"receiver"`
		Providers  []named `xml:"provider"`
	} `xml:"application"`
}

// KeepClasses parses a decoded AndroidManifest.xml and returns the fully
// qualified (dotted) names of every declared component. Returns an error if the
// file cannot be read/parsed (callers treat a missing manifest as "no rules").
func KeepClasses(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m manifestXML
	if err := xml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	pkg := m.Package
	seen := make(map[string]bool)
	var out []string
	add := func(name string) {
		fqcn := resolve(pkg, name)
		if fqcn != "" && !seen[fqcn] {
			seen[fqcn] = true
			out = append(out, fqcn)
		}
	}
	add(m.App.Name)
	for _, g := range [][]named{m.App.Activities, m.App.Aliases, m.App.Services, m.App.Receivers, m.App.Providers} {
		for _, n := range g {
			add(n.Name)
			add(n.Target)
		}
	}
	return out, nil
}

// resolve turns an android:name (which may be relative) into a FQCN.
//   - ".Foo"          -> pkg + ".Foo"
//   - "Foo" (no dot)  -> pkg + ".Foo"
//   - "com.x.Foo"     -> as-is
func resolve(pkg, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, ".") {
		return pkg + name
	}
	if !strings.Contains(name, ".") {
		if pkg == "" {
			return name
		}
		return pkg + "." + name
	}
	return name
}

// NS is exported for tests/documentation of the expected namespace.
const NS = androidNS
