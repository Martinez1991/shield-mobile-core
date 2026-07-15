package ios

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Mach-O metadata strip (issue #76). Removing symbols / superfluous __LINKEDIT is
// byte surgery best done by the platform `strip`; stripping also invalidates the
// code signature, which on Apple Silicon must be at least ad-hoc for the binary
// to run. So this package orchestrates the IPA round-trip — locate the app binary
// and framework Mach-Os, strip each, optionally ad-hoc re-sign, repackage byte-
// for-byte — and injects the strip/sign steps (Stripper/Signer): real tool shell-
// outs in production, fakes in tests, so the orchestration is offline-testable.
// (Full distribution re-signing with a real certificate is #77.)

// Stripper strips a Mach-O file in place (e.g. `xcrun strip -x`).
type Stripper func(machoPath string) error

// Signer re-signs a Mach-O file in place (ad-hoc, e.g. `codesign -f -s -`). A nil
// Signer leaves the binary unsigned.
type Signer func(machoPath string) error

// StripResult reports which bundle Mach-Os were stripped.
type StripResult struct {
	Stripped []string `json:"stripped"`
}

// StripIPA strips the app binary and framework Mach-Os of the IPA at inPath and
// writes the result to outPath. strip is required; sign is optional (nil leaves
// them unsigned). Non-Mach-O entries and everything else are preserved.
func StripIPA(inPath, outPath string, strip Stripper, sign Signer) (*StripResult, error) {
	if strip == nil {
		return nil, fmt.Errorf("StripIPA: a Stripper is required")
	}
	zr, err := zip.OpenReader(inPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	bundle, ok := FindBundle(&zr.Reader)
	if !ok {
		return nil, fmt.Errorf("no app bundle in %s", inPath)
	}

	work, err := os.MkdirTemp("", "shield-ios-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	sub := map[string][]byte{}
	res := &StripResult{}
	for _, name := range append([]string{bundle.MainBinary}, bundle.Frameworks...) {
		data, err := readEntry(&zr.Reader, name)
		if err != nil {
			continue
		}
		if _, err := Inspect(data); err != nil {
			continue // not a Mach-O; leave it alone
		}
		p := filepath.Join(work, filepath.Base(name))
		if err := os.WriteFile(p, data, 0o600); err != nil {
			return nil, err
		}
		if err := strip(p); err != nil {
			return nil, fmt.Errorf("strip %s: %w", name, err)
		}
		if sign != nil {
			if err := sign(p); err != nil {
				return nil, fmt.Errorf("sign %s: %w", name, err)
			}
		}
		stripped, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		sub[name] = stripped
		res.Stripped = append(res.Stripped, name)
	}

	if err := RewriteIPA(inPath, outPath, sub); err != nil {
		return nil, err
	}
	sort.Strings(res.Stripped)
	return res, nil
}

// ShellStripper builds a Stripper from a command line (e.g. "xcrun strip -x"); the
// Mach-O path is appended as the final argument.
func ShellStripper(cmdline string) Stripper {
	fields := strings.Fields(cmdline)
	return func(p string) error { return runTool(fields, p) }
}

// ShellSigner builds a Signer from a command line (e.g. "codesign -f -s -").
func ShellSigner(cmdline string) Signer {
	fields := strings.Fields(cmdline)
	return func(p string) error { return runTool(fields, p) }
}

func runTool(fields []string, path string) error {
	if len(fields) == 0 {
		return fmt.Errorf("empty command")
	}
	args := append(append([]string{}, fields[1:]...), path)
	cmd := exec.Command(fields[0], args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w: %s", fields[0], err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}
