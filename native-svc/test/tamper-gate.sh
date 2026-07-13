#!/usr/bin/env bash
# Native anti-tamper gate (issue #84): the injected self-checksum must detect an
# unpatched/tampered binary and stay silent once patched.
#
#   unpatched (sentinel)  -> checksum mismatch -> exit 67  (proves the check is live)
#   patched               -> matches -> runs identically, exit 0
#   patched then tampered -> mismatch -> exit 67           (detects code patching)
#
# Uses tools/tamper-patch.py (pure python) so it runs without Go/strace/qemu.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$here/.."
svc="${SVC:-$root/build/native-svc}"
src="$here/sample.c"
CLANG="${CLANG:-clang-18}"
PATCH="$root/tools/tamper-patch.py"

[[ -x "$svc" ]] || { echo "tamper-gate: native-svc not built at $svc" >&2; exit 1; }
command -v "$CLANG"  >/dev/null || { echo "tamper-gate: $CLANG not found" >&2; exit 1; }
command -v python3   >/dev/null || { echo "tamper-gate: python3 not found" >&2; exit 1; }

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
fail=0

"$CLANG" -O0 "$src" -o "$tmp/plain"
"$tmp/plain" > "$tmp/plain.out"

run_case() {
  local label="$1"; shift
  local passflags=(); for p in "$@"; do passflags+=(--pass "$p"); done
  "$CLANG" -O0 -emit-llvm -c "$src" -o "$tmp/pre.bc"
  "$svc" transform --arch host --seed 5 "${passflags[@]}" < "$tmp/pre.bc" > "$tmp/post.bc"
  "$CLANG" -O0 "$tmp/post.bc" -o "$tmp/prot"

  # 1. unpatched: sentinel != real sum -> must be detected.
  "$tmp/prot" >/dev/null 2>&1 && ec=0 || ec=$?
  if [[ $ec -ne 67 ]]; then echo "tamper-gate[$label]: FAIL — unpatched expected exit 67, got $ec"; fail=1; return; fi
  echo "tamper-gate[$label]: unpatched detected (exit 67)"

  # 2. patched: matches -> runs identically.
  python3 "$PATCH" patch "$tmp/prot" >/dev/null
  "$tmp/prot" > "$tmp/prot.out" && ec=0 || ec=$?
  if [[ $ec -ne 0 ]] || ! diff -q "$tmp/plain.out" "$tmp/prot.out" >/dev/null; then
    echo "tamper-gate[$label]: FAIL — patched run not identical (exit $ec)"; fail=1; return; fi
  echo "tamper-gate[$label]: patched run identical (exit 0)"

  # 3. tamper a code byte after patching -> must be detected.
  python3 "$PATCH" flip "$tmp/prot" >/dev/null
  "$tmp/prot" >/dev/null 2>&1 && ec=0 || ec=$?
  if [[ $ec -eq 67 ]]; then
    echo "tamper-gate[$label]: PASS — code tamper detected (exit 67)"
  else
    echo "tamper-gate[$label]: FAIL — tampered expected exit 67, got $ec"; fail=1
  fi
}

run_case tamper                 tamper
run_case tamper+flatten+mba+str tamper flatten mba strings

[[ $fail -eq 0 ]] && echo "tamper-gate: ALL PASS" || { echo "tamper-gate: FAILURES above" >&2; exit 1; }
