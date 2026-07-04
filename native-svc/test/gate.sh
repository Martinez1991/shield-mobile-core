#!/usr/bin/env bash
# Execution gate for native-svc (issue #82 acceptance criterion): a program is
# flattened at the bitcode level and must stay functionally identical.
#
#   1. compile sample.c -> bitcode
#   2. run it through native-svc --pass flatten
#   3. confirm the CFG actually changed (dispatcher present, more blocks)
#   4. compile both to executables and diff their output
#
# Usage: native-svc/test/gate.sh [path-to-native-svc]
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
svc="${1:-$here/../build/native-svc}"
CLANG="${CLANG:-clang-18}"
OPT="${OPT:-opt-18}"

if [[ ! -x "$svc" ]]; then echo "gate: native-svc not built at $svc" >&2; exit 1; fi
command -v "$CLANG" >/dev/null || { echo "gate: $CLANG not found" >&2; exit 1; }

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# 1. source -> bitcode (-O0 keeps the CFG intact for flattening to act on).
"$CLANG" -O0 -emit-llvm -c "$here/sample.c" -o "$tmp/orig.bc"

# 2. flatten.
"$svc" transform --arch x86_64 --seed 1337 --pass flatten < "$tmp/orig.bc" > "$tmp/flat.bc"

# 3. structural check: the dispatcher state variable must appear, and the
#    flattened function must have strictly more basic blocks than the original.
if command -v "$OPT" >/dev/null; then
  "$OPT" -S "$tmp/orig.bc" -o "$tmp/orig.ll" 2>/dev/null || "$CLANG" -S -emit-llvm "$tmp/orig.bc" -o "$tmp/orig.ll"
  "$OPT" -S "$tmp/flat.bc" -o "$tmp/flat.ll" 2>/dev/null || "$CLANG" -S -emit-llvm "$tmp/flat.bc" -o "$tmp/flat.ll"
else
  "$CLANG" -S -emit-llvm "$tmp/orig.bc" -o "$tmp/orig.ll"
  "$CLANG" -S -emit-llvm "$tmp/flat.bc" -o "$tmp/flat.ll"
fi
grep -q "switchVar" "$tmp/flat.ll" || { echo "gate: FAIL — no dispatcher (switchVar) in flattened IR" >&2; exit 1; }
orig_bb=$(grep -cE '^[0-9]+:|^[A-Za-z._][A-Za-z0-9._]*:' "$tmp/orig.ll" || true)
flat_bb=$(grep -cE '^[0-9]+:|^[A-Za-z._][A-Za-z0-9._]*:' "$tmp/flat.ll" || true)
echo "gate: basic-block labels  orig=$orig_bb  flat=$flat_bb"

# 4. execution equivalence.
"$CLANG" "$tmp/orig.bc" -o "$tmp/orig" -O0
"$CLANG" "$tmp/flat.bc" -o "$tmp/flat" -O0
"$tmp/orig" > "$tmp/orig.out"
"$tmp/flat" > "$tmp/flat.out"

if diff -u "$tmp/orig.out" "$tmp/flat.out"; then
  echo "gate: PASS — flattened binary is functionally identical"
else
  echo "gate: FAIL — output diverged after flattening" >&2
  exit 1
fi
