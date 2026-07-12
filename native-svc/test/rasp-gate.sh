#!/usr/bin/env bash
# Native RASP (anti-debug) gate (issue #84): the injected check must be silent
# under a normal run (functionally identical) and fire under a debugger.
#
#   normal run              -> same output as unprotected, exit 0
#   under a ptrace tracer   -> TracerPid != 0, so RASP exits 66 with no output
#
# The tracer is a self-contained ptrace harness (test/tracer.c), so the gate
# needs no strace/gdb. Also runs the full obfuscation+rasp composition, proving
# the RASP survives (and still fires) after flatten/mba/opaque/strings.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$here/.."
svc="${SVC:-$root/build/native-svc}"
src="$here/sample.c"
CLANG="${CLANG:-clang-18}"

[[ -x "$svc" ]] || { echo "rasp-gate: native-svc not built at $svc" >&2; exit 1; }
command -v "$CLANG" >/dev/null || { echo "rasp-gate: $CLANG not found" >&2; exit 1; }

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
fail=0

"$CLANG" -O0 "$src" -o "$tmp/plain"
"$tmp/plain" > "$tmp/plain.out"
"$CLANG" -O0 "$here/tracer.c" -o "$tmp/tracer"

run_case() {
  local label="$1"; shift
  local passflags=(); for p in "$@"; do passflags+=(--pass "$p"); done
  "$CLANG" -O0 -emit-llvm -c "$src" -o "$tmp/pre.bc"
  "$svc" transform --arch host --seed 5 "${passflags[@]}" < "$tmp/pre.bc" > "$tmp/post.bc"
  "$CLANG" -O0 "$tmp/post.bc" -o "$tmp/prot"

  # normal run: identical output, exit 0.
  "$tmp/prot" > "$tmp/prot.out" && ec=0 || ec=$?
  if [[ $ec -ne 0 ]] || ! diff -q "$tmp/plain.out" "$tmp/prot.out" >/dev/null; then
    echo "rasp-gate[$label]: FAIL — undebugged run not identical (exit $ec)"; fail=1; return
  fi
  echo "rasp-gate[$label]: normal run identical (exit 0)"

  # traced run: RASP must fire (exit 66, no normal output).
  "$tmp/tracer" "$tmp/prot" > "$tmp/traced.out" 2>/dev/null && tec=0 || tec=$?
  if [[ $tec -eq 66 && ! -s "$tmp/traced.out" ]]; then
    echo "rasp-gate[$label]: PASS — debugger detected (exit 66, no output)"
  else
    echo "rasp-gate[$label]: FAIL — under tracer expected exit 66/no output, got exit $tec"; fail=1
  fi
}

run_case rasp                    rasp
run_case rasp+flatten+mba+opq+str rasp flatten mba opaque strings

[[ $fail -eq 0 ]] && echo "rasp-gate: ALL PASS" || { echo "rasp-gate: FAILURES above" >&2; exit 1; }
