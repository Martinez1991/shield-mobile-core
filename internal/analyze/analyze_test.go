package analyze

import (
	"os"
	"path/filepath"
	"testing"
)

const sample = `.class public Lcom/bank/pay/Secrets;
.super Ljava/lang/Object;

.method public static keys()V
    .registers 2
    const-string v0, "sk_live_9f2a3b4c5d6e7f8a"
    const-string v0, "hello"
    const-string v1, "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcd1234"
    return-void
.end method
`

func TestAnalyzeDetectsSecrets(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "smali", "com", "bank", "pay")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Secrets.smali"), []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Classes != 1 || rep.Methods != 1 {
		t.Errorf("classes=%d methods=%d, want 1/1", rep.Classes, rep.Methods)
	}
	if rep.Strings != 3 {
		t.Errorf("strings=%d, want 3", rep.Strings)
	}

	kinds := map[string]bool{}
	for _, f := range rep.Findings {
		kinds[f.Kind] = true
		if f.Preview == "sk_live_9f2a3b4c5d6e7f8a" {
			t.Error("finding preview must not contain the raw secret")
		}
	}
	if !kinds["stripe-secret-key"] {
		t.Error("expected stripe key detection")
	}
	if !kinds["jwt"] {
		t.Error("expected JWT detection")
	}
}

func TestShannon(t *testing.T) {
	if e := shannon("aaaaaaaa"); e != 0 {
		t.Errorf("uniform string entropy = %f, want 0", e)
	}
	if e := shannon("abcdefgh"); e <= 2.9 {
		t.Errorf("diverse string entropy = %f, want ~3", e)
	}
}
