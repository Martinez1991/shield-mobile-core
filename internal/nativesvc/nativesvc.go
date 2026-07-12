// Package nativesvc is the Go-side seam for LLVM-based native protection
// (issue #82, ADR 0004). The actual obfuscation passes live in an out-of-tree
// executable, native-svc (C++/LLVM), invoked as a subprocess — never linked into
// the Go build, so the engine and go.mod stay stdlib-only and CGO-free. This
// package owns the boundary: pass selection from policy, discovering the
// executable, framing the request/response, and an offline Plan over an APK/AAB.
//
// When native-svc is not installed, callers get a typed ErrUnavailable and skip
// native protection gracefully — the Android DEX pipeline is unaffected — exactly
// as internal/apk degrades when apktool is absent.
package nativesvc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"shield/internal/native"
)

// Pass is one LLVM obfuscation pass native-svc can apply.
type Pass string

const (
	PassFlatten Pass = "flatten" // control-flow graph flattening
	PassMBA     Pass = "mba"     // mixed boolean-arithmetic substitution
	PassOpaque  Pass = "opaque"  // opaque predicates
	PassStrings Pass = "strings" // native string / const obfuscation
	PassRasp    Pass = "rasp"    // runtime anti-debug injection
)

// validPasses is the set native-svc understands; ParsePasses rejects the rest.
var validPasses = map[Pass]bool{
	PassFlatten: true, PassMBA: true, PassOpaque: true, PassStrings: true, PassRasp: true,
}

// ErrUnavailable means native-svc (the LLVM toolchain) is not installed. It is
// typed so callers can degrade (skip native passes) instead of hard-failing.
var ErrUnavailable = errors.New("native-svc (LLVM toolchain) not available on PATH or $SHIELD_NATIVE_SVC")

// Config selects passes and locates the out-of-tree native-svc executable.
type Config struct {
	Passes []Pass
	Seed   int64
	// Exec overrides the executable path; empty falls back to $SHIELD_NATIVE_SVC
	// then "native-svc" on PATH.
	Exec string
}

// ParsePasses converts policy pass names to Pass values, erroring on unknowns.
func ParsePasses(names []string) ([]Pass, error) {
	out := make([]Pass, 0, len(names))
	for _, n := range names {
		p := Pass(n)
		if !validPasses[p] {
			return nil, fmt.Errorf("unknown native pass %q (want flatten|mba|opaque|strings)", n)
		}
		out = append(out, p)
	}
	return out, nil
}

// Runner executes native-svc. It is injectable so the subprocess contract is
// testable offline without the real toolchain.
type Runner func(exec string, args []string, stdin []byte) (stdout []byte, err error)

// Service applies native passes via a located native-svc executable.
type Service struct {
	cfg  Config
	exec string
	run  Runner
}

// Locate resolves the native-svc executable: Config.Exec, then $SHIELD_NATIVE_SVC,
// then "native-svc" on PATH. Returns ErrUnavailable if none resolves.
func Locate(cfg Config) (string, error) {
	candidates := []string{cfg.Exec, os.Getenv("SHIELD_NATIVE_SVC"), "native-svc"}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if path, err := exec.LookPath(c); err == nil {
			return path, nil
		}
	}
	return "", ErrUnavailable
}

// New locates native-svc and returns a Service, or ErrUnavailable if the
// toolchain is not installed.
func New(cfg Config) (*Service, error) {
	path, err := Locate(cfg)
	if err != nil {
		return nil, err
	}
	return &Service{cfg: cfg, exec: path, run: execRunner}, nil
}

// newWithRunner builds a Service around an injected runner (tests).
func newWithRunner(cfg Config, exec string, run Runner) *Service {
	return &Service{cfg: cfg, exec: exec, run: run}
}

// Apply runs the configured passes over one object/bitcode blob and returns the
// transformed object. It implements the ADR 0004 subprocess contract:
//
//	native-svc transform --arch <abi> --seed <n> --pass <p> [--pass <p> …]  < obj > obj
func (s *Service) Apply(arch string, object []byte) ([]byte, error) {
	if len(s.cfg.Passes) == 0 {
		return nil, fmt.Errorf("no native passes configured")
	}
	args := []string{"transform", "--arch", arch, "--seed", fmt.Sprintf("%d", s.cfg.Seed)}
	for _, p := range s.cfg.Passes {
		args = append(args, "--pass", string(p))
	}
	out, err := s.run(s.exec, args, object)
	if err != nil {
		return nil, fmt.Errorf("native-svc transform (arch %s): %w", arch, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("native-svc returned an empty object (arch %s)", arch)
	}
	return out, nil
}

// execRunner is the production Runner: spawn native-svc, stream the object over
// stdin, capture stdout. Diagnostics on stderr are surfaced in the error.
func execRunner(execPath string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.Command(execPath, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

// Candidate is one native library eligible for LLVM protection.
type Candidate struct {
	Entry string `json:"entry"` // zip entry name
	ABI   string `json:"abi"`   // arm64-v8a, x86_64, …
	Arch  string `json:"arch"`  // normalized machine (arm64, x86_64, …)
}

// Plan lists the native .so in an APK/AAB that native-svc could protect. It is
// fully offline (reuses the #81 inspector) and does not need the toolchain, so
// callers can preview the native surface before deciding to run passes.
func Plan(path string) ([]Candidate, error) {
	inspected, err := native.InspectArchive(path)
	if err != nil {
		return nil, err
	}
	var out []Candidate
	for _, entry := range native.SortedLibs(inspected) {
		abi, _ := native.NativeLib(entry)
		out = append(out, Candidate{Entry: entry, ABI: abi, Arch: inspected[entry].Machine})
	}
	return out, nil
}
