package engine

import "testing"

func TestEncodeDecodeRoundTrip(t *testing.T) {
	seed8, step := deriveKey(0x5117e1d)
	cases := []string{
		"sk_live_9f2a3b4c5d",
		"",
		"a",
		"pix transfer 100.00",
		"unicode: café ☂ 日本語",
		"line1\nline2\ttab",
	}
	for _, in := range cases {
		enc := EncodeString(in, seed8, step)
		got, err := DecodeString(enc, seed8, step)
		if err != nil {
			t.Fatalf("decode(%q): %v", in, err)
		}
		if got != in {
			t.Errorf("round-trip mismatch: in=%q enc=%q got=%q", in, enc, got)
		}
	}
}

func TestEncodeIsDeterministic(t *testing.T) {
	s8, st := deriveKey(42)
	if EncodeString("hello world", s8, st) != EncodeString("hello world", s8, st) {
		t.Fatal("encoding must be deterministic for a fixed seed (P2)")
	}
}

func TestEncodeChangesWithSeed(t *testing.T) {
	a8, aStep := deriveKey(1)
	b8, bStep := deriveKey(2)
	if EncodeString("payload", a8, aStep) == EncodeString("payload", b8, bStep) {
		t.Fatal("different seeds should produce different ciphertext")
	}
}

func TestAESRoundTrip(t *testing.T) {
	var seed int64 = 0x5117e1d
	cases := []string{"sk_live_9f2a3b4c5d6e", "", "a", "café ☂ 日本語", "pix 100.00"}
	for _, in := range cases {
		blob, err := EncodeStringAES(in, seed)
		if err != nil {
			t.Fatalf("encode(%q): %v", in, err)
		}
		got, err := DecodeStringAES(blob, seed)
		if err != nil {
			t.Fatalf("decode(%q): %v", in, err)
		}
		if got != in {
			t.Errorf("AES round-trip: in=%q got=%q", in, got)
		}
	}
}

func TestAESKeyNeverLiteral(t *testing.T) {
	// The embedded material must differ from the derived key (key is SHA-256 of it),
	// so the actual AES key is never a literal in the binary.
	seed := int64(42)
	mat := aesKeyMaterial(seed)
	key := aesKey(seed)
	if string(mat) == string(key) {
		t.Error("key must be derived from material, not equal to it")
	}
	if len(key) != 32 {
		t.Errorf("AES-256 key must be 32 bytes, got %d", len(key))
	}
}

func TestAESIsDeterministic(t *testing.T) {
	a, _ := EncodeStringAES("payload", 7)
	b, _ := EncodeStringAES("payload", 7)
	if a != b {
		t.Error("AES encoding must be deterministic for a fixed seed (P2)")
	}
}

func TestDeriveKeyStepIsOdd(t *testing.T) {
	for _, seed := range []int64{0, 1, 255, 256, 0x5117e1d, -1} {
		_, step := deriveKey(seed)
		if step%2 == 0 || step < 1 || step > 127 {
			t.Errorf("seed %d: step %d out of range/even", seed, step)
		}
	}
}
