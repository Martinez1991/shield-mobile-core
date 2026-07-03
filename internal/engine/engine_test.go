package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"shield/internal/policy"
)

const cardValidatorSmali = `.class public Lcom/bank/pay/CardValidator;
.super Ljava/lang/Object;
.source "CardValidator.java"

.method public validate(Ljava/lang/String;)Z
    .registers 4
    .param p1, "card"    # Ljava/lang/String;
    .prologue
    .line 12
    const-string v0, "sk_live_9f2a3b4c5d6e"
    .line 13
    invoke-static {}, Lcom/bank/pay/Helper;->ping()V
    .line 14
    const/4 v1, 0x1
    return v1
.end method
`

const helperSmali = `.class public Lcom/bank/pay/Helper;
.super Ljava/lang/Object;

.method public static ping()V
    .registers 1
    const-string v0, "ok"
    return-void
.end method
`

func writeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	base := filepath.Join(root, "smali", "com", "bank", "pay")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	must(t, filepath.Join(base, "CardValidator.smali"), cardValidatorSmali)
	must(t, filepath.Join(base, "Helper.smali"), helperSmali)
	return root
}

func must(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testPolicy() policy.Policy {
	p := policy.Default()
	p.Name = "test"
	p.Metadata.Enabled = true
	p.Strings.Enabled = true
	p.Strings.MinLength = 2
	p.Rename.Enabled = true
	p.Rename.IncludePrefixes = []string{"com/bank"}
	p.Junk.Enabled = true
	p.Junk.Nops = 2
	p.Seed = 0x5117e1d
	return p
}

func TestRunFullPipeline(t *testing.T) {
	root := writeProject(t)
	res, err := Run(root, testPolicy())
	if err != nil {
		t.Fatal(err)
	}

	// 2 original classes + injected decryptor.
	if res.ClassesRenamed != 2 {
		t.Errorf("ClassesRenamed = %d, want 2", res.ClassesRenamed)
	}
	if res.StringsEncrypted != 2 {
		t.Errorf("StringsEncrypted = %d, want 2", res.StringsEncrypted)
	}
	if res.MetadataRemoved == 0 {
		t.Error("expected metadata directives removed")
	}
	if res.MethodsPadded == 0 {
		t.Error("expected methods padded with nops")
	}

	smaliDir := filepath.Join(root, "smali")

	// Original files must be gone (renamed) and new opaque paths present.
	if _, err := os.Stat(filepath.Join(smaliDir, "com", "bank", "pay", "CardValidator.smali")); err == nil {
		t.Error("original CardValidator.smali should have been renamed away")
	}
	// Decryptor injected.
	if _, err := os.Stat(filepath.Join(smaliDir, "shield", "rt", "SH.smali")); err != nil {
		t.Errorf("decryptor SH.smali not written: %v", err)
	}

	// Find the renamed CardValidator (contains the invoke to Helper).
	body := readRenamed(t, smaliDir)
	if strings.Contains(body, "sk_live_9f2a3b4c5d6e") {
		t.Error("plaintext secret still present after string encryption")
	}
	if strings.Contains(body, ".line ") || strings.Contains(body, ".source ") || strings.Contains(body, ".prologue") {
		t.Error("debug metadata still present")
	}
	if !strings.Contains(body, "Lshield/rt/SH;->d(Ljava/lang/String;)Ljava/lang/String;") {
		t.Error("decryptor call not injected")
	}
	if strings.Contains(body, "Lcom/bank/pay/Helper;") {
		t.Error("reference to Helper not rewritten by rename")
	}
	if !strings.Contains(body, "nop") {
		t.Error("expected nop padding")
	}

	// The encrypted secret must decrypt back to the original.
	seed8, step := deriveKey(testPolicy().Seed)
	cipher := firstEncryptedString(t, body)
	got, err := DecodeString(cipher, seed8, step)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sk_live_9f2a3b4c5d6e" {
		t.Errorf("decrypt = %q, want the original secret", got)
	}

	// Mapping present.
	foundMapping := false
	for _, e := range res.Mapping {
		if e.From == "com.bank.pay.CardValidator" {
			foundMapping = true
		}
	}
	if !foundMapping {
		t.Error("mapping missing CardValidator entry")
	}
}

