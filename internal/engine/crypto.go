package engine

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
)

// The string-encryption scheme (shield-platform.md section 3.3). Kept
// deliberately simple so it can be (a) reproduced exactly by the hand-written
// smali decryptor injected into the app and (b) unit-tested in pure Go.
//
// For each plaintext byte i:   cipher[i] = plain[i] XOR ((seed + i*step) & 0xFF)
// then the cipher bytes are standard-base64 encoded into an ASCII literal.
//
// XOR-with-rotating-keystream is the "low-risk" tier from the doc. AES-128/256-GCM
// with runtime-derived keys is the high-risk tier and is on the roadmap.

// deriveKey turns a 64-bit policy seed into the (seed8, step) pair the keystream
// uses. step is forced odd and non-zero so the keystream doesn't degenerate.
func deriveKey(seed int64) (seed8 int, step int) {
	s := uint64(seed)
	seed8 = int(s & 0xFF)
	step = int((s>>8)&0x7F) | 1 // 1..127, always odd
	return
}

func keystreamByte(seed8, step, i int) int {
	return (seed8 + i*step) & 0xFF
}

// EncodeString encrypts s and returns the base64 literal to embed in smali.
func EncodeString(s string, seed8, step int) string {
	b := []byte(s)
	c := make([]byte, len(b))
	for i := range b {
		c[i] = byte(int(b[i]) ^ keystreamByte(seed8, step, i))
	}
	return base64.StdEncoding.EncodeToString(c)
}

// DecodeString reverses EncodeString. Used by tests to prove the round-trip and
// mirrors exactly what the injected Lshield/rt/SH;->d method does at runtime.
func DecodeString(b64 string, seed8, step int) (string, error) {
	c, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	p := make([]byte, len(c))
	for i := range c {
		p[i] = byte(int(c[i]) ^ keystreamByte(seed8, step, i))
	}
	return string(p), nil
}

// ---- AES-256-GCM tier (section 3.3, high-risk strings) ------------------
//
// The AES key is never a literal in the binary: 32 bytes of key *material* are
// embedded and the runtime derives key = SHA-256(material). Each string uses a
// content-derived 12-byte nonce, so the same input+seed is reproducible (P2)
// without a global counter. The wire format is base64(nonce ‖ ciphertext ‖ tag),
// exactly what the injected smali decryptor consumes.

func seedBytes(seed int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(seed))
	return b
}

// aesKeyMaterial returns the 32 bytes the key is derived from (key =
// SHA-256(material)). NOTE: string "encryption" here is *reversible obfuscation*,
// not confidentiality — the material to reconstruct the key ships inside the
// artifact. It raises the bar against static extraction; it is not a secret an
// attacker with the APK cannot recover. Do not label it as confidentiality.
func aesKeyMaterial(seed int64) []byte {
	h := sha256.Sum256(append([]byte("shield-aes-km|"), seedBytes(seed)...))
	return h[:]
}

// aesMaskedMaterial returns the key material XOR a per-build keystream, so the
// *raw* material never appears as a single literal block in the DEX (issue #5).
// The injected decryptor unmasks it at runtime before hashing.
func aesMaskedMaterial(seed int64) []byte {
	raw := aesKeyMaterial(seed)
	s8, step := deriveKey(seed)
	out := make([]byte, len(raw))
	for i := range raw {
		out[i] = raw[i] ^ byte((s8+i*step)&0xFF)
	}
	return out
}

// aesUnmaskMaterial reverses aesMaskedMaterial (used by tests / mirrors smali).
func aesUnmaskMaterial(masked []byte, seed int64) []byte {
	s8, step := deriveKey(seed)
	out := make([]byte, len(masked))
	for i := range masked {
		out[i] = masked[i] ^ byte((s8+i*step)&0xFF)
	}
	return out
}

// aesKey derives the actual AES-256 key from the embedded material.
func aesKey(seed int64) []byte {
	m := aesKeyMaterial(seed)
	k := sha256.Sum256(m)
	return k[:]
}

func aesNonce(seed int64, plain string) []byte {
	h := sha256.Sum256(append(append(seedBytes(seed), '|'), plain...))
	return h[:12]
}

// EncodeStringAES encrypts s with AES-256-GCM and returns the base64 blob.
func EncodeStringAES(s string, seed int64) (string, error) {
	block, err := aes.NewCipher(aesKey(seed))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := aesNonce(seed, s)
	ct := gcm.Seal(nil, nonce, []byte(s), nil) // ciphertext ‖ 16-byte tag
	return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// DecodeStringAES reverses EncodeStringAES (used by tests; mirrors the smali).
func DecodeStringAES(blob string, seed int64) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(aesKey(seed))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce, body := raw[:12], raw[12:]
	pt, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
