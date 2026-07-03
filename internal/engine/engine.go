// Package engine is the SHIELD obfuscation core (shield-platform.md section 3).
// It runs an ordered set of protection passes over the smali IR, following the
// Protection Plan derived from a policy (the "Planner", section 2.2 stage 8).
package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"time"

	"shield/internal/manifest"
	"shield/internal/policy"
	"shield/internal/smali"
)

// Stage records how long one protection pass took (observability, issue #21).
// Callers turn these into per-stage latency metrics and spans.
type Stage struct {
	Name       string `json:"name"`
	DurationNS int64  `json:"durationNs"`
}

// dexMethodRefLimit is the Dalvik per-DEX method reference cap (64K).
const dexMethodRefLimit = 65536

// CacheVersion namespaces the content-addressed build cache. Bump it whenever a
// change alters the engine's output for the same input+policy, so stale cache
// entries are invalidated (issue #12).
const CacheVersion = "1"

// Result is the build evidence (section 2.2 stage 23: what was applied, where).
type Result struct {
	Policy           string     `json:"policy"`
	Seed             int64      `json:"seed"`
	ClassesTotal     int        `json:"classesTotal"`
	MetadataRemoved  int        `json:"metadataDirectivesRemoved"`
	StringsEncrypted int        `json:"stringsEncrypted"`
	ClassesRenamed   int        `json:"classesRenamed"`
	MembersRenamed   int        `json:"membersRenamed"`
	MethodsVirtual   int        `json:"methodsVirtualized"`
	MethodsFlattened int        `json:"methodsFlattened"`
	MethodsReordered int        `json:"methodsReordered"`
	OpaquePredicates int        `json:"opaquePredicates"`
	MethodsPadded    int        `json:"methodsPadded"`
	RASPInjected     bool       `json:"raspInjected"`
	ManifestKeeps    int        `json:"manifestKeeps"`
	MethodRefs       int        `json:"methodRefsEstimate"`
	MultidexRisk     bool       `json:"multidexRisk"`
	Applied          []string   `json:"appliedTechniques"`
	Stages           []Stage    `json:"stages,omitempty"`
	Mapping          []MapEntry `json:"-"`
}