func TestRunAESAndRASP(t *testing.T) {
	root := writeProject(t)
	p := testPolicy()
	p.Strings.Algorithm = "aes"
	p.RASP.Enabled = true
	res, err := Run(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if !res.RASPInjected {
		t.Error("RASP not injected")
	}
	smaliDir := filepath.Join(root, "smali")
	if _, err := os.Stat(filepath.Join(smaliDir, "shield", "rt", "RASP.smali")); err != nil {
		t.Errorf("RASP.smali not written: %v", err)
	}

	body := readRenamed(t, smaliDir)
	if strings.Contains(body, "sk_live_9f2a3b4c5d6e") {
		t.Error("plaintext secret present with AES")
	}
	blob := firstEncryptedString(t, body)
	got, err := DecodeStringAES(blob, p.Seed)
	if err != nil {
		t.Fatalf("AES decrypt: %v", err)
	}
	if got != "sk_live_9f2a3b4c5d6e" {
		t.Errorf("AES decrypt = %q, want original secret", got)
	}
}

func TestRunVirtualizes(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "smali", "com", "bank", "pay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `.class public Lcom/bank/pay/Calc;
.super Ljava/lang/Object;

.method public static mix(II)I
    .registers 4
    mul-int v0, p0, p0
    add-int v0, v0, p1
    add-int/lit8 v0, v0, 0x7
    return v0
.end method
`
	must(t, filepath.Join(dir, "Calc.smali"), src)

	p := policy.Default()
	p.VM.Enabled = true
	p.Rename.IncludePrefixes = []string{"com/bank"}
	p.Seed = 0x5117e1d
	res, err := Run(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.MethodsVirtual != 1 {
		t.Fatalf("MethodsVirtual = %d, want 1", res.MethodsVirtual)
	}
	if _, err := os.Stat(filepath.Join(root, "smali", "shield", "rt", "VM.smali")); err != nil {
		t.Errorf("VM interpreter not injected: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "Calc.smali"))
	if !strings.Contains(string(body), "Lshield/rt/VM;->run([B[J[Ljava/lang/Object;[Ljava/lang/String;)Ljava/lang/Object;") {
		t.Error("method body was not replaced by a VM call")
	}
	if strings.Contains(string(body), "mul-int v0, p0, p0") {
		t.Error("original arithmetic still present (not virtualized)")
	}
}

const riskFixture = `.class public Lcom/bank/pay/Vault;
.super Ljava/lang/Object;

.method public static enc(I)I
    .registers 3
    invoke-static {}, Ljavax/crypto/Cipher;->getInstance()V
    const-string v0, "sk_live_9f2a3b4c5d6e"
    if-lez p0, :a
    const/4 v0, 0x1
    return v0
    :a
    const/4 v0, 0x0
    return v0
.end method

.method public static add(II)I
    .registers 3
    add-int v0, p0, p1
    return v0
.end method
`

func riskProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "smali", "com", "bank", "pay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	must(t, filepath.Join(dir, "Vault.smali"), riskFixture)
	return root
}

func TestRunRiskDriven(t *testing.T) {
	// risk-driven: only the high-risk method (crypto call + secret + branch) is
	// virtualized; the trivial arithmetic method is left untouched.
	root := riskProject(t)
	p := policy.Default()
	p.VM.Enabled = true
	p.Rename.Enabled = false
	p.Strings.Enabled = false
	p.Rename.IncludePrefixes = []string{"com/bank"}
	p.Risk.Enabled = true
	p.Risk.Threshold = 0.3
	p.Seed = 0x5117e1d

	res, err := Run(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.MethodsVirtual != 1 {
		t.Fatalf("MethodsVirtual = %d, want 1 (only the high-risk enc)", res.MethodsVirtual)
	}
	body, _ := os.ReadFile(filepath.Join(root, "smali", "com", "bank", "pay", "Vault.smali"))
	s := string(body)
	if !strings.Contains(s, "Lshield/rt/VM;->run(") {
		t.Error("high-risk enc should have been virtualized")
	}
	if !strings.Contains(s, "add-int v0, p0, p1") {
		t.Error("low-risk add should be left untouched (not virtualized)")
	}

	// Explainability report (#70): both methods appear, sorted by score; enc is
	// protected with reasons, add is not.
	if len(res.RiskMap) != 2 {
		t.Fatalf("RiskMap has %d entries, want 2", len(res.RiskMap))
	}
	enc, add := res.RiskMap[0], res.RiskMap[1] // sorted by score desc
	if !strings.Contains(enc.Method, "enc") || !enc.Protected || enc.Technique != "vm" {
		t.Errorf("enc entry = %+v, want protected vm", enc)
	}
	if len(enc.Reasons) == 0 {
		t.Error("protected method should carry explaining reasons")
	}
	if enc.Score <= add.Score {
		t.Errorf("enc (%.2f) should outrank add (%.2f)", enc.Score, add.Score)
	}
	if !strings.Contains(add.Method, "add") || add.Protected {
		t.Errorf("add entry = %+v, want not protected", add)
	}
}

func TestRunUniformWithoutRisk(t *testing.T) {
	// without risk-driven, both methods are virtualized (uniform, current behavior).
	root := riskProject(t)
	p := policy.Default()
	p.VM.Enabled = true
	p.Rename.Enabled = false
	p.Strings.Enabled = false
	p.Rename.IncludePrefixes = []string{"com/bank"}
	p.Seed = 0x5117e1d

	res, err := Run(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.MethodsVirtual != 2 {
		t.Errorf("MethodsVirtual = %d, want 2 (uniform, no risk gate)", res.MethodsVirtual)
	}
}

func TestManifestKeepRulesPreventRename(t *testing.T) {
	root := t.TempDir()
	// Decoded-project layout: AndroidManifest.xml at root + smali/ tree.
	must(t, filepath.Join(root, "AndroidManifest.xml"), `<?xml version="1.0"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android" package="com.bank.app">
  <application android:name=".App">
    <activity android:name=".MainActivity"/>
  </application>
</manifest>`)
	dir := filepath.Join(root, "smali", "com", "bank", "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	must(t, filepath.Join(dir, "MainActivity.smali"),
		".class public Lcom/bank/app/MainActivity;\n.super Landroid/app/Activity;\n")
	must(t, filepath.Join(dir, "Helper.smali"),
		".class public Lcom/bank/app/Helper;\n.super Ljava/lang/Object;\n")

	p := policy.Default()
	p.Rename.Enabled = true
	p.Rename.IncludePrefixes = []string{"com/bank"}
	p.Strings.Enabled = false
	res, err := Run(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.ManifestKeeps != 2 { // App + MainActivity
		t.Errorf("ManifestKeeps = %d, want 2", res.ManifestKeeps)
	}
	// MainActivity is a manifest component -> must NOT be renamed (file stays).
	if _, err := os.Stat(filepath.Join(dir, "MainActivity.smali")); err != nil {
		t.Error("MainActivity was renamed despite being a manifest component")
	}
	// Helper is not in the manifest -> should be renamed away.
	if _, err := os.Stat(filepath.Join(dir, "Helper.smali")); err == nil {
		t.Error("Helper should have been renamed")
	}
	if res.ClassesRenamed != 1 {
		t.Errorf("ClassesRenamed = %d, want 1 (only Helper)", res.ClassesRenamed)
	}
}

func TestEstimateMethodRefs(t *testing.T) {
	root := writeProject(t)
	res, err := Run(root, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if res.MethodRefs <= 0 {
		t.Error("expected a positive method-ref estimate")
	}
	if res.MultidexRisk {
		t.Error("tiny fixture should not trip the multidex guard")
	}
}

func TestRunIsDeterministic(t *testing.T) {
	r1, r2 := writeProject(t), writeProject(t)
	if _, err := Run(r1, testPolicy()); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(r2, testPolicy()); err != nil {
		t.Fatal(err)
	}
	a := snapshot(t, filepath.Join(r1, "smali"))
	b := snapshot(t, filepath.Join(r2, "smali"))
	if a != b {
		t.Error("same input+policy+engine must yield identical output (P2)")
	}
}

func TestRenameWithoutScopeIsNoop(t *testing.T) {
	p := policy.Default()
	p.Rename.Enabled = true // no IncludePrefixes -> must be a safe no-op
	if err := p.Validate(); err != nil {
		t.Fatalf("unscoped rename should validate (no-op), got: %v", err)
	}
	if p.RenameScoped() {
		t.Error("RenameScoped should be false without includePrefixes")
	}
	root := writeProject(t)
	res, err := Run(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.ClassesRenamed != 0 {
		t.Errorf("expected 0 renamed without scope, got %d", res.ClassesRenamed)
	}
}

// --- helpers ---

var encStringRE = regexp.MustCompile(`const-string(?:/jumbo)?\s+[vp]\d+,\s+"([^"]*)"`)

func readRenamed(t *testing.T, smaliDir string) string {
	t.Helper()
	var found string
	_ = filepath.Walk(filepath.Join(smaliDir, "o"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), "->ping()V") {
			found = string(b)
		}
		return nil
	})
	if found == "" {
		t.Fatal("could not locate renamed CardValidator")
	}
	return found
}

func firstEncryptedString(t *testing.T, body string) string {
	t.Helper()
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		if strings.Contains(ln, "Lshield/rt/SH;->d(") && i > 0 {
			if m := encStringRE.FindStringSubmatch(lines[i-1]); m != nil {
				return m[1]
			}
		}
	}
	t.Fatal("no encrypted string found")
	return ""
}

func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		data, _ := os.ReadFile(path)
		b.WriteString(filepath.ToSlash(rel))
		b.WriteByte('\n')
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}
