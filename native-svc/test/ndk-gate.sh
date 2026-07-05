#!/usr/bin/env bash
# End-to-end native-library gate (issue #82 / #64): protect a shared library
# through the full compile -> transform -> link flow (native-svc/tools/protect-so.sh)
# and prove it.
#
#   HOST  (runnable): build unprotected vs protected .so, dlopen+call both, diff
#                     the output — functional identity on a target we can execute.
#   ANDROID (if NDK): build a protected arm64 Android .so and assert it is a valid
#                     AArch64 shared object (can't be executed on an x86 host
#                     without an emulator, so this is a structural gate).
#
# Env: CLANG (host clang, default clang-18), ANDROID_NDK_HOME / ANDROID_NDK_LATEST_HOME
# Usage: native-svc/test/ndk-gate.sh
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$here/.."
CLANG="${CLANG:-clang-18}"
READELF="${READELF:-llvm-readelf-18}"
protect="$root/tools/protect-so.sh"
lib="$here/libsample.c"

command -v "$CLANG" >/dev/null || { echo "ndk-gate: $CLANG not found" >&2; exit 1; }
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
fail=0

echo "== HOST (execution gate) =="
"$CLANG" -O0 -fPIC -shared "$lib" -o "$tmp/plain.so"
bash "$protect" --cc "$CLANG" --passes "flatten mba opaque strings" --seed 7 --arch host --out "$tmp/prot.so" "$lib"
"$CLANG" "$here/lib-driver.c" -ldl -o "$tmp/driver"
"$tmp/driver" "$tmp/plain.so" > "$tmp/plain.out"
"$tmp/driver" "$tmp/prot.so"  > "$tmp/prot.out"
if diff -q "$tmp/plain.out" "$tmp/prot.out" >/dev/null; then
  echo "ndk-gate[host]: PASS — protected .so is functionally identical"
else
  echo "ndk-gate[host]: FAIL — output diverged"; diff -u "$tmp/plain.out" "$tmp/prot.out" || true; fail=1
fi

echo "== ANDROID arm64 (structural gate) =="
NDK="${ANDROID_NDK_HOME:-${ANDROID_NDK_LATEST_HOME:-}}"
if [[ -z "$NDK" && -d "$HOME/android-ndk-r26d" ]]; then NDK="$HOME/android-ndk-r26d"; fi
# Pick the lowest-API aarch64 wrapper available, so this works across NDK versions.
ndkcc=""
if [[ -n "$NDK" ]]; then
  ndkcc=$(ls "$NDK"/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android*-clang 2>/dev/null | sort -V | head -1 || true)
fi
if [[ -n "$ndkcc" && -x "$ndkcc" ]]; then
  bash "$protect" --cc "$ndkcc" --passes "flatten mba opaque strings" --seed 7 --arch arm64-v8a --out "$tmp/libsample.so" "$lib"
  mach=$("$READELF" -h "$tmp/libsample.so" 2>/dev/null | awk -F: '/Machine/{gsub(/^ +/,"",$2);print $2}')
  typ=$("$READELF" -h "$tmp/libsample.so" 2>/dev/null | awk -F: '/Type/{gsub(/^ +/,"",$2);print $2}')
  echo "ndk-gate[android]: machine='$mach' type='$typ'"
  if echo "$mach" | grep -qi aarch64 && echo "$typ" | grep -qi -e dyn -e shared; then
    echo "ndk-gate[android]: PASS — valid AArch64 Android shared object produced"
  else
    echo "ndk-gate[android]: FAIL — not a valid arm64 shared object"; fail=1
  fi
else
  echo "ndk-gate[android]: SKIP — no NDK (set ANDROID_NDK_HOME); host gate already proved correctness"
fi

[[ $fail -eq 0 ]] && echo "ndk-gate: ALL PASS" || { echo "ndk-gate: FAILURES above" >&2; exit 1; }
