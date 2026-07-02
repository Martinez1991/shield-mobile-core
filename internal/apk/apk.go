// Package apk orchestrates the full APK round-trip (shield-platform.md section
// 2.1 pipeline and section 4). It shells out to apktool for DEX<->smali and to
// apksigner for signing; the SHIELD engine does the protection in between. Tools
// are optional: if they are absent, callers get a clear, actionable error rather
// than a crash.
package apk

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"shield/internal/engine"
	"shield/internal/policy"
)

// Options configures a protect run.
type Options struct {
	Input   string // path to .apk / .aab
	Output  string // protected artifact path
	Policy  policy.Policy
	WorkDir string // scratch dir (created if empty)

	// Optional signing (section 2.2 stage 22). If Keystore is empty the artifact
	// is left unsigned for the caller to sign (e.g. Play App Signing / fastlane).
	Keystore string
	KsPass   string
	KeyAlias string

	Log func(string)
}

// ToolAvailable reports whether a CLI tool is on PATH.
func ToolAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (o Options) logf(format string, a ...any) {
	if o.Log != nil {
		o.Log(fmt.Sprintf(format, a...))
	}
}

// Protect runs decode -> obfuscate -> build -> (sign).
func Protect(o Options) (*engine.Result, error) {
	if !ToolAvailable("apktool") {
		return nil, fmt.Errorf("apktool not found on PATH.\n" +
			"Install it (https://apktool.org) to enable the APK round-trip, or run\n" +
			"`shield obfuscate <smali-dir>` directly on an already-decoded project")
	}
	work := o.WorkDir
	if work == "" {
		w, err := os.MkdirTemp("", "shield-*")
		if err != nil {
			return nil, err
		}
		work = w
		defer os.RemoveAll(work)
	}
	decoded := filepath.Join(work, "decoded")

	// 1) Decode APK -> smali project.
	o.logf("decoding %s", o.Input)
	if err := run("apktool", "d", "-f", "-o", decoded, o.Input); err != nil {
		return nil, fmt.Errorf("apktool decode failed: %w", err)
	}

	// 2) Apply protection passes over the smali IR.
	o.logf("applying policy %q", o.Policy.Name)
	res, err := engine.Run(decoded, o.Policy)
	if err != nil {
		return nil, fmt.Errorf("obfuscation failed: %w", err)
	}

	// 3) Rebuild APK.
	built := filepath.Join(work, "built.apk")
	o.logf("rebuilding apk")
	if err := run("apktool", "b", "-o", built, decoded); err != nil {
		return nil, fmt.Errorf("apktool build failed: %w", err)
	}

	// 4) Sign (optional) or copy through.
	final := built
	if o.Keystore != "" {
		if !ToolAvailable("apksigner") {
			return nil, fmt.Errorf("keystore given but apksigner not on PATH")
		}
		o.logf("signing with %s", o.Keystore)
		if err := run("apksigner", "sign",
			"--ks", o.Keystore, "--ks-pass", "pass:"+o.KsPass,
			"--ks-key-alias", o.KeyAlias, built); err != nil {
			return nil, fmt.Errorf("apksigner failed: %w", err)
		}
	} else {
		o.logf("no keystore: leaving artifact unsigned")
	}

	if err := copyFile(final, o.Output); err != nil {
		return nil, err
	}
	o.logf("wrote %s", o.Output)
	return res, nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
