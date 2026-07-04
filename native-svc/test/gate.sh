#!/usr/bin/env bash
# Execution gate for native-svc (issue #82 acceptance criterion): each pass (and
# their composition) must transform the bitcode yet keep the program functionally
# identical.
#
#   for each pass-set: compile sample.c -> bitcode -> native-svc -> compile+run,
#   assert the IR actually changed AND the output is byte-identical to the original.
#
# Usage: native-svc/test/gate.sh [path-to-native-svc]
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
svc="${1:-$here/../build/native-svc}"
CLANG="${CLANG:-clang-18}"

if [[ ! -x "$svc" ]]; then echo "gate: native-svc not built at $svc" >&2; exit 1; fi
command -v "$CLANG" >/dev/null || { echo "gate: $CLANG not found" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Reference: original bitcode, compiled and run (-O0 keeps the CFG/ops intact).
"$CLANG" -O0 -emit-llvm -c "$here/sample.c" -o "$tmp/orig.bc"
"$CLANG" -S -emit-llvm "$tmp/orig.bc" -o "$tmp/orig.ll"
"$CLANG" "$tmp/orig.bc" -o "$tmp/orig" -O0
"$tmp/orig" > "$tmp/orig.out"

fail=0
run_case() {
  local label="$1"; shift
  local passflags=()
  for p in "$@"; do passflags+=(--pass "$p"); done

  "$svc" transform --arch x86_64 --seed 1337 "${passflags[@]}" < "$tmp/orig.bc" > "$tmp/t.bc"
  "$CLANG" -S -emit-llvm "$tmp/t.bc" -o "$tmp/t.ll"

  # structural: the IR must have actually changed.
  if cmp -s "$tmp/orig.ll" "$tmp/t.ll"; then
    echo "gate[$label]: FAIL — IR unchanged"; fail=1; return
  fi

  # execution: output must be identical to the original.
  "$CLANG" "$tmp/t.bc" -o "$tmp/t" -O0
  "$tmp/t" > "$tmp/t.out"
  if diff -q "$tmp/orig.out" "$tmp/t.out" >/dev/null; then
    echo "gate[$label]: PASS — transformed, functionally identical"
  else
    echo "gate[$label]: FAIL — output diverged"; diff -u "$tmp/orig.out" "$tmp/t.out" || true; fail=1
  fi
}

run_case flatten         flatten
run_case mba             mba
run_case opaque          opaque
run_case flatten+mba+opq flatten mba opaque

# flatten must introduce the dispatcher.
"$svc" transform --pass flatten < "$tmp/orig.bc" | "$CLANG" -S -emit-llvm -x ir - -o "$tmp/f.ll" 2>/dev/null || \
  { "$svc" transform --pass flatten < "$tmp/orig.bc" > "$tmp/f.bc"; "$CLANG" -S -emit-llvm "$tmp/f.bc" -o "$tmp/f.ll"; }
grep -q switchVar "$tmp/f.ll" && echo "gate[flatten]: dispatcher (switchVar) present" || { echo "gate[flatten]: FAIL — no dispatcher"; fail=1; }

[[ $fail -eq 0 ]] && echo "gate: ALL PASS" || { echo "gate: FAILURES above" >&2; exit 1; }
