package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Martinez1991/shield-mobile-core/internal/smali"
)

// decryptorDescriptor is the injected runtime helper (kept out of renaming).
const decryptorDescriptor = "Lshield/rt/SH;"

var constStringRE = regexp.MustCompile(`^(\s*)const-string(/jumbo)?\s+([vp]\d+),\s+"(.*)"\s*$`)

// passStrings encrypts eligible string literals (section 3.3). Each
// `const-string vX, "secret"` becomes a call to the injected decryptor so the
// plaintext never appears in the DEX. algo is "xor" or "aes". Returns count.
func passStrings(classes []*smali.Class, minLen int, algo string, seed int64) int {
	seed8, step := deriveKey(seed)
	encode := func(v string) string {
		if algo == "aes" {
			enc, err := EncodeStringAES(v, seed)
			if err == nil {
				return enc
			}
			// fall back to xor on the (practically impossible) AES setup error
		}
		return EncodeString(v, seed8, step)
	}
	count := 0
	for _, c := range classes {
		if c.Descriptor == decryptorDescriptor {
			continue
		}
		var out []string
		for _, ln := range c.Lines {
			m := constStringRE.FindStringSubmatch(ln)
			if m == nil {
				out = append(out, ln)
				continue
			}
			indent, jumbo, reg, lit := m[1], m[2], m[3], m[4]
			val, ok := unescapeSmali(lit)
			if !ok || len([]rune(val)) < minLen {
				out = append(out, ln)
				continue
			}
			enc := encode(val)
			out = append(out,
				fmt.Sprintf(`%sconst-string%s %s, "%s"`, indent, jumbo, reg, enc),
				fmt.Sprintf(`%sinvoke-static {%s}, %s->d(Ljava/lang/String;)Ljava/lang/String;`, indent, reg, decryptorDescriptor),
				fmt.Sprintf(`%smove-result-object %s`, indent, reg),
			)
			count++
		}
		c.Lines = out
	}
	return count
}

// DecryptorClass builds the injected Lshield/rt/SH; class as a smali.Class for
// the given algorithm ("xor" or "aes"). Its logic mirrors DecodeString /
// DecodeStringAES exactly.
func DecryptorClass(base, algo string, seed int64) *smali.Class {
	var src string
	if algo == "aes" {
		src = aesDecryptorSrc(seed)
	} else {
		seed8, step := deriveKey(seed)
		src = xorDecryptorSrc(seed8, step)
	}
	return &smali.Class{
		Base:       base,
		Descriptor: decryptorDescriptor,
		Lines:      strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n"),
	}
}

func xorDecryptorSrc(seed8, step int) string {
	return fmt.Sprintf(`.class public Lshield/rt/SH;
.super Ljava/lang/Object;

# SHIELD runtime string decryptor (generated). Reverses XOR keystream + base64.
# seed8=%d step=%d

.method public static d(Ljava/lang/String;)Ljava/lang/String;
    .locals 5
    const/4 v0, 0x0
    invoke-static {p0, v0}, Landroid/util/Base64;->decode(Ljava/lang/String;I)[B
    move-result-object v0
    array-length v1, v0
    const/4 v2, 0x0
    :loop
    if-ge v2, v1, :done
    aget-byte v3, v0, v2
    mul-int/lit8 v4, v2, %d
    add-int/lit16 v4, v4, %d
    and-int/lit16 v4, v4, 0xff
    xor-int/2addr v3, v4
    int-to-byte v3, v3
    aput-byte v3, v0, v2
    add-int/lit8 v2, v2, 0x1
    goto :loop
    :done
    new-instance v3, Ljava/lang/String;
    const-string v4, "UTF-8"
    invoke-direct {v3, v0, v4}, Ljava/lang/String;-><init>([BLjava/lang/String;)V
    return-object v3
.end method
`, seed8, step, step, seed8)
}

