package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Martinez1991/shield-mobile-core/policy"
)

const sampleYAML = `# SHIELD project config
version: 1

input: ./dist
output: ./protected

protection:
  obfuscation: true
  antidebug: true
  antifuto: true
  antitampering: true

include:
  - src/**
  - config/**

exclude:
  - tests/**
  - docs/**
`

func loadFrom(t *testing.T, name, content string) *Config {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load(%s): %v", name, err)
	}
	return c
}

func TestLoadYAML(t *testing.T) {
	c := loadFrom(t, "shield.yml", sampleYAML)
	if c.Version != 1 || c.Input != "./dist" || c.Output != "./protected" {
		t.Errorf("scalars = %+v", c)
	}
	if !c.Protection.Obfuscation || !c.Protection.Antidebug || !c.Protection.Antifuto || !c.Protection.Antitampering {
		t.Errorf("protection = %+v", c.Protection)
	}
	if len(c.Include) != 2 || c.Include[0] != "src/**" || c.Include[1] != "config/**" {
		t.Errorf("include = %v", c.Include)
	}
	if len(c.Exclude) != 2 || c.Exclude[0] != "tests/**" {
		t.Errorf("exclude = %v", c.Exclude)
	}
}

func TestLoadJSON(t *testing.T) {
	c := loadFrom(t, "shield.json", `{"input":"a","output":"b","protection":{"antidebug":true},"include":["x/**"]}`)
	if c.Input != "a" || c.Output != "b" || !c.Protection.Antidebug || c.Version != 1 {
		t.Errorf("json config = %+v", c)
	}
}

func TestToPolicy(t *testing.T) {
	c := loadFrom(t, "shield.yml", sampleYAML)
	p := c.ToPolicy(policy.Policy{})
	if !p.Rename.Enabled || !p.Strings.Enabled || !p.ControlFlow.Enabled {
		t.Error("obfuscation toggle did not enable the standard passes")
	}
	if !p.RASP.Enabled {
		t.Error("antidebug did not enable RASP")
	}
	if !p.Native.Enabled {
		t.Error("antitampering did not enable native")
	}
	// tamper present exactly once despite antitampering + antifuto both adding it.
	n := 0
	for _, pass := range p.Native.Passes {
		if pass == "tamper" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("tamper appears %d times, want 1 (deduped): %v", n, p.Native.Passes)
	}
}

func TestYAMLRejectsTabs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yml")
	_ = os.WriteFile(p, []byte("protection:\n\tobfuscation: true\n"), 0o600)
	if _, err := Load(p); err == nil {
		t.Error("expected an error for tab indentation")
	}
}

func TestUnsupportedExtension(t *testing.T) {
	p := filepath.Join(t.TempDir(), "shield.toml")
	_ = os.WriteFile(p, []byte("x=1"), 0o600)
	if _, err := Load(p); err == nil {
		t.Error("expected an error for an unsupported extension")
	}
}

func TestSelector(t *testing.T) {
	cases := []struct {
		include, exclude []string
		path             string
		want             bool
	}{
		{[]string{"src/**"}, nil, "src/a/b/Foo.smali", true},
		{[]string{"src/**"}, nil, "lib/Foo.smali", false},
		{[]string{"config/*.json"}, nil, "config/app.json", true},
		{[]string{"config/*.json"}, nil, "config/deep/app.json", false}, // * is one segment
		{[]string{"src/**"}, []string{"src/tests/**"}, "src/tests/Foo.smali", false},
		{[]string{"src/**"}, []string{"src/tests/**"}, "src/main/Foo.smali", true},
		{[]string{"src/**", "!src/gen/**"}, nil, "src/gen/R.smali", false}, // ! in include
		{nil, nil, "anything/goes.smali", true},                            // no include = all
		{nil, []string{"**/BuildConfig.smali"}, "a/b/BuildConfig.smali", false},
	}
	for _, c := range cases {
		got := NewSelector(c.include, c.exclude).Match(c.path)
		if got != c.want {
			t.Errorf("Selector(inc=%v exc=%v).Match(%q) = %v, want %v",
				c.include, c.exclude, c.path, got, c.want)
		}
	}
}
