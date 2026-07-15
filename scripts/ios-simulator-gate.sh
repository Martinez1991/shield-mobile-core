#!/usr/bin/env bash
# iOS Simulator differential gate (issue #78) — runs on a macOS runner. Builds a
# Mach-O for the iOS Simulator, runs it in a booted simulator (real iOS dyld /
# libSystem) via `simctl spawn`, then protects it through the Go worker (strip +
# ad-hoc re-sign) and runs the protected binary the same way — requiring identical
# output. This lifts iOS verification from "runs on the macOS host" to "runs in the
# iOS runtime". No Apple certificate needed (ad-hoc / Simulator).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/.." && pwd)"
GO="${GO:-go}"

tmp="$(mktemp -d)"
UDID=""
cleanup() { [[ -n "$UDID" ]] && xcrun simctl shutdown "$UDID" >/dev/null 2>&1 || true; rm -rf "$tmp"; }
trap cleanup EXIT

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

# Build for the iOS Simulator (arm64) and ad-hoc sign so it can run.
xcrun --sdk iphonesimulator clang -arch arm64 -mios-simulator-version-min=15.0 "$tmp/sample.c" -o "$tmp/Sample"
codesign -f -s - "$tmp/Sample"

# Pick (or create) an available iPhone simulator and boot it.
UDID=$(xcrun simctl list devices available -j | python3 -c '
import json,sys
d=json.load(sys.stdin)["devices"]
for rt,devs in d.items():
    if "iOS" in rt:
        for x in devs:
            if "iPhone" in x["name"]:
                print(x["udid"]); sys.exit()
')
if [[ -z "$UDID" ]]; then
  RT=$(xcrun simctl list runtimes -j | python3 -c 'import json,sys;rs=[r for r in json.load(sys.stdin)["runtimes"] if r.get("isAvailable") and "iOS" in r["name"]];print(rs[-1]["identifier"])')
  DT=$(xcrun simctl list devicetypes -j | python3 -c 'import json,sys;ts=[t for t in json.load(sys.stdin)["devicetypes"] if "iPhone" in t["name"]];print(ts[-1]["identifier"])')
  UDID=$(xcrun simctl create shield-sim "$DT" "$RT")
fi
echo "ios-sim-gate: using simulator $UDID"
xcrun simctl boot "$UDID" >/dev/null 2>&1 || true
xcrun simctl bootstatus "$UDID" -b >/dev/null

# Unprotected run inside the iOS Simulator runtime.
xcrun simctl spawn "$UDID" "$tmp/Sample" > "$tmp/plain.out"

# Protect (strip + ad-hoc re-sign) through the Go worker via an IPA round-trip.
python3 - "$tmp" <<'PY'
import sys, zipfile
d = sys.argv[1]
with zipfile.ZipFile(d + "/in.ipa", "w", zipfile.ZIP_DEFLATED) as z:
    z.write(d + "/Sample", "Payload/Sample.app/Sample")
    z.writestr("Payload/Sample.app/Info.plist", b"<plist/>")
PY
( cd "$repo" && "$GO" run ./cmd/shield-iosstrip \
    --in "$tmp/in.ipa" --out "$tmp/out.ipa" \
    --strip "xcrun strip -x" --sign "codesign -f -s -" )
python3 - "$tmp" <<'PY'
import sys, zipfile
d = sys.argv[1]
with zipfile.ZipFile(d + "/out.ipa") as z:
    open(d + "/stripped", "wb").write(z.read("Payload/Sample.app/Sample"))
PY
chmod +x "$tmp/stripped"

# Protected run inside the same simulator.
xcrun simctl spawn "$UDID" "$tmp/stripped" > "$tmp/strip.out"

if diff -q "$tmp/plain.out" "$tmp/strip.out" >/dev/null; then
  echo "ios-sim-gate: PASS — protected binary runs identically in the iOS Simulator"
else
  echo "ios-sim-gate: FAIL — output diverged in the iOS Simulator" >&2
  diff -u "$tmp/plain.out" "$tmp/strip.out" || true
  exit 1
fi