// Run applies the policy's protection plan to the smali project rooted at root,
// rewriting files in place. Deterministic: same input+policy+engine => same output.
func Run(root string, p policy.Policy) (*Result, error) {
	classes, err := smali.LoadProject(root)
	if err != nil {
		return nil, err
	}
	res := &Result{Policy: p.Name, Seed: p.Seed, ClassesTotal: len(classes)}
	base := root
	if len(classes) > 0 {
		base = classes[0].Base
	}

	// stage times one protection pass into res.Stages (issue #21). The timing is
	// wall-clock and so absent from the deterministic on-disk output.
	stage := func(name string, fn func()) {
		start := time.Now()
		fn()
		res.Stages = append(res.Stages, Stage{Name: name, DurationNS: time.Since(start).Nanoseconds()})
	}

	// Reachability-aware keep-rules: policy keeps + components declared in the
	// AndroidManifest (never rename framework entry points). Missing manifest is
	// not an error (e.g. a raw smali dir).
	keep := append([]string(nil), p.Rename.KeepClasses...)
	if mk, err := manifest.KeepClasses(filepath.Join(root, "AndroidManifest.xml")); err == nil {
		keep = append(keep, mk...)
		res.ManifestKeeps = len(mk)
	}

	// RASP is injected FIRST so the subsequent passes obfuscate it too — its
	// detection signatures (su paths, "frida", Build strings, Xposed class) get
	// string-encrypted and its control flow flattened, instead of sitting in
	// plaintext (committee finding RC3). Its class + public method names stay
	// stable (Lshield/rt/RASP; is in neverRename) so the host API still resolves.
	if p.RASP.Enabled {
		classes = append(classes, RASPClass(base))
		res.RASPInjected = true
		res.Applied = append(res.Applied, "rasp")
	}

	// Plan order (section 2.2): metadata -> strings -> member-rename ->
	// class-rename -> control-flow -> junk. Member renaming runs before class
	// renaming so it can reason about original, scoped descriptors.
	if p.Metadata.Enabled {
		stage("metadata", func() {
			res.MetadataRemoved = passMetadata(classes)
			res.Applied = append(res.Applied, "metadata-removal")
		})
	}
	// Code virtualization runs before string encryption so it can lift a
	// plaintext const-string into the VM's string pool (otherwise encryption
	// would already have rewritten it into a decrypt call, and the method would
	// no longer be virtualizable). The pool literals the wrapper emits are then
	// encrypted by passStrings below — defense in depth. It also runs before
	// class renaming (needs original scoped descriptors); the interpreter class
	// is injected last, pristine.
	var vmClass *smali.Class
	if p.VM.Enabled {
		stage("virtualize", func() {
			n, vc := passVirtualize(classes, p.Rename.IncludePrefixes, p.Seed, base)
			res.MethodsVirtual = n
			vmClass = vc
			if n > 0 {
				res.Applied = append(res.Applied, "code-virtualization")
			}
		})
	}
	// Control-flow flattening runs after virtualization (VM claims its methods
	// first; flatten skips the wrappers) and before string encryption/renaming.
	// A flattened method contains a packed-switch, which reorder bails on, so the
	// control-flow passes stay disjoint.
	if p.ControlFlow.Flatten {
		stage("flatten", func() {
			res.MethodsFlattened = passFlatten(classes, p.Rename.IncludePrefixes, p.Seed)
			if res.MethodsFlattened > 0 {
				res.Applied = append(res.Applied, "control-flow-flattening")
			}
		})
	}
	if p.Strings.Enabled {
		stage("strings", func() {
			algo := p.Strings.Algorithm
			if algo == "" {
				algo = "xor"
			}
			res.StringsEncrypted = passStrings(classes, p.Strings.MinLength, algo, p.Seed)
			if res.StringsEncrypted > 0 {
				classes = append(classes, DecryptorClass(base, algo, p.Seed))
			}
			res.Applied = append(res.Applied, "string-encryption("+algo+")")
		})
	}
	if p.Rename.Enabled && p.Rename.Members {
		stage("rename-members", func() {
			res.MembersRenamed = passRenameMembers(classes, p.Rename.IncludePrefixes, keep)
			res.Applied = append(res.Applied, "member-renaming")
		})
	}
	if p.Rename.Enabled {
		stage("rename-classes", func() {
			renameMap := passRename(classes, p.Rename.IncludePrefixes, keep)
			res.ClassesRenamed = len(renameMap)
			res.Mapping = mappingFrom(renameMap)
			res.Applied = append(res.Applied, "identifier-renaming")
		})
	}
	if p.ControlFlow.Reorder {
		stage("reorder", func() {
			res.MethodsReordered = passReorder(classes, p.Seed)
			res.Applied = append(res.Applied, "block-reordering")
		})
	}
	if p.ControlFlow.Enabled {
		stage("opaque-predicates", func() {
			res.OpaquePredicates = passControlFlow(classes, p.Seed)
			res.Applied = append(res.Applied, "opaque-predicates")
		})
	}
	if p.Junk.Enabled {
		stage("junk", func() {
			res.MethodsPadded = passJunk(classes, p.Junk.Nops)
			res.Applied = append(res.Applied, "junk-code")
		})
	}
	// The VM interpreter is injected last (kept pristine for now).
	if vmClass != nil {
		classes = append(classes, vmClass)
	}

	// Multidex guard (Dalvik 64K method-reference limit). Injected runtime
	// classes and added references can push a single-DEX app over the cap.
	res.MethodRefs = estimateMethodRefs(classes)
	res.MultidexRisk = res.MethodRefs > dexMethodRefLimit*95/100

	if err := persist(classes); err != nil {
		return nil, err
	}
	return res, nil
}

var invokeRefRE = regexp.MustCompile(`L[\w$/]+;->[\w$<>]+\([^)]*\)\[*[\w$/;]+`)

// estimateMethodRefs approximates the number of distinct method references in
// the resulting DEX (defined methods + invoke targets). It is an estimate, not
// an exact DEX method_ids count, but a good early-warning for multidex.
func estimateMethodRefs(classes []*smali.Class) int {
	set := make(map[string]struct{})
	for _, c := range classes {
		for _, ln := range c.Lines {
			if m := methodDeclRE.FindStringSubmatch(ln); m != nil {
				set[c.Descriptor+"->"+m[3]+m[4]] = struct{}{}
				continue
			}
			for _, ref := range invokeRefRE.FindAllString(ln, -1) {
				set[ref] = struct{}{}
			}
		}
	}
	return len(set)
}

// persist writes every class to its (possibly renamed) path and removes files
// left behind by renames.
func persist(classes []*smali.Class) error {
	writes := make(map[string][]byte, len(classes))
	var deletes []string
	for _, c := range classes {
		newPath := filepath.Join(c.Base, smali.DescToRelPath(c.Descriptor))
		if err := c.Save(newPath); err != nil {
			return err
		}
		writes[newPath] = nil
		if c.Path != "" && c.Path != newPath {
			deletes = append(deletes, c.Path)
		}
	}
	for _, p := range deletes {
		if _, kept := writes[p]; kept {
			continue
		}
		_ = os.Remove(p)
	}
	return nil
}
