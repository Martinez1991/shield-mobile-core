// Package engine is the SHIELD obfuscation core (shield-platform.md section 3).
// It runs an ordered set of protection passes over the smali IR, following the
// Protection Plan derived from a policy (the "Planner", section 2.2 stage 8).
package engine

import (
	"os"
	"path/filepath"

	"shield/internal/policy"
	"shield/internal/smali"
)

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
	MethodsReordered int        `json:"methodsReordered"`
	OpaquePredicates int        `json:"opaquePredicates"`
	MethodsPadded    int        `json:"methodsPadded"`
	RASPInjected     bool       `json:"raspInjected"`
	Applied          []string   `json:"appliedTechniques"`
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

	// Plan order (section 2.2): metadata -> strings -> member-rename ->
	// class-rename -> control-flow -> junk. Member renaming runs before class
	// renaming so it can reason about original, scoped descriptors.
	if p.Metadata.Enabled {
		res.MetadataRemoved = passMetadata(classes)
		res.Applied = append(res.Applied, "metadata-removal")
	}
	if p.Strings.Enabled {
		algo := p.Strings.Algorithm
		if algo == "" {
			algo = "xor"
		}
		res.StringsEncrypted = passStrings(classes, p.Strings.MinLength, algo, p.Seed)
		if res.StringsEncrypted > 0 {
			classes = append(classes, DecryptorClass(base, algo, p.Seed))
		}
		res.Applied = append(res.Applied, "string-encryption("+algo+")")
	}
	if p.Rename.Enabled && p.Rename.Members {
		res.MembersRenamed = passRenameMembers(classes, p.Rename.IncludePrefixes, p.Rename.KeepClasses)
		res.Applied = append(res.Applied, "member-renaming")
	}
	// Code virtualization runs before class renaming (needs original scoped
	// descriptors). The interpreter class is injected last, pristine.
	var vmClass *smali.Class
	if p.VM.Enabled {
		n, vc := passVirtualize(classes, p.Rename.IncludePrefixes, p.Seed, base)
		res.MethodsVirtual = n
		vmClass = vc
		if n > 0 {
			res.Applied = append(res.Applied, "code-virtualization")
		}
	}
	if p.Rename.Enabled {
		renameMap := passRename(classes, p.Rename.IncludePrefixes, p.Rename.KeepClasses)
		res.ClassesRenamed = len(renameMap)
		res.Mapping = mappingFrom(renameMap)
		res.Applied = append(res.Applied, "identifier-renaming")
	}
	if p.ControlFlow.Reorder {
		res.MethodsReordered = passReorder(classes, p.Seed)
		res.Applied = append(res.Applied, "block-reordering")
	}
	if p.ControlFlow.Enabled {
		res.OpaquePredicates = passControlFlow(classes, p.Seed)
		res.Applied = append(res.Applied, "opaque-predicates")
	}
	if p.Junk.Enabled {
		res.MethodsPadded = passJunk(classes, p.Junk.Nops)
		res.Applied = append(res.Applied, "junk-code")
	}
	// Runtime classes are injected last so they stay pristine (no opaque/junk).
	if p.RASP.Enabled {
		classes = append(classes, RASPClass(base))
		res.RASPInjected = true
		res.Applied = append(res.Applied, "rasp")
	}
	if vmClass != nil {
		classes = append(classes, vmClass)
	}

	if err := persist(classes); err != nil {
		return nil, err
	}
	return res, nil
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
