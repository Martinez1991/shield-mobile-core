#!/usr/bin/env bash
# protect-so.sh — reference compile -> transform -> link for a native shared
# library (issue #82 / #64, ADR 0004). This is the flow the sandboxed worker
# drives: LLVM passes act on bitcode, so we compile source to bitcode, run it
# through native-svc, and link the protected bitcode to a .so.
#
# Usage:
#   protect-so.sh --cc <clang> [--passes "flatten mba"] [--seed N] \
#                 [--arch <abi>] [--svc <native-svc>] --out <lib.so> <src.c>
set -euo pipefail

CC=""; PASSES="flatten mba"; SEED=1; ARCH="host"; OUT=""; SRC=""
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SVC="$here/../build/native-svc"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cc) CC="$2"; shift 2;;
    --passes) PASSES="$2"; shift 2;;
    --seed) SEED="$2"; shift 2;;
    --arch) ARCH="$2"; shift 2;;
    --svc) SVC="$2"; shift 2;;
    --out) OUT="$2"; shift 2;;
    -*) echo "protect-so: unknown flag $1" >&2; exit 2;;
    *) SRC="$1"; shift;;
  esac
done

[[ -n "$CC"  ]] || { echo "protect-so: --cc required" >&2; exit 2; }
[[ -n "$OUT" ]] || { echo "protect-so: --out required" >&2; exit 2; }
[[ -n "$SRC" ]] || { echo "protect-so: source file required" >&2; exit 2; }
[[ -x "$SVC" ]] || { echo "protect-so: native-svc not found at $SVC" >&2; exit 1; }

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

passflags=()
for p in $PASSES; do passflags+=(--pass "$p"); done

# 1. source -> bitcode (position-independent; -O0 keeps structure for the passes)
"$CC" -O0 -fPIC -emit-llvm -c "$SRC" -o "$tmp/pre.bc"
# 2. obfuscate
"$SVC" transform --arch "$ARCH" --seed "$SEED" "${passflags[@]}" < "$tmp/pre.bc" > "$tmp/post.bc"
# 3. link the protected bitcode into a shared object
"$CC" -shared -fPIC "$tmp/post.bc" -o "$OUT"

echo "protect-so: wrote $OUT (passes: $PASSES, arch: $ARCH)"
