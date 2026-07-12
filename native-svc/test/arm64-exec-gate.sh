#!/usr/bin/env bash
# arm64 EXECUTION gate (issue #82 / #64): the arm64 machine code produced from
# native-svc-transformed bitcode is run for real under qemu-user (aarch64 ISA)
# and must produce the same output as the unprotected build. This lifts the arm64
# artifact from "structurally valid" (ndk-gate) to "executes identically" — the
# native counterpart of the golden/ART gate, ISA-level and KVM-free (fast in CI).
#
# It targets aarch64-linux-gnu (glibc) rather than Android/Bionic on purpose:
# statically-linked Bionic binaries abort under qemu-user (TLS must be 64-aligned),
# and the passes are OS/ABI-independent IR->IR work, so glibc arm64 is a faithful
# proof of the arm64 codegen. The Android build itself is covered by ndk-gate.sh.
#
# Deps: clang (emit + arm64 object), gcc-aarch64-linux-gnu (static link), qemu-user.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$here/.."
svc="${SVC:-$root/build/native-svc}"
src="$here/sample.c"
PASSES="${PASSES:-flatten mba opaque strings}"
CLANG="${CLANG:-clang-18}"
XGCC="${XGCC:-aarch64-linux-gnu-gcc}"
QEMU="${QEMU:-$(command -v qemu-aarch64-static qemu-aarch64 2>/dev/null | head -1 || true)}"

[[ -x "$svc" ]] || { echo "arm64-gate: native-svc not built at $svc" >&2; exit 1; }
command -v "$CLANG" >/dev/null || { echo "arm64-gate: $CLANG not found" >&2; exit 1; }
command -v "$XGCC"  >/dev/null || { echo "arm64-gate: $XGCC not found (install gcc-aarch64-linux-gnu)" >&2; exit 1; }
[[ -n "$QEMU" ]] || { echo "arm64-gate: qemu-aarch64 not found (install qemu-user-static)" >&2; exit 1; }

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
passflags=(); for p in $PASSES; do passflags+=(--pass "$p"); done

# Untransformed vs transformed bitcode (targeting arm64).
"$CLANG" --target=aarch64-linux-gnu -O0 -emit-llvm -c "$src" -o "$tmp/pre.bc"
"$svc" transform --arch arm64-v8a --seed 4242 "${passflags[@]}" < "$tmp/pre.bc" > "$tmp/post.bc"

# Bitcode -> arm64 objects -> statically-linked arm64 executables.
"$CLANG" --target=aarch64-linux-gnu -O0 -c "$tmp/pre.bc"  -o "$tmp/pre.o"
"$CLANG" --target=aarch64-linux-gnu -O0 -c "$tmp/post.bc" -o "$tmp/post.o"
"$XGCC" -static "$tmp/pre.o"  -o "$tmp/plain"
"$XGCC" -static "$tmp/post.o" -o "$tmp/prot"

echo "arm64-gate: running under $(basename "$QEMU") (passes: $PASSES)"
"$QEMU" "$tmp/plain" > "$tmp/plain.out"
"$QEMU" "$tmp/prot"  > "$tmp/prot.out"

fail=0
if diff -q "$tmp/plain.out" "$tmp/prot.out" >/dev/null; then
  echo "arm64-gate: PASS — protected arm64 binary executes identically"
else
  echo "arm64-gate: FAIL — arm64 output diverged"; diff -u "$tmp/plain.out" "$tmp/prot.out" || true; fail=1
fi

# strings pass must have removed the plaintext from the protected arm64 binary.
if echo "$PASSES" | grep -qw strings; then
  if grep -aq 'classify(' "$tmp/prot"; then
    echo "arm64-gate: FAIL — plaintext 'classify(' still in protected arm64 binary"; fail=1
  else
    echo "arm64-gate: plaintext absent from protected arm64 binary"
  fi
fi

[[ $fail -eq 0 ]] && echo "arm64-gate: ALL PASS" || { echo "arm64-gate: FAILURES above" >&2; exit 1; }
