#!/usr/bin/env bash
# Validates the code-virtualization VM (shield-platform.md §8) end-to-end on the
# JVM: Go emits a per-build opcode permutation + compiled bytecode + expected
# results; a Java port of the injected smali interpreter (scripts/jvm/VM.java)
# runs the bytecode and must reproduce them exactly. This confirms the on-device
# interpreter algorithm is correct (JVM int semantics match Android's ART).
#
# Requires Java (javac + java). No jars needed.
set -euo pipefail
cd "$(dirname "$0")/.."

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "==> emitting VM test vector from Go"
SHIELD_VM_VEC="$WORK/vec.txt" go test ./internal/engine -run TestVMDumpVector >/dev/null

echo "==> compiling JVM harness"
javac -d "$WORK" scripts/jvm/VM.java

echo "==> running interpreter on Go bytecode"
java -cp "$WORK" VM "$WORK/vec.txt"
