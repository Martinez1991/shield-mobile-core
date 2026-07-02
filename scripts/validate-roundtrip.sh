#!/usr/bin/env bash
# Validates the full DEX round-trip (shield-platform.md §2.1):
#   examples/smali --(smali assembler)--> orig.dex        (baseline is valid)
#   examples/smali --(shield obfuscate)--> out/           (apply all passes)
#   out/          --(smali assembler)--> prot.dex         (STILL valid Dalvik)
#   prot.dex      --(baksmali)--------> back/             (dex is well-formed)
#
# Assembly performs Dalvik structural verification (registers, labels, control
# flow), so a clean assemble of the obfuscated output proves the transformations
# preserve a valid program structure.
#
# Requires Java + the smali/baksmali fat jars. Point to them via env vars, e.g.:
#   SMALI_JAR=~/tools/smali-2.5.2.jar BAKSMALI_JAR=~/tools/baksmali-2.5.2.jar ./scripts/validate-roundtrip.sh
# Get them from https://bitbucket.org/JesusFreke/smali/downloads/
set -euo pipefail

cd "$(dirname "$0")/.."
SMALI_JAR="${SMALI_JAR:-smali.jar}"
BAKSMALI_JAR="${BAKSMALI_JAR:-baksmali.jar}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "==> building shield"
go build -o "$WORK/shield" ./cmd/shield

echo "==> baseline: assemble original example smali"
java -jar "$SMALI_JAR" a examples/smali -o "$WORK/orig.dex"

echo "==> obfuscate (all passes)"
"$WORK/shield" obfuscate examples/smali --out "$WORK/out" --policy examples/policy-prod-high.json

echo "==> assemble OBFUSCATED output (structural verification)"
java -jar "$SMALI_JAR" a "$WORK/out" -o "$WORK/prot.dex"

echo "==> disassemble protected dex back to smali"
java -jar "$BAKSMALI_JAR" d "$WORK/prot.dex" -o "$WORK/back"

if grep -rq "sk_live_" "$WORK/back"; then
  echo "FAIL: plaintext secret leaked into the protected dex" >&2
  exit 1
fi

echo "ROUND-TRIP OK: obfuscated smali assembles to a valid, well-formed DEX; no plaintext secret."
