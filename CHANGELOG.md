# Changelog

All notable changes to SHIELD. Format loosely follows [Keep a Changelog];
versions are git tags with a matching GitHub release.

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

[0.2.0]: https://github.com/Martinez1991/shield-platform/releases/tag/v0.2.0
[0.1.0]: https://github.com/Martinez1991/shield-platform/releases/tag/v0.1.0
[Keep a Changelog]: https://keepachangelog.com/
