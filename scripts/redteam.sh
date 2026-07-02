#!/usr/bin/env bash
# Automated red-team KPI (shield-platform.md section 20, issue #9).
#
# Decompiles a baseline vs a protected DEX with a real decompiler (jadx) and
# measures reversibility: does a known secret leak, and how much noise/failure
# the protection injects. The no-leak check is a hard GATE (regression of known
# bypass): if a secret survives into the protected decompilation, exit 1.
#
# Requires: Java, the smali assembler jar, and jadx.
#   SMALI_JAR=~/tools/smali-2.5.2.jar JADX_CMD=~/tools/jadx/bin/jadx ./scripts/redteam.sh
# (JADX_CMD defaults to `jadx` on PATH.)
# Note: no `pipefail` — a no-match grep (exit 1) in the metric pipelines is the
# SUCCESS case (no secret) and must not abort the script.
set -eu
cd "$(dirname "$0")/.."

SMALI_JAR="${SMALI_JAR:-smali.jar}"
JADX_CMD="${JADX_CMD:-jadx}"
SRC="${1:-examples/smali}"
POLICY="${2:-examples/policy-prod-high.json}"

# Known secrets that must never survive into the protected output.
SECRET_RE='sk_live_[0-9A-Za-z]+|AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----|eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.'

if ! command -v java >/dev/null; then echo "java required" >&2; exit 2; fi
if ! command -v "$JADX_CMD" >/dev/null && [ ! -x "$JADX_CMD" ]; then
  echo "jadx not found (set JADX_CMD). Get it from https://github.com/skylot/jadx" >&2; exit 2
fi

WORK="$(mktemp -d)"; trap 'rm -rf "$WORK"' EXIT

echo "==> building shield + assembling baseline/protected DEX"
go build -o "$WORK/shield" ./cmd/shield
java -jar "$SMALI_JAR" a "$SRC" -o "$WORK/orig.dex" >/dev/null
"$WORK/shield" obfuscate "$SRC" --out "$WORK/out" --policy "$POLICY" >/dev/null
java -jar "$SMALI_JAR" a "$WORK/out" -o "$WORK/prot.dex" >/dev/null

echo "==> decompiling with jadx"
"$JADX_CMD" "$WORK/orig.dex" -d "$WORK/j_orig" >/dev/null 2>&1 || true
"$JADX_CMD" "$WORK/prot.dex" -d "$WORK/j_prot" >/dev/null 2>&1 || true

loc()   { find "$1" -name '*.java' -exec cat {} + 2>/dev/null | wc -l | tr -d ' '; }
count() { find "$1" -name '*.java' -exec cat {} + 2>/dev/null | grep -Eo "$2" | wc -l | tr -d ' '; }
leaks() { grep -rlE "$SECRET_RE" "$1" 2>/dev/null | wc -l | tr -d ' '; }

orig_secret=$(leaks "$WORK/j_orig")
prot_secret=$(leaks "$WORK/j_prot")
orig_loc=$(loc "$WORK/j_orig"); prot_loc=$(loc "$WORK/j_prot")
dec_calls=$(count "$WORK/j_prot" 'SH\.d\(')
vm_calls=$(count "$WORK/j_prot" '\.run\(')
jadx_warn=$(count "$WORK/j_prot" 'JADX (WARN|INFO: Failed|ERROR)')

echo ""
echo "===================== RED-TEAM KPI ====================="
printf "%-34s %10s %10s\n" "metric" "baseline" "protected"
printf "%-34s %10s %10s\n" "files leaking a known secret" "$orig_secret" "$prot_secret"
printf "%-34s %10s %10s\n" "decompiled Java LOC" "$orig_loc" "$prot_loc"
printf "%-34s %10s %10s\n" "string-decryptor calls (SH.d)" "-" "$dec_calls"
printf "%-34s %10s %10s\n" "vm dispatch calls (VM.run)" "-" "$vm_calls"
printf "%-34s %10s %10s\n" "jadx warnings/failures" "-" "$jadx_warn"
echo "========================================================"

if [ "$prot_secret" -ne 0 ]; then
  echo "GATE FAILED: a known secret survived into the protected decompilation." >&2
  exit 1
fi
echo "GATE OK: no known secret leaked into the protected output."
