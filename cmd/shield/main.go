// Command shield is the SHIELD platform CLI (shield-platform.md section 12).
//
//	shield analyze  <smali-dir>            static inventory + sensitive-code report
//	shield obfuscate <smali-dir> --out ... apply protection passes to a smali project
//	shield protect  <app.apk>   --out ...  full APK round-trip (needs apktool)
//	shield policy   show|validate ...      policy-as-code helpers
//	shield version
//
// Exit codes: 0 ok, >=10 protection failure, >=20 policy failure.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"shield/internal/analyze"
	"shield/internal/apk"
	"shield/internal/engine"
	"shield/internal/policy"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "analyze":
		cmdAnalyze(os.Args[2:])
	case "obfuscate":
		cmdObfuscate(os.Args[2:])
	case "protect":
		cmdProtect(os.Args[2:])
	case "policy":
		cmdPolicy(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("shield", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `SHIELD — Mobile Application Shielding CLI

Usage:
  shield analyze   <smali-dir> [--json]
  shield obfuscate <smali-dir> --out <dir> [--policy p.json | --preset name]
                   [--in-place] [--mapping f] [--report f]
  shield protect   <app.apk> --out <apk> [--policy p.json | --preset name]
                   [--ks keystore --ks-pass p --ks-alias a]
  shield policy    show <preset> | validate <policy.json>
  shield version

Presets: prod-high, balanced, minimal, default
`)
}

// ---- analyze ------------------------------------------------------------

func cmdAnalyze(args []string) {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "machine-readable output")
	subject, rest := splitSubject(args)
	if subject == "" {
		die(20, "analyze: missing <smali-dir> (must be the first argument)")
	}
	_ = fs.Parse(rest)
	rep, err := analyze.Run(subject)
	if err != nil {
		die(10, "analyze: %v", err)
	}
	if *asJSON {
		printJSON(rep)
		return
	}
	fmt.Printf("Classes: %d  Methods: %d  Strings: %d\n", rep.Classes, rep.Methods, rep.Strings)
	if len(rep.Findings) == 0 {
		fmt.Println("Sensitive findings: none")
		return
	}
	fmt.Printf("Sensitive findings: %d\n", len(rep.Findings))
	for _, f := range rep.Findings {
		fmt.Printf("  [%-18s] %-40s %s (entropy %.2f)\n", f.Kind, dotted(f.Class), f.Preview, f.Entropy)
	}
}

// ---- obfuscate ----------------------------------------------------------

func cmdObfuscate(args []string) {
	fs := flag.NewFlagSet("obfuscate", flag.ExitOnError)
	out := fs.String("out", "", "output smali dir (required unless --in-place)")
	inPlace := fs.Bool("in-place", false, "transform the input dir directly (destructive)")
	policyPath := fs.String("policy", "", "policy JSON file")
	preset := fs.String("preset", "default", "built-in preset")
	mappingPath := fs.String("mapping", "", "write rename mapping here (default <out>/mapping.txt)")
	reportPath := fs.String("report", "", "write JSON evidence report here")
	src, rest := splitSubject(args)
	if src == "" {
		die(20, "obfuscate: missing <smali-dir> (must be the first argument)")
	}
	_ = fs.Parse(rest)
	pol := resolvePolicy(*policyPath, *preset)
	if pol.Rename.Enabled && !pol.RenameScoped() {
		fmt.Fprintln(os.Stderr, "warning: rename enabled but no includePrefixes set — renaming will be skipped (unscoped, unsafe). Use a policy file with rename.includePrefixes.")
	}

	target := src
	if !*inPlace {
		if *out == "" {
			die(20, "obfuscate: --out required (or pass --in-place)")
		}
		if err := copyTree(src, *out); err != nil {
			die(10, "copy: %v", err)
		}
		target = *out
	}

	res, err := engine.Run(target, pol)
	if err != nil {
		die(10, "obfuscate: %v", err)
	}

	if res.ClassesRenamed > 0 {
		mp := *mappingPath
		if mp == "" {
			mp = filepath.Join(target, "mapping.txt")
		}
		if err := engine.WriteMappingFile(mp, res.Mapping); err != nil {
			die(10, "mapping: %v", err)
		}
	}
	if *reportPath != "" {
		if err := writeJSONFile(*reportPath, res); err != nil {
			die(10, "report: %v", err)
		}
	}
	fmt.Printf("policy=%s  classes=%d  metadata-=%d  strings-enc=%d  classes-renamed=%d  members-renamed=%d  manifest-keeps=%d  virtualized=%d  reordered=%d  opaque=%d  padded=%d  rasp=%t  method-refs~%d\n",
		res.Policy, res.ClassesTotal, res.MetadataRemoved, res.StringsEncrypted,
		res.ClassesRenamed, res.MembersRenamed, res.ManifestKeeps, res.MethodsVirtual, res.MethodsReordered, res.OpaquePredicates, res.MethodsPadded, res.RASPInjected, res.MethodRefs)
	if res.MultidexRisk {
		fmt.Fprintf(os.Stderr, "warning: ~%d method refs — near the 64K single-DEX limit; ensure multidex is enabled.\n", res.MethodRefs)
	}
	fmt.Printf("applied: %v\n", res.Applied)
}

