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

	"github.com/Martinez1991/shield-mobile-core/engine"
	"github.com/Martinez1991/shield-mobile-core/internal/ios"
	"github.com/Martinez1991/shield-mobile-core/policy"
)

// Options configures a protect run.
type Options struct {
	Input   string // path to .apk / .aab
	Output  string // protected artifact path
	Policy  policy.Policy
	WorkDir string // scratch dir (created if empty)

	// Optional signing (section 2.2 stage 22). If Keystore is empty the artifact
	// is left unsigned for the caller to sign (e.g. Play App Signing / fastlane).
	//
	// The password must be supplied out-of-band (never on the shield CLI argv):
	// either KsPassFile (a file readable by apksigner) or KsPass (an in-memory
	// value the caller sourced from an env var / secret store). If both are set,
	// KsPassFile wins.
	Keystore   string
	KsPass     string
	KsPassFile string
	KeyAlias   string

	Log func(string)
}

// ksPassArg builds the apksigner `--ks-pass` argument without exposing the
// secret on any command line (CWE-214). A caller-provided file is used directly;
// an in-memory password is written to a 0600 temp file inside the work dir
// (removed with it).
func ksPassArg(o Options, work string) (string, error) {
	if o.KsPassFile != "" {
		return "file:" + o.KsPassFile, nil
	}
	if o.KsPass == "" {
		return "", fmt.Errorf("keystore set but no password provided (use --ks-pass-file or SHIELD_KS_PASS)")
	}
	f := filepath.Join(work, ".kspass")
	if err := os.WriteFile(f, []byte(o.KsPass), 0o600); err != nil {
		return "", err
	}
	return "file:" + f, nil
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

// Protect runs decode -> obfuscate -> build -> (sign). An Android App Bundle
// (.aab) is routed to the bundle round-trip (section 4, issue #16); APKs go
// through apktool below.
func Protect(o Options) (*engine.Result, error) {
	if IsAAB(o.Input) {
		return protectAAB(o)
	}
	if ios.IsIPA(o.Input) {
		return nil, fmt.Errorf("iOS/IPA recognized, but Mach-O protection is not available yet " +
			"(the pipeline is in progress — see issue #63). Android APK/AAB is supported today")
	}
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
		// CWE-214: never pass the keystore password on the command line. Write it
		// to a 0600 temp file inside the (soon-deleted) work dir and hand apksigner
		// a `file:` reference so it is not visible in the process listing.
		passArg, err := ksPassArg(o, work)
		if err != nil {
			return nil, err
		}
		o.logf("signing with %s", o.Keystore)
		if err := run("apksigner", "sign",
			"--ks", o.Keystore, "--ks-pass", passArg,
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
