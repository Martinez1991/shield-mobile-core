# ADR 0004 — LLVM-based native protection as an out-of-tree subprocess (`native-svc`)

- Status: Accepted
- Date: 2026-07-04
- Issue: [#82](https://github.com/Martinez1991/shield-platform/issues/82)
- Related: [#64](https://github.com/Martinez1991/shield-platform/issues/64) (native epic), [#18](https://github.com/Martinez1991/shield-platform/issues/18) (sandboxed worker), [ADR 0002](0002-nats-queue.md), [ADR 0003](0003-otlp-tracing.md)

## Context

Android apps ship native code as ELF `.so` under `lib/<abi>/` (inspected read-only
by `internal/native`, #81). The strongest native obfuscation — CFG flattening,
mixed boolean-arithmetic (MBA), opaque predicates, native string/const
encryption — is done with **LLVM passes**, which are C++ against the LLVM pass
API. That is a different language and a heavy build toolchain (LLVM/Clang), and it
operates on bitcode/objects, not on our Go data structures.

Two hard constraints frame the decision:

- The **engine is stdlib-only and deterministic** (ADR 0001) and must stay that
  way — the golden/ART gate depends on it. Nothing here may reach the engine.
- Unlike NATS (ADR 0002) and OTel (ADR 0003), an LLVM pass **cannot be a Go
  module dependency**. There is no pure-Go LLVM; a linked binding needs CGO + a
  local LLVM. Adding that to `go.mod`/CGO would compromise the reproducible,
  cross-compilable, zero-CGO build of the whole platform.

## Decision

**Ship native LLVM protection as a separate out-of-tree executable, `native-svc`,
invoked as a subprocess from the sandboxed worker — never linked into the Go
build. Model it in Go behind a thin seam (`internal/nativesvc`), exactly as
`internal/apk` already shells out to `apktool`/`apksigner`.**

- `go.mod` gains **nothing**. `go build ./...` links no LLVM. The engine's
  dependency graph and the golden/ART gate are untouched.
- `native-svc` is discovered on PATH (or via `$SHIELD_NATIVE_SVC`). When absent,
  callers get a **typed `ErrUnavailable`** and degrade gracefully — the Android
  DEX pipeline is unaffected — mirroring the "apktool not found" path.
- It runs inside the existing no-egress, gVisor-sandboxed worker (#18): the
  toolchain never touches the control plane or the network.

### The subprocess contract (what `native-svc` must implement)

A single object in, a single object out, passes selected by flags:

```
native-svc transform --arch <abi> --pass <p> [--pass <p> …]   < object.o  > object.o
```

- reads the object/bitcode on **stdin**, writes the transformed object to
  **stdout**, diagnostics on **stderr**, non-zero exit on failure;
- is **deterministic** given the same input, passes and a per-build seed
  (`--seed`), so a protected build stays reproducible.

`internal/nativesvc` owns this contract on the Go side (request framing, pass
selection from policy, subprocess execution with an injectable runner for tests),
so `native-svc` can be built and swapped independently.

### Why a subprocess, not a CGO binding or a network service

- **Subprocess** keeps `go.mod`/CGO pristine, matches the existing `apktool`
  idiom, and isolates a crash/miscompile in the LLVM toolchain from the Go
  process. This is the choice.
- **CGO binding** would force LLVM onto every build host and break the zero-CGO,
  cross-compiled release. Rejected.
- **Network microservice** adds egress and a deployment surface the no-egress
  worker forbids; a local exec is simpler and stays inside the sandbox. Rejected
  for now (a co-located service is a later scaling option, same contract).

## Scope delivered here vs. deferred

**Delivered (Go, offline-testable, zero-dep):** the ADR, the `internal/nativesvc`
seam — pass model, policy `native` section, `native-svc` discovery/`ErrUnavailable`,
subprocess request/response framing with an injectable runner, and an
offline `Plan` over an APK/AAB (which `.so` are candidates, reusing #81).

**Delivered since (needs the LLVM toolchain, so gated in the `native` CI
workflow, not the Go CI):** the `native-svc` executable with **control-flow
flattening** and **mixed boolean-arithmetic (MBA)** substitution over LLVM
bitcode (`native-svc/`, built out-of-tree with LLVM 18), composable, and their
**execution gate** (`native-svc/test/gate.sh`) — the #82 acceptance criterion:
each pass (and the composition) transforms the bitcode and is proven functionally
identical by compiling and diffing its output. Determinism-per-seed and the
contract exit codes are covered too.

The compile→transform→link flow is realized in `native-svc/tools/protect-so.sh`
and gated end-to-end by `native-svc/test/ndk-gate.sh`: the host path `dlopen`s and
runs the protected `.so` (functional identity), and — with the Android NDK — the
same flow builds a real **arm64 Android `.so`** (structural gate; running it needs
an emulator). LLVM 18-written bitcode links cleanly with the NDK's clang.

The four obfuscation passes (`flatten`, `mba`, `opaque`, `strings`) plus RASP
`rasp` (anti-debug) and `tamper` (self-checksum anti-tamper) passes (#84) are
implemented and composable, each covered by an execution gate — including
`arm64-exec-gate.sh` (protected arm64 runs identically under qemu-user),
`rasp-gate.sh` (silent normally, exits under a ptrace tracer) and `tamper-gate.sh`
(unpatched/tampered detected, patched identical). The `tamper` pass has a
post-link step, `tools/tamper-patch.py`, that stamps the real section checksum.

The Go worker drives the whole APK/AAB flow in `internal/nativesvc.ProtectArchive`
(`cmd/shield-nativeapk`): recompilable native modules ship as bitcode sidecars
`lib/<abi>/<name>.so.bc`, each transformed → linked → tamper-patched → repackaged
byte-for-byte, plain `.so` left untouched. The link/patch steps are injected, so
the orchestration is offline-tested; `apk-flow-gate.sh` proves the round-trip end
to end (runs identically, detects tampering).

An **on-device execution gate** (`native-svc/test/ondevice-gate.sh`) runs the
protected arm64 binary on a real Android device over adb and requires identical
output, plus anti-tamper (patched → identical, a flipped code byte → exit 67) —
verified on a Galaxy S24 (Bionic + ART). It is manual/local (no device in CI), so
it is not wired into a workflow; the qemu-user gate is the CI counterpart.

The native protection loop is complete: analyze → six passes → execution gates
(host, arm64 qemu-user, and on-device) → worker-driven APK round-trip.

## Consequences

- No change to `go.mod`, the engine, or the golden/ART gate. The platform still
  builds with zero CGO and cross-compiles.
- The worker gains an optional native stage that is a no-op (typed skip) unless
  `native-svc` is installed in its image; deploying it is an image/infra decision,
  not a code dependency.
- `internal/nativesvc` fixes the boundary so the C++ side can be developed against
  a stable Go-side contract, tested here today with a fake runner.