// aesDecryptorSrc emits the AES-256-GCM decryptor. The AES key never appears as
// a literal: 32 bytes of *masked* key material are embedded, unmasked at runtime
// with a per-build XOR keystream, then hashed (key = SHA-256(material)). Blob
// layout is nonce(12) ‖ ciphertext ‖ tag(16). Validated to assemble to a valid
// DEX and to interop with the JVM crypto stack (scripts/validate-aes.sh).
func aesDecryptorSrc(seed int64) string {
	s8, step := deriveKey(seed)
	var arr strings.Builder
	for _, b := range aesMaskedMaterial(seed) {
		fmt.Fprintf(&arr, "        0x%02xt\n", b)
	}
	unmask := fmt.Sprintf(`    array-length v2, v6
    const/4 v0, 0x0
    :umloop
    if-ge v0, v2, :umdone
    aget-byte v1, v6, v0
    mul-int/lit8 v3, v0, %d
    add-int/lit16 v3, v3, %d
    and-int/lit16 v3, v3, 0xff
    xor-int/2addr v1, v3
    int-to-byte v1, v1
    aput-byte v1, v6, v0
    add-int/lit8 v0, v0, 0x1
    goto :umloop
    :umdone
`, step, s8)
	return `.class public Lshield/rt/SH;
.super Ljava/lang/Object;

# SHIELD runtime string decryptor (generated). AES-256-GCM, key = SHA-256(material).

.method public static d(Ljava/lang/String;)Ljava/lang/String;
    .locals 10
    const/4 v0, 0x0
    invoke-static {p0, v0}, Landroid/util/Base64;->decode(Ljava/lang/String;I)[B
    move-result-object v1
    const/4 v2, 0x0
    const/16 v3, 0xc
    invoke-static {v1, v2, v3}, Ljava/util/Arrays;->copyOfRange([BII)[B
    move-result-object v4
    array-length v5, v1
    invoke-static {v1, v3, v5}, Ljava/util/Arrays;->copyOfRange([BII)[B
    move-result-object v5
    const/16 v6, 0x20
    new-array v6, v6, [B
    fill-array-data v6, :km
` + unmask + `    const-string v7, "SHA-256"
    invoke-static {v7}, Ljava/security/MessageDigest;->getInstance(Ljava/lang/String;)Ljava/security/MessageDigest;
    move-result-object v7
    invoke-virtual {v7, v6}, Ljava/security/MessageDigest;->digest([B)[B
    move-result-object v6
    new-instance v7, Ljavax/crypto/spec/SecretKeySpec;
    const-string v8, "AES"
    invoke-direct {v7, v6, v8}, Ljavax/crypto/spec/SecretKeySpec;-><init>([BLjava/lang/String;)V
    new-instance v8, Ljavax/crypto/spec/GCMParameterSpec;
    const/16 v9, 0x80
    invoke-direct {v8, v9, v4}, Ljavax/crypto/spec/GCMParameterSpec;-><init>(I[B)V
    const-string v9, "AES/GCM/NoPadding"
    invoke-static {v9}, Ljavax/crypto/Cipher;->getInstance(Ljava/lang/String;)Ljavax/crypto/Cipher;
    move-result-object v9
    const/4 v0, 0x2
    invoke-virtual {v9, v0, v7, v8}, Ljavax/crypto/Cipher;->init(ILjava/security/Key;Ljava/security/spec/AlgorithmParameterSpec;)V
    invoke-virtual {v9, v5}, Ljavax/crypto/Cipher;->doFinal([B)[B
    move-result-object v5
    new-instance v0, Ljava/lang/String;
    const-string v6, "UTF-8"
    invoke-direct {v0, v5, v6}, Ljava/lang/String;-><init>([BLjava/lang/String;)V
    return-object v0

    :km
    .array-data 1
` + strings.TrimRight(arr.String(), "\n") + `
    .end array-data
.end method
`
}

// unescapeSmali decodes a smali string literal body (without the surrounding
// quotes) into its runtime value. Returns false for constructs we don't handle,
// so the caller can safely skip encrypting them.
func unescapeSmali(lit string) (string, bool) {
	var b strings.Builder
	rs := []rune(lit)
	for i := 0; i < len(rs); i++ {
		ch := rs[i]
		if ch != '\\' {
			b.WriteRune(ch)
			continue
		}
		i++
		if i >= len(rs) {
			return "", false
		}
		switch rs[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case '"':
			b.WriteByte('"')
		case '\'':
			b.WriteByte('\'')
		case '\\':
			b.WriteByte('\\')
		case 'u':
			if i+4 >= len(rs) {
				return "", false
			}
			cp, err := strconv.ParseInt(string(rs[i+1:i+5]), 16, 32)
			if err != nil {
				return "", false
			}
			b.WriteRune(rune(cp))
			i += 4
		default:
			return "", false
		}
	}
	return b.String(), true
}
