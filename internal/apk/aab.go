package apk

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/engine"
	"github.com/Martinez1991/shield-mobile-core/internal/manifest"
	"github.com/Martinez1991/shield-mobile-core/internal/policy"
)

// AAB (Android App Bundle) support (shield-platform.md section 4, issue #16). An
// .aab is a plain zip whose modules — the base module plus any dynamic feature
// modules — each carry their DEX under "<module>/dex/*.dex". SHIELD protects each
// module's DEX and repacks the bundle, copying every other entry (manifest,
// resources, BundleConfig.pb, signatures) through byte-for-byte.
//
// The DEX round-trip shells out to baksmali/smali (as the correctness gate does);
// the (un)packing is pure Go so it stays deterministic and offline-testable.
//
// A module's AndroidManifest.xml is protobuf-encoded (aapt2 XmlNode); its
// declared components are parsed into keep-rules (manifest.KeepClassesPB, #51) and
// fed to the rename pass, so renaming never breaks a manifest-referenced class.

// IsAAB reports whether path is an Android App Bundle: by extension, or by the
// presence of the BundleConfig.pb marker inside the zip.
func IsAAB(path string) bool {
	if strings.EqualFold(filepath.Ext(path), ".aab") {
		return true
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return false
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == "BundleConfig.pb" {
			return true
		}
	}
	return false
}

// moduleOfDex returns the module name for a zip entry that is a module DEX
// ("base/dex/classes.dex" -> "base"), or ok=false otherwise.
func moduleOfDex(name string) (string, bool) {
	name = strings.TrimPrefix(name, "./")
	parts := strings.Split(name, "/")
	if len(parts) == 3 && parts[1] == "dex" &&
		strings.HasPrefix(parts[2], "classes") && strings.HasSuffix(parts[2], ".dex") {
		return parts[0], true
	}
	return "", false
}

// moduleOfManifest returns the module for a protobuf manifest entry
// ("base/manifest/AndroidManifest.xml" -> "base"), or ok=false.
func moduleOfManifest(name string) (string, bool) {
	name = strings.TrimPrefix(name, "./")
	parts := strings.Split(name, "/")
	if len(parts) == 3 && parts[1] == "manifest" && parts[2] == "AndroidManifest.xml" {
		return parts[0], true
	}
	return "", false
}

// protectAAB runs the bundle round-trip: protect each module's DEX and repack.
func protectAAB(o Options) (*engine.Result, error) {
	for _, t := range []string{"baksmali", "smali"} {
		if !ToolAvailable(t) {
			return nil, fmt.Errorf("%s not found on PATH.\n"+
				"AAB protection needs baksmali and smali (https://github.com/baksmali/smali)\n"+
				"for the DEX round-trip. Install them, or protect an APK with apktool instead", t)
		}
	}
	work := o.WorkDir
	if work == "" {
		w, err := os.MkdirTemp("", "shield-aab-*")
		if err != nil {
			return nil, err
		}
		work = w
		defer os.RemoveAll(work)
	}

	zr, err := zip.OpenReader(o.Input)
	if err != nil {
		return nil, fmt.Errorf("open aab: %w", err)
	}
	defer zr.Close()

	// Group DEX entries by module, and index each module's protobuf manifest.
	modules := map[string][]*zip.File{}
	manifests := map[string]*zip.File{}
	var order []string
	for _, f := range zr.File {
		if m, ok := moduleOfDex(f.Name); ok {
			if _, seen := modules[m]; !seen {
				order = append(order, m)
			}
			modules[m] = append(modules[m], f)
		}
		if m, ok := moduleOfManifest(f.Name); ok {
			manifests[m] = f
		}
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("no module DEX (<module>/dex/classes*.dex) found in %s", o.Input)
	}

	sub := map[string][]byte{} // zip entry -> new content (nil = drop)
	agg := &engine.Result{}
	for _, mod := range order {
		o.logf("protecting module %q", mod)
		res, dex, err := protectModule(work, mod, modules[mod], manifests[mod], o.Policy, o.logf)
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", mod, err)
		}
		dexes := modules[mod]
		sub[dexes[0].Name] = dex      // replace the first DEX with the assembled one
		for _, f := range dexes[1:] { // drop any extra classesN.dex (consolidated)
			sub[f.Name] = nil
		}
		mergeResult(agg, res)
	}

	if err := rewriteAAB(o.Input, o.Output, sub); err != nil {
		return nil, err
	}
	o.logf("wrote %s", o.Output)
	return agg, nil
}

