# Changelog

All notable changes to SHIELD. Format loosely follows [Keep a Changelog];
versions are git tags with a matching GitHub release.

## [Unreleased]

- **Native RASP — anti-debug** (`native-svc` `rasp` pass, #84): injects a load-time
  check (reads `/proc/self/status` TracerPid via libc) that exits the process if a
  debugger is attached and is silent otherwise, so an undebugged run stays
  functionally identical. Its globals are named `__shield_*` so the `strings` pass
  skips them. `native-svc/test/rasp-gate.sh` proves both properties with a
  self-contained `ptrace` tracer (no strace/gdb needed), including the full
  `rasp+flatten+mba+opaque+strings` composition.
- **Native RASP — anti-tamper** (`native-svc` `tamper` pass, #84): moves functions
  into a `shieldtext` section and injects a load-time self-checksum over it
  (between the linker-defined `__start_/__stop_shieldtext`), compared to a
  post-link-patched `__shield_tamper_expected`. `tools/tamper-patch.py` (pure
  stdlib ELF) writes the real checksum after linking; if the code is patched
  afterward, the runtime sum diverges and the process exits. `test/tamper-gate.sh`
  proves unpatched→detected, patched→identical, tampered→detected — closing #84.

## [0.4.0] — 2026-07-05

Native code protection via an out-of-tree LLVM toolchain, plus binary analysis in
the CLI. The Go engine is unchanged — stdlib-only, deterministic, CGO-free, and
the golden/ART gate stays green; the new native work lives entirely in the
`native-svc` subprocess and its Go seam.

- **`shield analyze <app.ipa|.apk|.aab>`** now inspects the binaries inside an app
  artifact — Mach-O (IPA) and ELF `.so` (APK/AAB) — reporting architecture,
  structure and secret-string density (`internal/inspect`, #87). Read-only,
  stdlib-only.
- **Native LLVM protection** ([ADR 0004](docs/adr/0004-llvm-native-svc.md), #82):
  LLVM passes ship as an out-of-tree `native-svc` subprocess (never linked;
  `go.mod`/engine unchanged, CGO-free). `internal/nativesvc` is the Go seam —
  pass model, policy `native` section, `native-svc` discovery with a typed
  `ErrUnavailable`, the subprocess contract (injectable runner), and an offline
  `Plan`. The `native-svc` executable implements four composable passes over LLVM
  bitcode — **control-flow flattening**, **mixed boolean-arithmetic (MBA)**
  substitution, **opaque predicates** (always-true, via a volatile global,
  guarding bogus junk blocks) and **string encryption** (XOR local literals with
  a load-time decryptor) — each verified by an **execution gate**
  (`native-svc/test/gate.sh`, `native` CI workflow) proving the transformed
  program is functionally identical (the strings gate also asserts the plaintext
  is absent from the binary and restored at runtime).
  An **end-to-end native-library flow** (`tools/protect-so.sh`) compiles source →
  bitcode → `native-svc` → `.so`; `test/ndk-gate.sh` `dlopen`s and runs the
  protected host `.so` (functional identity) and, with the Android NDK, builds a
  real **arm64 Android `.so`** through the same flow. `test/arm64-exec-gate.sh`
  then **runs the protected arm64 binary under qemu-user** and asserts it executes
  identically — the native counterpart of the golden/ART gate, ISA-level.

## [0.3.0] — 2026-07-03

Risk-driven protection (the AI risk-map v0) plus the analysis foundations for iOS
and native code. The engine stays pure Go / stdlib-only and deterministic — the
new binary inspectors use only `debug/macho` and `debug/elf`.

### Risk-driven Planner (AI risk-map v0, #65)
- `internal/risk`: deterministic per-method static features (complexity, sensitive
  calls to crypto/keystore/net/reflection, secret-string density) over the typed
  IR, and an explainable heuristic score — no ML (#67, #68).
- Policy `risk.enabled` + `risk.threshold`: the expensive passes (VM, flattening)
  only transform methods above the threshold, concentrating cost on the hot spots
  instead of uniformly (#69). Default (risk off) is byte-for-byte unchanged and
  ART-green.
- `Result.RiskMap`: per-method score, reasons and protect decision, for audit (#70).

### iOS foundation (#63)
- `internal/ios`: IPA detection, app-bundle/binary/framework location, and a
  byte-preserving IPA repack (#74); Mach-O inspection over `debug/macho` —
  segments, sections, symbols, architecture, and `__cstring` secret density (#75).
  `apk.Protect` recognizes an IPA with a clear "not yet available" message.

### Native (Android .so) foundation (#64)
- `internal/native`: ELF `.so` inspection over `debug/elf` — sections, symbols,
  machine, and `.rodata` secret density; `lib/<abi>/*.so` classification and
  whole-archive inspection (#81).

> The invasive native/iOS transforms (LLVM passes, native/Mach-O code injection,
> re-signing) and their execution gates are decomposed as follow-up sub-issues;
> they need an external toolchain and macOS/native infra and are deferred.

## [0.2.0] — 2026-07-03

Expanded code virtualization on a new typed IR, Android App Bundle support, and
the platform layer (sandboxed queue worker, observability, field RASP ingest).

The **engine stays pure Go / stdlib-only and deterministic**. Two external
dependencies were introduced deliberately and confined to platform packages
(never reachable from the engine): NATS (`internal/queue`, ADR 0002) and
OpenTelemetry (`internal/obs`, ADR 0003).

Every transform that changes emitted smali is verified **byte-for-byte on real
ART** by the golden differential correctness gate (issue #3).

### Typed IR (#20, ADR 0001)
- New `internal/ir`: structured smali instruction model, register **type
  inference** (lattice + forward dataflow fixpoint over the CFG), and **liveness /
  def-use** (backward dataflow). Go-native, not dexlib2 — preserving the zero-dep,
  deterministic engine.

### Code virtualization / VM (#14, #48–#50)
- Long / 64-bit arithmetic, narrowing conversions, and **long method parameters**.
- **Objects**: reference params, `move-object`, `return-object`; unified Object
  return ABI. **Reference/wide flattening** gated by IR type-consistency (#48).
- **`const-string` virtualization** into a per-method string pool (#42).
- **Data-driven `invoke`** via reflection: `invoke-static` (#44) plus
  `invoke-virtual`/`invoke-interface` with a receiver (#50), and int/long/object
  args and returns (#49).

### Control-flow flattening (#20, #43)
- New `passFlatten`: rewrites a method into a central `packed-switch` dispatcher,
  driven by the typed IR (type-consistency gate + dead state register from
  liveness) so the dispatcher join introduces no verifier type conflict.

### Android App Bundle (#16, #51)
- `shield protect app.aab`: bundle round-trip that protects each module's DEX and
  copies every other entry byte-for-byte (pure-Go zip).
- **Protobuf manifest keep-rules**: a hand-rolled aapt2 `XmlNode` decoder extracts
  component names so renaming stays safe on AAB.

### Platform: worker, queue, autoscaling (#18, #52, ADR 0002)
- `internal/queue`: `Queue` interface with `MemQueue`, `DirQueue`, and a **NATS
  JetStream** backend. `internal/worker`: concurrent consumer with graceful
  shutdown. `cmd/shield-worker`.
- `deploy/`: hardened Dockerfile + K8s manifests (gVisor, deny-all-egress,
  read-only rootfs, ephemeral) and a KEDA ScaledObject scaling on queue depth.

### Observability (#21, #53, ADR 0003)
- `internal/obs`: dependency-free Prometheus text metrics (per-stage latency,
  build counters, queue depth), OpenTelemetry-shaped spans exported over **OTLP**
  (opt-in `--otlp-endpoint`), and per-pass timings recorded by the engine.
- `deploy/observability/`: Prometheus config, alert rules, Grafana dashboard.

### Field RASP callback ingest (#54)
- `cmd/rasp-ingest`: receives runtime attestation/tamper callbacks, authenticated
  by per-tenant HMAC-SHA256 with timestamp-window + nonce anti-replay, emitting
  `shield_rasp_events_total` / `shield_rasp_rejected_total`.

## [0.1.0] — 2026-07-02

Obfuscation engine MVP: rename (class/member, manifest keep-rules), string
encryption (XOR/AES-256-GCM), control-flow (reorder, opaque predicates, junk),
straight-line integer code virtualization, RASP injection, policy-as-code, the
CLI (`analyze`/`obfuscate`/`protect`/`policy`/`retrace`), and the golden/ART
runtime-correctness gate.

[0.4.0]: https://github.com/Martinez1991/shield-platform/releases/tag/v0.4.0
[0.3.0]: https://github.com/Martinez1991/shield-platform/releases/tag/v0.3.0
[0.2.0]: https://github.com/Martinez1991/shield-platform/releases/tag/v0.2.0
[0.1.0]: https://github.com/Martinez1991/shield-platform/releases/tag/v0.1.0
[Keep a Changelog]: https://keepachangelog.com/
