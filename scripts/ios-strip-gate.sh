#!/usr/bin/env bash
# iOS Mach-O strip gate (issue #76) — runs on a macOS runner. Drives the Go worker
# (cmd/shield-iosstrip) over an IPA carrying a Mach-O, then proves the round-trip:
# the stripped binary still runs identically, and a local symbol is gone. Stripping
# breaks the code signature, so the worker ad-hoc re-signs (codesign -f -s -) — no
# Apple certificate needed — which is what lets the binary run on Apple Silicon.
# (Real distribution re-signing is #77; the iOS Simulator differential is #78.)
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/.." && pwd)"
CLANG="${CLANG:-clang}"
GO="${GO:-go}"

tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT

# A small Mach-O with a local (static) symbol we can watch disappear.
cat > "$tmp/sample.c" <<'EOF'
#include <stdio.h>
__attribute__((noinline)) static int classify(int n) {
  int a = 0;
  for (int i = 1; i <= n; i++) a += (i % 15 == 0) ? 100 : (i % 3 == 0) ? 3 : (i % 5 == 0) ? 5 : 1;
  return a;
}
int main(void) {
  for (int n = 0; n <= 30; n += 6) printf("classify(%d)=%d\n", n, classify(n));
  return 0;
}
EOF
"$CLANG" -O0 "$tmp/sample.c" -o "$tmp/Sample"
"$tmp/Sample" > "$tmp/plain.out"
nm "$tmp/Sample" | grep -q _classify || { echo "ios-strip-gate: setup — local symbol _classify missing before strip" >&2; exit 1; }
echo "ios-strip-gate: before strip — _classify present"

# Package it as an IPA (Payload/Sample.app/Sample).
python3 - "$tmp" <<'PY'
import sys, zipfile
d = sys.argv[1]
with zipfile.ZipFile(d + "/in.ipa", "w", zipfile.ZIP_DEFLATED) as z:
    z.write(d + "/Sample", "Payload/Sample.app/Sample")
    z.writestr("Payload/Sample.app/Info.plist", b"<plist/>")
PY

# Strip through the Go worker (strip + ad-hoc re-sign).
( cd "$repo" && "$GO" run ./cmd/shield-iosstrip \
    --in "$tmp/in.ipa" --out "$tmp/out.ipa" \
    --strip "xcrun strip -x" --sign "codesign -f -s -" )

# Extract the stripped binary.
python3 - "$tmp" <<'PY'
import sys, zipfile
d = sys.argv[1]
with zipfile.ZipFile(d + "/out.ipa") as z:
    assert "Payload/Sample.app/Sample" in z.namelist(), z.namelist()
    assert "Payload/Sample.app/Info.plist" in z.namelist(), "copy-through lost"
    open(d + "/stripped", "wb").write(z.read("Payload/Sample.app/Sample"))
PY
chmod +x "$tmp/stripped"

fail=0
# 1. still runs identically.
if "$tmp/stripped" > "$tmp/stripped.out" 2>/dev/null && diff -q "$tmp/plain.out" "$tmp/stripped.out" >/dev/null; then
  echo "ios-strip-gate: PASS — stripped (ad-hoc signed) binary runs identically"
else
  echo "ios-strip-gate: FAIL — stripped binary diverged/failed" >&2; diff -u "$tmp/plain.out" "$tmp/stripped.out" || true; fail=1
fi
# 2. the local symbol is gone.
if nm "$tmp/stripped" 2>/dev/null | grep -q _classify; then
  echo "ios-strip-gate: FAIL — local symbol _classify still present after strip" >&2; fail=1
else
  echo "ios-strip-gate: PASS — local symbol _classify removed"
fi

[[ $fail -eq 0 ]] && echo "ios-strip-gate: ALL PASS" || { echo "ios-strip-gate: FAILURES above" >&2; exit 1; }