// ---- protect ------------------------------------------------------------

func cmdProtect(args []string) {
	fs := flag.NewFlagSet("protect", flag.ExitOnError)
	out := fs.String("out", "", "protected APK output path (required)")
	policyPath := fs.String("policy", "", "policy JSON file")
	preset := fs.String("preset", "prod-high", "built-in preset")
	ks := fs.String("ks", "", "keystore for signing (optional)")
	ksPassFile := fs.String("ks-pass-file", "", "file containing the keystore password (preferred; else env SHIELD_KS_PASS)")
	ksAlias := fs.String("ks-alias", "", "key alias")
	input, rest := splitSubject(args)
	if input == "" {
		die(20, "protect: missing <app.apk> (must be the first argument)")
	}
	_ = fs.Parse(rest)
	if *out == "" {
		die(20, "protect: --out required")
	}
	pol := resolvePolicy(*policyPath, *preset)
	// CWE-214: the password is never a CLI flag. Source it from a file or env.
	res, err := apk.Protect(apk.Options{
		Input: input, Output: *out, Policy: pol,
		Keystore: *ks, KsPassFile: *ksPassFile, KsPass: os.Getenv("SHIELD_KS_PASS"), KeyAlias: *ksAlias,
		Log: func(s string) { fmt.Fprintln(os.Stderr, "•", s) },
	})
	if err != nil {
		die(10, "protect: %v", err)
	}
	if res.MultidexRisk {
		fmt.Fprintf(os.Stderr, "warning: ~%d method refs — near the 64K single-DEX limit; ensure multidex is enabled.\n", res.MethodRefs)
	}
	fmt.Printf("protected %s -> %s (strings-enc=%d renamed=%d)\n",
		input, *out, res.StringsEncrypted, res.ClassesRenamed)
}

// ---- policy -------------------------------------------------------------

func cmdPolicy(args []string) {
	if len(args) < 2 {
		die(20, "policy: usage: shield policy show <preset> | validate <file>")
	}
	switch args[0] {
	case "show":
		printJSON(policy.Preset(args[1]))
	case "validate":
		p, err := policy.Load(args[1])
		if err != nil {
			die(20, "policy invalid: %v", err)
		}
		fmt.Printf("policy %q v%d: valid\n", p.Name, p.Version)
	default:
		die(20, "policy: unknown subcommand %q", args[0])
	}
}

// ---- helpers ------------------------------------------------------------

// splitSubject pulls the leading positional argument (a path) out so the rest
// can be parsed as flags. Go's flag package stops at the first non-flag token,
// so we accept the documented "path-first" form: `shield <cmd> <path> [flags]`.
func splitSubject(args []string) (subject string, rest []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", args
	}
	return args[0], args[1:]
}

func resolvePolicy(path, preset string) policy.Policy {
	if path != "" {
		p, err := policy.Load(path)
		if err != nil {
			die(20, "%v", err)
		}
		return p
	}
	p := policy.Preset(preset)
	if err := p.Validate(); err != nil {
		die(20, "%v", err)
	}
	return p
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func dotted(s string) string {
	if len(s) > 2 && s[0] == 'L' && s[len(s)-1] == ';' {
		return s
	}
	return s
}

func die(code int, format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(code)
}