// protectModule disassembles a module's DEX, applies the engine, and reassembles
// a single DEX, returning its bytes. The module's protobuf manifest (if present)
// is parsed for component keep-rules so the rename pass never renames a class the
// manifest references by name.
func protectModule(work, mod string, dexes []*zip.File, manifestFile *zip.File, pol policy.Policy, logf func(string, ...any)) (*engine.Result, []byte, error) {
	root := filepath.Join(work, mod)
	smaliDir := filepath.Join(root, "smali")
	if err := os.MkdirAll(smaliDir, 0o755); err != nil {
		return nil, nil, err
	}
	for i, f := range dexes {
		dexPath := filepath.Join(root, fmt.Sprintf("in%d.dex", i))
		if err := extractZipFile(f, dexPath); err != nil {
			return nil, nil, err
		}
		if err := run("baksmali", "d", dexPath, "-o", smaliDir); err != nil {
			return nil, nil, fmt.Errorf("baksmali disassemble: %w", err)
		}
	}

	// Derive keep-rules from the protobuf manifest so renaming stays safe (#51).
	if manifestFile != nil {
		if keeps, err := manifestKeeps(manifestFile); err != nil {
			logf("module %q: manifest keep-rules skipped: %v", mod, err)
		} else if len(keeps) > 0 {
			pol.Rename.KeepClasses = append(append([]string(nil), pol.Rename.KeepClasses...), keeps...)
			logf("module %q: kept %d manifest components", mod, len(keeps))
		}
	}

	res, err := engine.Run(root, pol)
	if err != nil {
		return nil, nil, fmt.Errorf("obfuscation: %w", err)
	}

	outDex := filepath.Join(root, "out.dex")
	if err := run("smali", "a", smaliDir, "-o", outDex); err != nil {
		return nil, nil, fmt.Errorf("smali assemble: %w", err)
	}
	b, err := os.ReadFile(outDex)
	if err != nil {
		return nil, nil, err
	}
	return res, b, nil
}

// rewriteAAB copies inPath to outPath, replacing entries named in sub with the
// given bytes (or dropping them when the bytes are nil). Untouched entries are
// copied raw, preserving their compression and metadata exactly.
func rewriteAAB(inPath, outPath string, sub map[string][]byte) error {
	zr, err := zip.OpenReader(inPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)

	for _, f := range zr.File {
		content, replaced := sub[f.Name]
		if replaced {
			if content == nil {
				continue // dropped
			}
			hdr := f.FileHeader
			hdr.Method = zip.Deflate
			hdr.CompressedSize64 = 0
			hdr.CRC32 = 0
			w, err := zw.CreateHeader(&hdr)
			if err != nil {
				zw.Close()
				return err
			}
			if _, err := w.Write(content); err != nil {
				zw.Close()
				return err
			}
			continue
		}
		// Copy the entry verbatim (compressed bytes + header), preserving it.
		w, err := zw.CreateRaw(&f.FileHeader)
		if err != nil {
			zw.Close()
			return err
		}
		rc, err := f.OpenRaw()
		if err != nil {
			zw.Close()
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			zw.Close()
			return err
		}
	}
	return zw.Close()
}

// manifestKeeps reads a protobuf AndroidManifest.xml zip entry and returns the
// fully-qualified names of the components the rename pass must not touch.
func manifestKeeps(f *zip.File) ([]string, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return manifest.KeepClassesPB(b)
}

func extractZipFile(f *zip.File, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// mergeResult accumulates per-module engine results into agg.
func mergeResult(agg, r *engine.Result) {
	agg.StringsEncrypted += r.StringsEncrypted
	agg.ClassesRenamed += r.ClassesRenamed
	agg.MembersRenamed += r.MembersRenamed
	agg.MethodsVirtual += r.MethodsVirtual
	agg.MethodsFlattened += r.MethodsFlattened
	agg.MethodsReordered += r.MethodsReordered
	agg.OpaquePredicates += r.OpaquePredicates
	agg.MethodRefs += r.MethodRefs
	if r.RASPInjected {
		agg.RASPInjected = true
	}
	if r.MultidexRisk {
		agg.MultidexRisk = true
	}
	agg.Mapping = append(agg.Mapping, r.Mapping...)
}
