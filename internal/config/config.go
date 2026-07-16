// Package config loads a SHIELD project config file (shield.yml or shield.json)
// so a build doesn't need a wall of CLI flags — it's reusable, versionable in Git
// and CI-friendly. It maps the config's protection toggles onto an engine policy
// and its include/exclude globs onto a Selector. Stdlib-only (JSON natively, a
// minimal YAML subset in yaml.go), keeping the core zero-dependency.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/policy"
)

// Config is a shield.yml / shield.json.
type Config struct {
	Version    int        `json:"version"`
	Input      string     `json:"input"`
	Output     string     `json:"output"`
	Preset     string     `json:"preset"` // optional base preset the toggles extend
	Protection Protection `json:"protection"`
	Include    []string   `json:"include"`
	Exclude    []string   `json:"exclude"`
}

// Protection is the high-level toggle block. Each maps to concrete engine passes
// (see ToPolicy) so users think in outcomes, not internal pass names.
type Protection struct {
	Obfuscation   bool `json:"obfuscation"`
	Antidebug     bool `json:"antidebug"`
	Antifuto      bool `json:"antifuto"`
	Antitampering bool `json:"antitampering"`
}

// Load reads a config from path. The format is chosen by extension: .json is
// parsed natively; .yml/.yaml (or no extension) via the minimal YAML reader.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	case ".yml", ".yaml", "":
		m, err := yamlToMap(data)
		if err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
		jb, _ := json.Marshal(m)
		if err := json.Unmarshal(jb, &c); err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("config %s: unsupported format (use .yml or .json)", path)
	}
	if c.Version == 0 {
		c.Version = 1
	}
	return &c, nil
}

// ToPolicy applies the config's protection toggles onto a base policy (e.g. a
// preset), enabling the concrete passes each toggle stands for.
func (c *Config) ToPolicy(base policy.Policy) policy.Policy {
	return c.Protection.Apply(base)
}

// Apply enables the passes each toggle stands for on top of a base policy. Only
// enables (never disables), so it composes with a preset and with CLI flags.
func (pr Protection) Apply(p policy.Policy) policy.Policy {
	if pr.Obfuscation {
		p.Rename.Enabled = true
		p.Strings.Enabled = true
		p.ControlFlow.Enabled = true
		p.Metadata.Enabled = true
	}
	if pr.Antidebug {
		p.RASP.Enabled = true // root/debugger/emulator/Frida/Xposed detection
	}
	if pr.Antitampering {
		p.Native.Enabled = true
		p.Native.Passes = withPass(p.Native.Passes, "tamper") // self-checksum
	}
	if pr.Antifuto {
		// Anti-theft / anti-extraction: today backed by anti-tamper + RASP
		// (repackaging/extraction detection). Device/environment binding is
		// tracked as future work — see the follow-up issue.
		p.Native.Enabled = true
		p.Native.Passes = withPass(p.Native.Passes, "tamper")
		p.RASP.Enabled = true
	}
	return p
}

// Selector builds the include/exclude matcher for this config.
func (c *Config) Selector() *Selector { return NewSelector(c.Include, c.Exclude) }

func withPass(passes []string, p string) []string {
	for _, x := range passes {
		if x == p {
			return passes
		}
	}
	return append(passes, p)
}
