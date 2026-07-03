#!/usr/bin/env bash
# Runtime correctness gate (shield-platform.md section 20, issue #3).
#
# Differential testing of golden apps on REAL ART: assemble the original and the
# protected golden dex, run each on an Android device/emulator via app_process,
# and require byte-identical output. A divergence means the obfuscation broke
# semantics -> exit 1 (this is the release gate).
#
# Requires: Java + the smali assembler jar. To run the ART diff, an emulator/
# device must be connected (adb). Without one, only the structural assembly is
# checked (both dex must assemble) and the ART diff is skipped.
#
#   SMALI_JAR=~/tools/smali-2.5.2.jar ./scripts/golden-diff.sh
set -eu
cd "$(dirname "$0")/.."

SMALI_JAR="${SMALI_JAR:-smali.jar}"
GOLDEN="testdata/golden/smali"
POLICY="testdata/golden/policy.json"
MAIN="golden.Main"

WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT

echo "==> building shield"
go build -o "$WORK/shield" ./cmd/shield

echo "==> assembling ORIGINAL golden dex"
java -jar "$SMALI_JAR" a "$GOLDEN" -o "$WORK/orig.dex" >/dev/null

echo "==> obfuscating + assembling PROTECTED golden dex"
"$WORK/shield" obfuscate "$GOLDEN" --out "$WORK/prot" --policy "$POLICY" >/dev/null
java -jar "$SMALI_JAR" a "$WORK/prot" -o "$WORK/prot.dex" >/dev/null
echo "   both assembled OK (structural check passed)"

# Need an adb device/emulator for the semantic differential run.
DEVICE=""
if command -v adb >/dev/null 2>&1; then
  DEVICE="$(adb devices 2>/dev/null | grep -vi 'list of devices' | grep -w 'device' | head -1 | awk '{print $1}' || true)"
fi
if [ -z "$DEVICE" ]; then
  echo "==> no adb device: skipping the ART differential run (structural check only)."
  echo "    Run under CI (emulator) or with a connected device for the full gate."
  exit 0
fi
echo "==> using device: $DEVICE"

run() { # $1 = dex path
  adb push "$1" /data/local/tmp/golden.dex >/dev/null
  adb shell "CLASSPATH=/data/local/tmp/golden.dex app_process /system/bin $MAIN" | tr -d '\r'
}

echo "==> running ORIGINAL on ART"
ORIG="$(run "$WORK/orig.dex")"
echo "$ORIG" | sed 's/^/   orig: /'

echo "==> running PROTECTED on ART"
PROT="$(run "$WORK/prot.dex")"
echo "$PROT" | sed 's/^/   prot: /'

# Sanity: the original must have produced the expected golden output.
EXPECT=$'20\n10\ngolden-secret-42\n1\n100\n7\n22375\n20000199998\n736\n10000000010\nAA\nBB\npos\nneg\n5\n0\n42\nnonzero\nzero\n5000000000\n42\n123'
if [ "$ORIG" != "$EXPECT" ]; then
  echo "GATE ERROR: original output unexpected (harness/env problem):" >&2
  echo "$ORIG" >&2
  exit 2
fi

if [ "$ORIG" != "$PROT" ]; then
  echo "GATE FAILED: protected output differs from original (obfuscation broke semantics)." >&2
  diff <(echo "$ORIG") <(echo "$PROT") >&2 || true
  exit 1
fi

echo "GATE OK: protected golden app behaves identically to the original on ART."
