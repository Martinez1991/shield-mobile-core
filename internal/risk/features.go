// Package risk scores how much each method is worth protecting, so the Planner
// can spend expensive techniques (code virtualization, flattening) on the hot
// spots instead of uniformly (shield-platform.md §2.2 stage 8; issue #65). This
// is the v0 heuristic: deterministic static features + explainable weights, no
// ML — a stdlib-only baseline the engine can rely on.
package risk

import (
	"regexp"
	"strings"

	"shield/internal/analyze"
	"shield/internal/ir"
)

// MethodFeatures are deterministic static signals extracted from one method.
type MethodFeatures struct {
	Name           string `json:"name"`
	Instructions   int    `json:"instructions"`   // size
	Branches       int    `json:"branches"`       // jumps + switches (complexity proxy)
	Calls          int    `json:"calls"`          // invoke-*
	SensitiveCalls int    `json:"sensitiveCalls"` // calls into crypto/keystore/net/reflection/exec
	ConstStrings   int    `json:"constStrings"`
	SecretHits     int    `json:"secretHits"` // const-string literals that look like secrets
}

// sensitiveOwners: package/class prefixes whose use signals security-relevant
// logic worth protecting.
var sensitiveOwners = []string{
	"Ljavax/crypto/", "Ljava/security/", "Landroid/security/",
	"Ljava/net/", "Ljavax/net/ssl/", "Lokhttp3/", "Lretrofit2/",
	"Ljava/lang/reflect/", "Ldalvik/system/", // DexClassLoader, PathClassLoader
}

// sensitiveMethods: specific method refs beyond an owner prefix.
var sensitiveMethods = []string{
	"Ljava/lang/Runtime;->exec", "Ljava/lang/ProcessBuilder;",
}

var constStringLitRE = regexp.MustCompile(`^\s*const-string(?:/jumbo)?\s+[vp]\d+,\s*"(.*)"\s*$`)

// Analyze extracts features from a method block (from ".method ..." to
// ".end method"). Unparseable methods yield a zero-value MethodFeatures.
func Analyze(block []string) MethodFeatures {
	var f MethodFeatures
	if m, ok := ir.ParseMethod(block); ok {
		f.Name = m.Name
		f.Instructions = len(m.Insns)
		for _, insn := range m.Insns {
			op := insn.Op
			switch {
			case strings.HasPrefix(op, "if-"), strings.HasPrefix(op, "goto"),
				op == "packed-switch", op == "sparse-switch":
				f.Branches++
			case strings.HasPrefix(op, "invoke-"):
				f.Calls++
				if len(insn.Args) > 0 && isSensitiveCall(insn.Args[len(insn.Args)-1]) {
					f.SensitiveCalls++
				}
			case op == "const-string" || op == "const-string/jumbo":
				f.ConstStrings++
			}
		}
	}
	// Secret density from the raw const-string literals (the typed IR mangles a
	// quoted literal, so scan the original lines for it).
	for _, ln := range block {
		if mm := constStringLitRE.FindStringSubmatch(ln); mm != nil && analyze.LooksSecret(mm[1]) {
			f.SecretHits++
		}
	}
	return f
}

// isSensitiveCall reports whether an invoke's method reference
// ("Lowner;->name(params)ret") targets a security-relevant API.
func isSensitiveCall(ref string) bool {
	for _, p := range sensitiveOwners {
		if strings.HasPrefix(ref, p) {
			return true
		}
	}
	for _, p := range sensitiveMethods {
		if strings.HasPrefix(ref, p) {
			return true
		}
	}
	return false
}
