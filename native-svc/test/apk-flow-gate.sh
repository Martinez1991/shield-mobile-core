#!/usr/bin/env bash
# End-to-end APK native-protection gate (issue #82/#64): drive the Go worker
# (cmd/shield-nativeapk) over a recompilable APK and prove the round-trip.
#
#   build a "recompilable APK" = zip carrying lib/x86_64/libsample.so.bc (bitcode)
#   -> shield-nativeapk: native-svc transform -> link -> tamper-patch -> repackage
#   -> the protected .so runs identically (dlopen) AND detects tampering.
#
# Uses the host x86_64 toolchain so it runs without qemu. Needs go, clang, python3.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$here/.."
repo="$root/.."
svc="${SVC:-$root/build/native-svc}"
CLANG="${CLANG:-clang-18}"
GO="${GO:-go}"

[[ -x "$svc" ]] || { echo "apk-flow-gate: native-svc not built at $svc" >&2; exit 1; }
for t in "$CLANG" python3 "$GO"; do command -v "$t" >/dev/null || { echo "apk-flow-gate: $t not found" >&2; exit 1; }; done

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# 1. recompilable native module: bitcode sidecar for x86_64.
"$CLANG" --target=x86_64-linux-gnu -O0 -fPIC -emit-llvm -c "$here/libsample.c" -o "$tmp/libsample.so.bc"
# reference unprotected .so + a driver.
"$CLANG" -O0 -fPIC -shared "$here/libsample.c" -o "$tmp/plain.so"
"$CLANG" -O0 "$here/lib-driver.c" -ldl -o "$tmp/driver"
"$tmp/driver" "$tmp/plain.so" > "$tmp/ref.out"

# 2. assemble the recompilable APK (zip) with python.
python3 - "$tmp/in.apk" "$tmp/libsample.so.bc" <<'PY'
import sys, zipfile
apk, bc = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(apk, "w", zipfile.ZIP_DEFLATED) as z:
    z.write(bc, "lib/x86_64/libsample.so.bc")
    z.writestr("classes.dex", b"DEXPLACEHOLDER")
    z.writestr("AndroidManifest.xml", b"<manifest/>")
PY

# 3. run the Go worker.
( cd "$repo" && "$GO" run ./cmd/shield-nativeapk \
    --in "$tmp/in.apk" --out "$tmp/out.apk" \
    --passes "flatten mba opaque strings tamper" \
    --native-svc "$svc" --cc "$CLANG" --python python3 \
    --patch "$root/tools/tamper-patch.py" )

# 4. extract the protected .so from the output APK.
python3 - "$tmp/out.apk" "$tmp/prot.so" <<'PY'
import sys, zipfile
out, dst = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out) as z:
    names = z.namelist()
    assert "lib/x86_64/libsample.so" in names, names
    assert "lib/x86_64/libsample.so.bc" not in names, "sidecar not dropped"
    assert "classes.dex" in names, "copy-through lost"
    open(dst, "wb").write(z.read("lib/x86_64/libsample.so"))
PY

fail=0

# 5. protected .so runs identically (patched -> anti-tamper passes at dlopen).
"$tmp/driver" "$tmp/prot.so" > "$tmp/prot.out" && ec=0 || ec=$?
if [[ $ec -eq 0 ]] && diff -q "$tmp/ref.out" "$tmp/prot.out" >/dev/null; then
  echo "apk-flow-gate: PASS — protected .so runs identically after round-trip"
else
  echo "apk-flow-gate: FAIL — protected .so diverged (exit $ec)"; diff -u "$tmp/ref.out" "$tmp/prot.out" || true; fail=1
fi

# 6. tamper the protected .so -> anti-tamper must fire on dlopen (driver dies 67).
python3 "$root/tools/tamper-patch.py" flip "$tmp/prot.so" >/dev/null
"$tmp/driver" "$tmp/prot.so" >/dev/null 2>&1 && tec=0 || tec=$?
if [[ $tec -eq 67 ]]; then
  echo "apk-flow-gate: PASS — tampering the shipped .so is detected (exit 67)"
else
  echo "apk-flow-gate: FAIL — tampered .so expected exit 67, got $tec"; fail=1
fi

[[ $fail -eq 0 ]] && echo "apk-flow-gate: ALL PASS" || { echo "apk-flow-gate: FAILURES above" >&2; exit 1; }
