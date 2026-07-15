package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Martinez1991/shield-mobile-core/internal/policy"
	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// TestAssembleRoundTrip proves the obfuscated smali assembles into a valid DEX
// (Dalvik structural/register/control-flow verification). It runs only when a
// smali assembler fat jar is available via SHIELD_SMALI_JAR, so CI without the
// Android toolchain still passes. Get the jar from:
//
//	https://bitbucket.org/JesusFreke/smali/downloads/smali-2.5.2.jar
//
//	SHIELD_SMALI_JAR=/path/smali.jar go test ./internal/engine -run RoundTrip -v
func TestAssembleRoundTrip(t *testing.T) {
	jar := os.Getenv("SHIELD_SMALI_JAR")
	if jar == "" {
		t.Skip("set SHIELD_SMALI_JAR to a smali assembler jar to run the DEX round-trip")
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not on PATH")
	}

	root := writeProject(t)
	p := policy.Preset("prod-high")
	p.Rename.IncludePrefixes = []string{"com/bank"} // scope renaming for the fixture
	p.Seed = 0x5117e1d
	if _, err := Run(root, p); err != nil {
		t.Fatal(err)
	}

	dex := filepath.Join(t.TempDir(), "out.dex")
	cmd := exec.Command("java", "-jar", jar, "a", filepath.Join(root, "smali"), "-o", dex)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("smali assembler rejected obfuscated output: %v\n%s", err, out)
	}
	if fi, err := os.Stat(dex); err != nil || fi.Size() == 0 {
		t.Fatalf("expected a non-empty dex, err=%v", err)
	}
}

// TestVMAssembles validates that the generated interpreter and a virtualized
// method assemble to a valid DEX (Dalvik structural/register verification).
func TestVMAssembles(t *testing.T) {
	jar := os.Getenv("SHIELD_SMALI_JAR")
	if jar == "" {
		t.Skip("set SHIELD_SMALI_JAR to run the VM assembly check")
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not on PATH")
	}

	wire := vmPermutation(0x5117e1d)
	code, _, ok := compileMethod(strings.Split(vmPoly, "\n"), wire, nil)
	if !ok {
		t.Fatal("poly must be virtualizable")
	}

	dir := t.TempDir()
	pinfos, _, _ := parseParams("II")
	// virtualized class
	host := &smali.Class{
		Descriptor: "Lcom/x/Y;",
		Lines: append([]string{".class public Lcom/x/Y;", ".super Ljava/lang/Object;", ""},
			virtualizedBody(".method public static poly(II)I", pinfos, code, nil)...),
	}
	writeClass(t, dir, "com/x/Y.smali", host.Lines)
	vm := VMClass(dir, wire)
	writeClass(t, dir, "github.com/Martinez1991/shield-mobile-core/rt/VM.smali", vm.Lines)

	dex := filepath.Join(t.TempDir(), "vm.dex")
	cmd := exec.Command("java", "-jar", jar, "a", dir, "-o", dex)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("smali assembler rejected VM output: %v\n%s", err, out)
	}
}

func writeClass(t *testing.T, root, rel string, lines []string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}
