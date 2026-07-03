# ADR 0002 — NATS JetStream as the production queue backend

- Status: Accepted
- Date: 2026-07-03
- Issue: [#52](https://github.com/Martinez1991/shield-platform/issues/52)
- Related: [#18](https://github.com/Martinez1991/shield-platform/issues/18) (the worker + `queue.Queue` seam), [ADR 0001](0001-typed-ir.md)

## Context

The worker (#18) consumes from a `queue.Queue` with two stdlib-only
implementations — `MemQueue` (dev/test) and `DirQueue` (a shared-volume file
inbox). Neither offers durable, at-least-once delivery with a consumer-lag signal
across many pods; production scale needs a broker.

The engine and its tooling have been **stdlib-only** on purpose (ADR 0001), and
every follow-up so far avoided external dependencies. A real broker adapter
cannot: it needs a client library. This is the deliberate first exception.

## Decision

**Adopt NATS JetStream, via the pure-Go `github.com/nats-io/nats.go` client, as
the production queue backend. Confine the dependency to `internal/queue`.**

- The **engine stays stdlib-only** — it never imports the queue package. The dep
  is reachable only from the worker path.
- `NatsQueue` implements the existing `queue.Queue` interface, so nothing else
  (worker, metrics, KEDA-by-depth) changes.

### Why NATS over Kafka

- **Pure Go, small tree.** `nats.go` pulls only nkeys/nuid/compress/x-crypto.
  The good Kafka clients either need CGO (`confluent-kafka-go`) or are heavier.
- **JetStream fits the model.** A durable stream + a shared durable pull consumer
  gives competing consumers, explicit ack/nak (at-least-once), and a `NumPending`
  count that is exactly `Queue.Depth()` and the KEDA `nats-jetstream` scaler
  signal.
- **Operationally light** for a build queue's throughput.

Kafka remains a valid alternative for teams already running it; a second adapter
against the same interface is a follow-up, not a rewrite.

## Testing

`nats.go` is the only added runtime dependency. Behaviour is delegated to that
mature library; the adapter is thin (publish to a subject, fetch from a pull
consumer, ack/nak, pending count). It is:

- **compile- and conformance-checked** always (`var _ Queue = (*NatsQueue)(nil)`);
- **integration-tested** against a real server when `SHIELD_NATS_URL` is set
  (skipped in CI to keep the default suite offline and broker-free).

We deliberately did **not** vendor the full embedded `nats-server` just for a
test — it drags in a large transitive tree (go-tpm, jwt, antithesis-sdk) that is
disproportionate for a priority-low adapter. The env-gated integration test plus
compile-time conformance is the proportionate bar.

## Consequences

- go.mod gains `nats.go` (+ its small tree). The engine's dependency graph is
  unchanged; `go build ./internal/engine/...` links nothing new.
- Deploy: the worker selects the backend at runtime (`--nats-url` → NATS, else the
  filesystem `DirQueue`); the KEDA ScaledObject gains a `nats-jetstream` trigger
  option.
- Determinism/offline guarantees for the **engine and its golden/ART gate** are
  untouched; only the queue path can now reach a network broker.
