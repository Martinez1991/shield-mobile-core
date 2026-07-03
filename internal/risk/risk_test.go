package risk

import (
	"strings"
	"testing"
)

const cryptoMethod = `.method public static enc(I)I
    .registers 3
    invoke-static {}, Ljavax/crypto/Cipher;->getInstance()V
    const-string v0, "sk_live_9f2a3b4c5d6e"
    if-lez p0, :a
    const/4 v0, 0x1
    return v0
    :a
    const/4 v0, 0x0
    return v0
.end method`

const getterMethod = `.method public static getX(I)I
    .registers 2
    const/4 v0, 0x5
    add-int v0, v0, p0
    return v0
.end method`

func lines(s string) []string { return strings.Split(s, "\n") }

func TestFeatures(t *testing.T) {
	f := Analyze(lines(cryptoMethod))
	if f.SensitiveCalls != 1 {
		t.Errorf("SensitiveCalls = %d, want 1 (javax.crypto)", f.SensitiveCalls)
	}
	if f.Calls != 1 {
		t.Errorf("Calls = %d, want 1", f.Calls)
	}
	if f.SecretHits != 1 {
		t.Errorf("SecretHits = %d, want 1 (sk_live token)", f.SecretHits)
	}
	if f.ConstStrings != 1 {
		t.Errorf("ConstStrings = %d, want 1", f.ConstStrings)
	}
	if f.Branches != 1 {
		t.Errorf("Branches = %d, want 1 (if-lez)", f.Branches)
	}

	g := Analyze(lines(getterMethod))
	if g.SensitiveCalls != 0 || g.SecretHits != 0 || g.Branches != 0 || g.Calls != 0 {
		t.Errorf("trivial getter has risk signals: %+v", g)
	}
}

func TestScoreOrders(t *testing.T) {
	enc, _ := Score(Analyze(lines(cryptoMethod)))
	get, _ := Score(Analyze(lines(getterMethod)))
	if !(enc > get) {
		t.Errorf("crypto method (%.2f) should outscore the getter (%.2f)", enc, get)
	}
	if get > 0.2 {
		t.Errorf("trivial getter scored %.2f, want ~0", get)
	}
	if enc <= 0.5 {
		t.Errorf("sensitive method scored only %.2f, want > 0.5", enc)
	}
}

func TestScoreMonotonic(t *testing.T) {
	base := MethodFeatures{Branches: 2, Calls: 3}
	more := base
	more.SensitiveCalls = 2
	bs, _ := Score(base)
	ms, _ := Score(more)
	if !(ms > bs) {
		t.Errorf("adding sensitive calls must raise the score: %.2f -> %.2f", bs, ms)
	}
}

func TestAssessReasons(t *testing.T) {
	a := Assess(lines(cryptoMethod))
	if len(a.Reasons) == 0 {
		t.Fatal("no reasons")
	}
	// the strongest signal (a sensitive call) leads the explanation.
	if !strings.Contains(a.Reasons[0], "sensível") {
		t.Errorf("top reason = %q, want the sensitive-call signal first", a.Reasons[0])
	}
	if a.Features.Name != "enc" {
		t.Errorf("assessment name = %q, want enc", a.Features.Name)
	}
}
