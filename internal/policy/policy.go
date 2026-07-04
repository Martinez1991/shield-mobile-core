// Package policy implements Policy-as-Code (shield-platform.md principle P4,
// sections 2.2 stage 8 "Planner" and 10 "policy editor"). A policy is a
// declarative, versionable description of which protection passes to apply and
// at what intensity. Loaded from JSON to stay dependency-free (YAML support is
// on the roadmap; the shape maps 1:1).
package policy

import (
	"encoding/json"
	"fmt"
	"os"
)

// Policy is the tenant-supplied protection specification.
type Policy struct {
	Name    string `json:"name"`
	Version int    `json:"version"`

	// Rename (section 3.1 Identifier Renaming).
	Rename struct {
		Enabled bool `json:"enabled"`
		// IncludePrefixes limits renaming to app-owned packages (internal form,
		// e.g. "com/bank/pay"). Empty => renaming is a no-op (safe default:
		// never rename third-party/library code we can't reason about).
		IncludePrefixes []string `json:"includePrefixes"`
		// KeepClasses are descriptors ("Lcom/foo/Bar;") or dotted names kept
		// verbatim (reachability-aware keep rules: entry points, reflection).
		KeepClasses []string `json:"keepClasses"`
		// Members renames methods/fields too. Scope is deliberately conservative:
		// only private/static methods and private fields of app-owned classes
		// (never vtable-dispatched or externally-visible members), so virtual
		// dispatch and framework overrides are never broken.
		Members bool `json:"members"`
	} `json:"rename"`

	// VM (section 8, Code Virtualization). Compiles eligible straight-line integer
	// methods of app-owned classes (scoped by rename.includePrefixes) to a
	// polymorphic bytecode run by the injected Lshield/rt/VM; interpreter.
	VM struct {
		Enabled bool `json:"enabled"`
	} `json:"vm"`

	// Risk (section 2.2 stage 8 Planner; issue #65). When enabled, the expensive
	// passes (VM, flattening) are applied risk-driven: only to methods whose
	// static risk score (internal/risk) is >= Threshold, instead of uniformly.
	Risk struct {
		Enabled   bool    `json:"enabled"`
		Threshold float64 `json:"threshold"` // [0,1); methods below are left untouched
	} `json:"risk"`

	// Native (issue #82, ADR 0004). LLVM obfuscation of ELF .so via the
	// out-of-tree native-svc subprocess. Enabled is a no-op unless native-svc is
	// installed in the worker image; the engine/DEX pipeline is unaffected.
	Native struct {
		Enabled bool     `json:"enabled"`
		Passes  []string `json:"passes"` // flatten|mba|opaque|strings
	} `json:"native"`

	// RASP (section 6). Injects the Lshield/rt/RASP; runtime (root/debugger/
	// emulator detection with deferred reaction). It is a library the host/app
	// can call; wiring it into entry points is app-specific.
	RASP struct {
		Enabled bool `json:"enabled"`
	} `json:"rasp"`

	// ControlFlow (section 3.2). Inserts always-true opaque predicates guarding
	// dead junk blocks at method entry. Verifier-safe: reuses a free local
	// register at entry (no register-count changes), real body untouched.
	ControlFlow struct {
		Enabled bool `json:"enabled"`
		// Reorder physically shuffles basic blocks (§7 Instruction Reordering),
		// safe-by-construction layout flattening.
		Reorder bool `json:"reorder"`
		// Flatten rewrites a method's control flow into a central dispatcher loop
		// (§3.2/8 flattening), using the typed IR for register layout/eligibility.
		Flatten bool `json:"flatten"`
	} `json:"controlFlow"`

	// Strings (section 3.3 String Encryption).
	Strings struct {
		Enabled   bool `json:"enabled"`
		MinLength int  `json:"minLength"`
		// Algorithm: "xor" (low-risk, cheap) or "aes" (AES-256-GCM, runtime-derived
		// key). Defaults to "xor".
		Algorithm string `json:"algorithm"`
	} `json:"strings"`

	// Metadata (section 3.5 Metadata Removal).
	Metadata struct {
		Enabled bool `json:"enabled"`
	} `json:"metadata"`

	// Junk (subset of section 3.2 Control Flow; conservative NOP padding only.
	// Full flattening/opaque predicates are roadmap V3 — see README).
	Junk struct {
		Enabled bool `json:"enabled"`
		Nops    int  `json:"nops"`
	} `json:"junk"`

	// Seed makes the build deterministic (principle P2). Same seed + same input
	// + same engine => functionally identical output.
	Seed int64 `json:"seed"`
}

// Default returns a conservative, semantics-preserving policy.
func Default() Policy {
	var p Policy
	p.Name = "default"
	p.Version = 1
	p.Strings.Enabled = true
	p.Strings.MinLength = 3
	p.Strings.Algorithm = "xor"
	p.Metadata.Enabled = true
	p.Rename.Enabled = false // opt-in: needs an include prefix to be safe
	p.Junk.Enabled = false
	p.Junk.Nops = 3
	p.Seed = 0x5117e1d
	return p
}

// Preset returns a named built-in policy. Unknown names fall back to Default.
func Preset(name string) Policy {
	p := Default()
	switch name {
	case "prod-high", "prod_high":
		p.Name = "prod-high"
		p.Rename.Enabled = true
		p.Rename.Members = true
		p.Strings.Enabled = true
		p.Strings.MinLength = 2
		p.Strings.Algorithm = "aes"
		p.Metadata.Enabled = true
		p.ControlFlow.Enabled = true
		p.ControlFlow.Reorder = true
		p.VM.Enabled = true
		p.RASP.Enabled = true
		p.Junk.Enabled = true
		p.Junk.Nops = 5
	case "balanced":
		p.Name = "balanced"
		p.Rename.Enabled = true
		p.Strings.Enabled = true
		p.Metadata.Enabled = true
		p.ControlFlow.Enabled = true
	case "minimal":
		p.Name = "minimal"
		p.Strings.Enabled = false
		p.Metadata.Enabled = true
	}
	return p
}

// Load parses a JSON policy file.
func Load(path string) (Policy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Default(), err
	}
	return Parse(raw)
}

// Parse parses a JSON policy from bytes, starting from Default() so omitted
// fields keep their conservative defaults.
func Parse(raw []byte) (Policy, error) {
	p := Default()
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("invalid policy: %w", err)
	}
	return p, p.Validate()
}

// Validate checks the policy is internally consistent (a lightweight version of
// policy-svc validation, section 1.2).
func (p Policy) Validate() error {
	if p.Strings.MinLength < 0 {
		return fmt.Errorf("strings.minLength must be >= 0")
	}
	switch p.Strings.Algorithm {
	case "", "xor", "aes":
	default:
		return fmt.Errorf("strings.algorithm must be xor or aes, got %q", p.Strings.Algorithm)
	}
	if p.Junk.Nops < 0 {
		return fmt.Errorf("junk.nops must be >= 0")
	}
	return nil
}

// RenameScoped reports whether renaming will actually do anything. Renaming is
// deliberately a no-op without includePrefixes (we never rename code we can't
// scope — it would risk breaking libraries/reflection), so callers can warn.
func (p Policy) RenameScoped() bool {
	return p.Rename.Enabled && len(p.Rename.IncludePrefixes) > 0
}
