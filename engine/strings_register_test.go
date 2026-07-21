package engine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// A non-range invoke encodes its register in 4 bits (v0–v15). const-string allows
// an 8-bit register, so a string sitting in v16+ (or a param register) must have
// its decryptor call emitted as invoke-static/range, else smali assembly fails.
func TestDecryptInvokeRangeForHighRegisters(t *testing.T) {
	c := &smali.Class{
		Descriptor: "Lcom/x/Y;",
		Lines: []string{
			`    const-string v3, "alpha value"`,
			`    const-string v17, "bravo value"`,
			`    const-string p2, "charlie value"`,
		},
	}
	if n := passStrings([]*smali.Class{c}, 2, "xor", 0x5117e1d); n != 3 {
		t.Fatalf("encrypted %d strings, want 3", n)
	}
	joined := strings.Join(c.Lines, "\n")
	if !strings.Contains(joined, "invoke-static {v3},") {
		t.Errorf("v3 (low) should use a non-range invoke:\n%s", joined)
	}
	if !strings.Contains(joined, "invoke-static/range {v17 .. v17},") {
		t.Errorf("v17 (high) should use /range:\n%s", joined)
	}
	if !strings.Contains(joined, "invoke-static/range {p2 .. p2},") {
		t.Errorf("p2 (param) should use /range:\n%s", joined)
	}
}

// TestStringEncryptHighRegisterAssembles reproduces the real-world failure (a
// string constant in v16+, as generated protobuf classes have) and proves the
// encrypted output assembles. Runs only when SHIELD_SMALI_JAR is set. Without the
// /range fix, smali rejects it with "Invalid register: v17. Must be between v0
// and v15, inclusive".
func TestStringEncryptHighRegisterAssembles(t *testing.T) {
	jar := os.Getenv("SHIELD_SMALI_JAR")
	if jar == "" {
		t.Skip("set SHIELD_SMALI_JAR to a smali assembler jar to run the DEX round-trip")
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not on PATH")
	}

	c := &smali.Class{
		Descriptor: "Lcom/x/Hi;",
		Lines: []string{
			".class public Lcom/x/Hi;",
			".super Ljava/lang/Object;",
			"",
			".method public static big()Ljava/lang/String;",
			"    .registers 20",
			`    const-string v17, "a sufficiently long secret literal"`,
			"    return-object v17",
			".end method",
		},
	}
	passStrings([]*smali.Class{c}, 2, "aes", 0x5117e1d)

	dir := t.TempDir()
	writeClass(t, dir, "com/x/Hi.smali", c.Lines)
	dex := filepath.Join(t.TempDir(), "out.dex")
	if out, err := exec.Command("java", "-jar", jar, "a", dir, "-o", dex).CombinedOutput(); err != nil {
		t.Fatalf("smali rejected high-register string-encrypt output: %v\n%s", err, out)
	}
}
