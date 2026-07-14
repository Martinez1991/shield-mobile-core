package nativesvc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"archive/zip"

	"shield/internal/native"
)

// Archive orchestration (issue #82/#64): the Go worker connects native-svc to the
// APK/AAB flow. LLVM passes need bitcode, and a stripped, already-linked .so has
// none — so a *recompilable* native module is shipped as a bitcode sidecar,
// "lib/<abi>/<name>.so.bc". ProtectArchive transforms each with native-svc, links
// it back to a .so, optionally applies the anti-tamper post-link patch, writes it
// as "lib/<abi>/<name>.so" (dropping the .bc), and copies every other entry
// byte-for-byte. Plain .so with no sidecar are left untouched and reported.
//
// The link and patch steps need a toolchain, so they are injected (Linker/Patcher)
// — real shell-outs in production, fakes in tests — keeping the orchestration
// itself deterministic and offline-testable.

// Linker turns protected bitcode into a linked shared object for an ABI.
type Linker func(abi string, bitcode []byte) (so []byte, err error)

// Patcher applies the post-link anti-tamper patch to a .so, returning the patched
// bytes. Required only when the tamper pass is configured.
type Patcher func(so []byte) ([]byte, error)

// ArchiveResult reports what ProtectArchive did.
type ArchiveResult struct {
	Protected []string `json:"protected"` // .so entries produced from bitcode
	Skipped   []string `json:"skipped"`   // native libs left as-is (no bitcode)
}

// isBitcodeSidecar reports whether a zip entry is a recompilable native module
// (a "lib/<abi>/<name>.so.bc" bitcode sidecar) and returns its ABI and .so name.
func isBitcodeSidecar(name string) (abi, soName string, ok bool) {
	if !strings.HasSuffix(name, ".so.bc") {
		return "", "", false
	}
	soName = strings.TrimSuffix(name, ".bc")
	abi, ok = native.NativeLib(soName)
	return abi, soName, ok
}

