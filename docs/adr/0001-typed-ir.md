# ADR 0001 — Typed IR: grow a Go-native one, do not bridge to dexlib2

- Status: Accepted
- Date: 2026-07-03
- Issue: [#20](https://github.com/Martinez1991/shield-platform/issues/20)
- Related: [#14](https://github.com/Martinez1991/shield-platform/issues/14) (VM coverage — the immediate consumer)

## Context

The engine's editable IR is, today, **smali text manipulated line-by-line with
regexes** (`internal/smali`, and every pass on top of it). This is the
"structural debt" #20 calls out: without reconstructing register **types** and
**liveness**, three things stay out of reach or unverifiable:

1. **Control-flow flattening** with a central dispatcher (needs liveness to move
   values across the dispatcher without clobbering).
2. **Data-driven `invoke`** inside the code-virtualization VM (needs per-register
   types to marshal call arguments into the right slots — see #14).
3. **Type-aware pass coordination**, e.g. virtualizing a `const-string` that the
   string-encryption pass would otherwise rewrite first (a pass-ordering problem
   surfaced while closing out #14).

Note we are **not** starting from zero: the VM compiler
(`internal/engine/vm.go:compileMethod`) already parses instructions into a
structured form, resolves labels in two passes, and tracks a register file. That
is an ad-hoc, VM-local proto-IR. #20 is about **promoting it to a first-class,
reusable, typed IR**.

The issue text proposes adopting **dexlib2**. dexlib2 is a Java library. The
engine is deliberately **Go, standard-library only, zero external dependencies**
— a property we have defended in every slice (deterministic, offline, fully
testable builds; the only JVM we shell out to is `smali.jar`, and only in CI to
*assemble* and ART-test output, never inside the engine). So the dexlib2 proposal
is in direct tension with a core project principle and cannot be taken literally.

## Decision

**Grow a Go-native, typed IR incrementally over the smali we already parse. Do
not bridge the engine to dexlib2 or any JVM process for its core IR.**

We add an `internal/ir` package that models a method as structured instructions
and reconstructs register types (and, later, liveness) by dataflow analysis. It
grows **only as far as the transforms that consume it require** — not as a
dexlib2 clone.

## Options considered

### A. Bridge to dexlib2 (JVM subprocess) — rejected
- Pros: mature, complete type/liveness; battle-tested on real-world dex.
- Cons: puts a **JVM on the engine's core path**, not just the test path. Breaks
  the self-contained pure-Go engine, deterministic packaging, and cross-platform
  story. Most of dexlib2's surface (full dex parsing, encoding) is irrelevant —
  we operate on smali *after* baksmali. High integration/marshalling cost for a
  capability we only need a slice of.

### B. Full Go-native DEX + IR reimplementation — deferred
- Pros: pure Go; complete.
- Cons: very large and premature. We manipulate smali text, not raw `.dex`, so a
  from-scratch dex reader/writer + full type system is far more than any current
  transform needs.

### C. Incremental Go-native typed IR over smali (chosen)
- Pros: preserves zero-deps and determinism; reuses the proto-IR already inside
  the VM pass; each increment is independently unit-testable, and ART-verifiable
  the moment it drives a transform. Scales effort to actual need.
- Cons: we own the type/liveness rules (must be correct); coverage grows opcode
  by opcode. Mitigated by unit tests per opcode and the existing ART differential
  gate for any transform built on it.

## Consequences

- The engine stays pure Go / stdlib-only. No new runtime or build dependency.
- Correctness strategy is unchanged and layered: **unit tests** validate the
  analysis (types/liveness) in isolation; the **ART differential gate** (issue
  #3) validates any transform that consumes the IR, byte-for-byte on real ART.
- Coverage is bounded and honest: an opcode the IR does not yet model is
  reported as `Unknown`/conservative, and a transform that needs it simply
  declines to fire (as the VM already does when `compileMethod` bails).

## Incremental rollout (bricks)

1. **Structured instruction model + method parser** — `Insn`, register/operand
   parsing, method decl + body + labels. *(this PR)*
2. **Register type inference** — a small lattice (`Int/Long/Float/Double/Ref`)
   with a forward dataflow + fixpoint over the CFG (fallthrough, branches, goto),
   seeded from the method's parameter types. Unit-tested against golden methods.
   *(this PR)*
3. **Liveness / def-use** — backward dataflow; enables safe register reuse for
   flattening.
4. **Consume it in the VM** (#14 remainder, each ART-gated): type-directed
   `invoke` argument marshalling; type-aware `const-string` virtualization /
   coordination with the string-encryption pass.

Bricks 1–2 land here as analysis only (no transform, so unit tests are the gate).
Later bricks re-enter the ART loop as they change emitted smali.
