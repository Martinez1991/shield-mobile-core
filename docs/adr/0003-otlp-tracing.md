# ADR 0003 — OTLP trace export via the OpenTelemetry SDK

- Status: Accepted
- Date: 2026-07-03
- Issue: [#53](https://github.com/Martinez1991/shield-platform/issues/53)
- Related: [#21](https://github.com/Martinez1991/shield-platform/issues/21) (spans as logs), [ADR 0002](0002-nats-queue.md) (the dependency-isolation pattern)

## Context

Observability (#21) emits one span per pipeline pass as OpenTelemetry-shaped
structured logs (`trace_id`, `span_id`, `parent_span_id`, `duration_ms`) — no real
export. To view build traces in Tempo/Jaeger and correlate them with metrics, the
spans must be exported over OTLP, which needs the OpenTelemetry SDK.

ADR 0002 already established the pattern for a first external dependency (NATS,
confined to `internal/queue`). This is the second, following the same rule.

## Decision

**Export spans over OTLP via the OpenTelemetry Go SDK, with the dependency
confined to `internal/obs`.** The log-based spans remain the default; OTLP is
opt-in (an endpoint flag). When enabled, `obs.Tracer` also opens a real OTel span
per `Start`/`Child`, and reuses the OTel span's IDs for the log fields so logs and
exported traces correlate.

- The **engine stays stdlib-only** — it never imports `obs` tracing setup; only
  the worker wires OTLP. Engine determinism and the golden/ART gate are untouched.
- **Default is offline**: with no endpoint, no SDK exporter runs and only the
  structured-log spans are emitted (unchanged from #21).

## Testing

The span-generation bridge is verified **offline** with the SDK's in-memory
`tracetest.SpanRecorder` (no collector): spans, parent/child linkage and
attributes are asserted directly. Export to a real collector is exercised only
when `--otlp-endpoint` is set (documented; not run in CI). This is stronger than
ADR 0002's conformance-only default, because the SDK ships an in-process recorder.

## Consequences

- go.mod gains the OTel SDK + OTLP/HTTP exporter. Its tree is larger than NATS's
  (it pulls `google.golang.org/protobuf` and gRPC via the otlp proto module, even
  for the HTTP exporter) — an accepted cost of the standard OTLP stack, and still
  reachable only from the worker path, never the engine.
- The Prometheus metrics and Grafana dashboard (#21) are unchanged; traces now
  complement them.
- `SHIELD_OTLP_RASP`-style field callbacks (#54) can later feed the same tracer.
