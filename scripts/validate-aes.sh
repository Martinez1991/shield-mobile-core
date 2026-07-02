#!/usr/bin/env bash
# Validates the AES-256-GCM string decryptor (shield-platform.md §3.3, issue #5)
# end-to-end on the JVM: Go emits the masked key material + keystream params +
# ciphertext + expected plaintext; a Java port of the injected smali decryptor
# (scripts/jvm/AesDec.java) unmasks, derives the key and decrypts, and must
# reproduce the plaintext. Confirms the on-device decryptor path is correct.
#
# Requires Java (javac + java). No jars needed.
set -euo pipefail
cd "$(dirname "$0")/.."

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "==> emitting AES vector from Go"
SHIELD_AES_VEC="$WORK/vec.txt" go test ./internal/engine -run TestAESDumpVector >/dev/null

echo "==> compiling JVM harness"
javac -d "$WORK" scripts/jvm/AesDec.java

echo "==> decrypting Go ciphertext with the mirrored decryptor"
java -cp "$WORK" AesDec "$WORK/vec.txt"