// ProtectArchive protects the recompilable native modules of an APK/AAB at inPath
// and writes the result to outPath. link is required; patch is required iff the
// tamper pass is configured. Returns what was protected vs skipped.
func (s *Service) ProtectArchive(inPath, outPath string, link Linker, patch Patcher) (*ArchiveResult, error) {
	if link == nil {
		return nil, fmt.Errorf("ProtectArchive: a Linker is required")
	}
	needsPatch := false
	for _, p := range s.cfg.Passes {
		if p == PassTamper {
			needsPatch = true
		}
	}
	if needsPatch && patch == nil {
		return nil, fmt.Errorf("ProtectArchive: the tamper pass requires a Patcher")
	}

	zr, err := zip.OpenReader(inPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	sub := map[string][]byte{}
	res := &ArchiveResult{}
	for _, f := range zr.File {
		abi, soName, ok := isBitcodeSidecar(f.Name)
		if !ok {
			continue
		}
		bc, err := readZipEntry(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		prot, err := s.Apply(abi, bc)
		if err != nil {
			return nil, fmt.Errorf("transform %s: %w", f.Name, err)
		}
		so, err := link(abi, prot)
		if err != nil {
			return nil, fmt.Errorf("link %s: %w", soName, err)
		}
		if needsPatch {
			if so, err = patch(so); err != nil {
				return nil, fmt.Errorf("tamper-patch %s: %w", soName, err)
			}
		}
		sub[soName] = so  // replace/create the linked .so
		sub[f.Name] = nil // drop the bitcode sidecar
		res.Protected = append(res.Protected, soName)
	}

	// Report native libs we left untouched (no recompilable bitcode).
	for _, f := range zr.File {
		if _, ok := native.NativeLib(f.Name); !ok || strings.HasSuffix(f.Name, ".bc") {
			continue
		}
		if _, replaced := sub[f.Name]; !replaced {
			res.Skipped = append(res.Skipped, f.Name)
		}
	}

	if err := rewriteZip(inPath, outPath, sub); err != nil {
		return nil, err
	}
	sort.Strings(res.Protected)
	sort.Strings(res.Skipped)
	return res, nil
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// rewriteZip copies inPath to outPath, replacing entries named in sub (nil drops
// the entry) and copying every other entry verbatim (raw compressed bytes +
// header), so untouched files are byte-for-byte preserved.
func rewriteZip(inPath, outPath string, sub map[string][]byte) error {
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

	seen := map[string]bool{}
	for _, f := range zr.File {
		seen[f.Name] = true
		if content, replaced := sub[f.Name]; replaced {
			if content == nil {
				continue // dropped
			}
			if err := writeDeflate(zw, f.FileHeader.Name, content); err != nil {
				zw.Close()
				return err
			}
			continue
		}
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

	// New entries in sub that weren't in the input (e.g. a .so created from a
	// bitcode sidecar that shipped without a prebuilt .so).
	newNames := make([]string, 0, len(sub))
	for name, content := range sub {
		if !seen[name] && content != nil {
			newNames = append(newNames, name)
		}
	}
	sort.Strings(newNames)
	for _, name := range newNames {
		if err := writeDeflate(zw, name, sub[name]); err != nil {
			zw.Close()
			return err
		}
	}
	return zw.Close()
}

func writeDeflate(zw *zip.Writer, name string, content []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return err
	}
	_, err = w.Write(content)
	return err
}

// abiTargets maps Android ABIs to the clang target triple used to link them for a
// plain GNU/Linux toolchain (the CI/host path). NDKLinker uses the NDK wrappers.
var abiTargets = map[string]string{
	"arm64-v8a":   "aarch64-linux-gnu",
	"armeabi-v7a": "arm-linux-gnueabihf",
	"x86_64":      "x86_64-linux-gnu",
	"x86":         "i686-linux-gnu",
}

// ClangLinker links bitcode into a shared object with a clang that cross-targets
// each ABI (the GNU triple). Suitable for the host/CI gate; production Android
// builds use NDKLinker.
func ClangLinker(clang string) Linker {
	return func(abi string, bitcode []byte) ([]byte, error) {
		target, ok := abiTargets[abi]
		if !ok {
			return nil, fmt.Errorf("no clang target for ABI %q", abi)
		}
		return linkBitcode(clang, []string{"--target=" + target}, bitcode)
	}
}

// NDKLinker links bitcode with the Android NDK per-ABI clang wrapper under
// ndkBin (…/toolchains/llvm/prebuilt/<host>/bin), for API level api.
func NDKLinker(ndkBin string, api int) Linker {
	triples := map[string]string{
		"arm64-v8a": "aarch64-linux-android", "armeabi-v7a": "armv7a-linux-androideabi",
		"x86_64": "x86_64-linux-android", "x86": "i686-linux-android",
	}
	return func(abi string, bitcode []byte) ([]byte, error) {
		t, ok := triples[abi]
		if !ok {
			return nil, fmt.Errorf("no NDK triple for ABI %q", abi)
		}
		cc := filepath.Join(ndkBin, fmt.Sprintf("%s%d-clang", t, api))
		return linkBitcode(cc, nil, bitcode)
	}
}

// linkBitcode writes bitcode to a temp file and links it into a .so with cc.
func linkBitcode(cc string, extra []string, bitcode []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "shield-link-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	bcPath := filepath.Join(dir, "in.bc")
	soPath := filepath.Join(dir, "out.so")
	if err := os.WriteFile(bcPath, bitcode, 0o600); err != nil {
		return nil, err
	}
	args := append([]string{"-shared", "-fPIC"}, extra...)
	args = append(args, bcPath, "-o", soPath)
	cmd := exec.Command(cc, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w: %s", cc, err, bytes.TrimSpace(stderr.Bytes()))
	}
	return os.ReadFile(soPath)
}

// ScriptPatcher runs the tamper-patch.py post-link patcher (python script) on a
// .so, returning the patched bytes.
func ScriptPatcher(python, script string) Patcher {
	return func(so []byte) ([]byte, error) {
		dir, err := os.MkdirTemp("", "shield-patch-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		soPath := filepath.Join(dir, "lib.so")
		if err := os.WriteFile(soPath, so, 0o600); err != nil {
			return nil, err
		}
		cmd := exec.Command(python, script, "patch", soPath)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("tamper-patch: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
		}
		return os.ReadFile(soPath)
	}
}
