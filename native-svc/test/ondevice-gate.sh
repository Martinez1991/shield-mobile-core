#!/usr/bin/env bash
# On-device arm64 execution gate (issue #64): run the protected arm64 binary on a
# REAL Android device (via adb) and require identical output to the unprotected
# build. This is the strongest native gate — real Bionic + ART hardware — versus
# the qemu-user ISA gate. It is manual/local (a device isn't available in CI), so
# it is NOT wired into a workflow; run it with a phone connected over adb.
#
# Build runs in WSL (NDK); deploy/run uses the Windows adb.exe over interop, so
# binaries are staged on a C:-visible path. Env: ADB, NDK/ANDROID_NDK_HOME, PASSES.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$here/.."
svc="${SVC:-$root/build/native-svc}"
src="$here/sample.c"
PASSES="${PASSES:-flatten mba opaque strings}"
# Resolve adb: $ADB, then adb on PATH, then the Windows SDK default (WSL interop).
ADB="${ADB:-$(command -v adb adb.exe 2>/dev/null | head -1 || true)}"
[[ -n "$ADB" ]] || ADB=$(ls /mnt/c/Users/*/AppData/Local/Android/Sdk/platform-tools/adb.exe 2>/dev/null | head -1 || true)

NDK="${ANDROID_NDK_HOME:-$HOME/android-ndk-r26d}"
CC=$(ls "$NDK"/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android*-clang 2>/dev/null | sort -V | head -1 || true)

[[ -x "$svc" ]] || { echo "ondevice-gate: native-svc not built at $svc" >&2; exit 1; }
[[ -n "$CC" && -x "$CC" ]] || { echo "ondevice-gate: no NDK aarch64 clang" >&2; exit 1; }
[[ -x "$ADB" ]] || { echo "ondevice-gate: adb not found at $ADB" >&2; exit 1; }
"$ADB" get-state >/dev/null 2>&1 || { echo "ondevice-gate: no device (adb get-state)" >&2; exit 1; }

# Stage on a C:-visible dir so the Windows adb.exe can read the files. Uses the
# gitignored build/ dir (the repo is on C:); translate /mnt/<drive> -> <DRIVE>:.
stage="$root/build/ondevice"
winstage=$(echo "$stage" | sed -E 's|^/mnt/([a-z])|\U\1:|')
[[ "$winstage" == "$stage" ]] && { echo "ondevice-gate: stage $stage is not on a /mnt/<drive> path adb.exe can read" >&2; exit 1; }
rm -rf "$stage"; mkdir -p "$stage"
dev="/data/local/tmp/shield"
cleanup() { "$ADB" shell "rm -rf $dev" >/dev/null 2>&1 || true; }
trap cleanup EXIT

passflags=(); for p in $PASSES; do passflags+=(--pass "$p"); done

# Build unprotected + protected arm64 Android executables.
"$CC" -O0 "$src" -o "$stage/plain"
"$CC" -O0 -emit-llvm -c "$src" -o "$stage/pre.bc"
"$svc" transform --arch arm64-v8a --seed 4242 "${passflags[@]}" < "$stage/pre.bc" > "$stage/post.bc"
"$CC" -O0 "$stage/post.bc" -o "$stage/prot"

# Deploy and run on the real device.
"$ADB" shell "mkdir -p $dev" >/dev/null
"$ADB" push "$winstage/plain" "$dev/plain" >/dev/null
"$ADB" push "$winstage/prot" "$dev/prot" >/dev/null
"$ADB" shell "chmod 755 $dev/plain $dev/prot" >/dev/null

model=$("$ADB" shell getprop ro.product.model | tr -d '\r')
abi=$("$ADB" shell getprop ro.product.cpu.abi | tr -d '\r')
echo "ondevice-gate: running on $model ($abi), passes: $PASSES"

"$ADB" shell "$dev/plain" > "$stage/plain.out"
"$ADB" shell "$dev/prot" > "$stage/prot.out"

fail=0
if diff -q "$stage/plain.out" "$stage/prot.out" >/dev/null; then
  echo "ondevice-gate: PASS — protected arm64 binary runs identically on real hardware"
else
  echo "ondevice-gate: FAIL — output diverged on device" >&2
  diff -u "$stage/plain.out" "$stage/prot.out" || true; fail=1
fi

# Anti-tamper on real hardware: patched -> identical; a flipped code byte -> exit 67.
if command -v python3 >/dev/null; then
  "$svc" transform --arch arm64-v8a --seed 4242 --pass flatten --pass tamper < "$stage/pre.bc" > "$stage/tam.bc"
  "$CC" -O0 "$stage/tam.bc" -o "$stage/tam"
  python3 "$root/tools/tamper-patch.py" patch "$stage/tam" >/dev/null
  "$ADB" push "$winstage/tam" "$dev/tam" >/dev/null && "$ADB" shell "chmod 755 $dev/tam" >/dev/null
  "$ADB" shell "$dev/tam" > "$stage/tam.out"; tec=$("$ADB" shell "$dev/tam >/dev/null 2>&1; echo \$?" | tr -d '\r')
  if [[ "$tec" == "0" ]] && diff -q "$stage/plain.out" "$stage/tam.out" >/dev/null; then
    echo "ondevice-gate: PASS — tamper-patched binary runs identically on device"
  else
    echo "ondevice-gate: FAIL — patched tamper binary diverged (exit $tec)" >&2; fail=1
  fi
  python3 "$root/tools/tamper-patch.py" flip "$stage/tam" >/dev/null
  "$ADB" push "$winstage/tam" "$dev/tam" >/dev/null && "$ADB" shell "chmod 755 $dev/tam" >/dev/null
  fec=$("$ADB" shell "$dev/tam >/dev/null 2>&1; echo \$?" | tr -d '\r')
  if [[ "$fec" == "67" ]]; then
    echo "ondevice-gate: PASS — code tamper detected on real hardware (exit 67)"
  else
    echo "ondevice-gate: FAIL — tampered binary expected exit 67 on device, got $fec" >&2; fail=1
  fi
fi

[[ $fail -eq 0 ]] && echo "ondevice-gate: ALL PASS" || { echo "ondevice-gate: FAILURES above" >&2; exit 1; }
