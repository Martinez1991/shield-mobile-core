# native-svc — LLVM native obfuscation service

Out-of-tree LLVM component behind the Go seam `internal/nativesvc`
([ADR 0004](../docs/adr/0004-llvm-native-svc.md), issue
[#82](https://github.com/Martinez1991/shield-platform/issues/82)). It is a
standalone executable invoked as a subprocess by the sandboxed worker — **never
linked into the Go build**, so the SHIELD engine stays stdlib-only and CGO-free.

## Contract

```
native-svc transform --arch <abi> --seed <n> --pass <p> [--pass <p> ...]  < in.bc > out.bc
```

Reads LLVM bitcode/IR on stdin, applies the passes, verifies the module, writes
bitcode on stdout. Deterministic given the same input, passes and seed.

Passes: `flatten` (control-flow flattening) and `mba` (mixed boolean-arithmetic
substitution) are implemented and can be composed; `opaque` and `strings` are
declared in the contract and return "not implemented yet" (exit 3) until built —
never a silent no-op.

## Build (Ubuntu 24.04 / WSL2)

```bash
sudo apt update
sudo apt install -y llvm-18-dev clang-18 cmake ninja-build build-essential

cmake -S native-svc -B native-svc/build -G Ninja \
      -DLLVM_DIR=/usr/lib/llvm-18/lib/cmake/llvm
cmake --build native-svc/build
```

Produces `native-svc/build/native-svc`. Put it on the worker's PATH (or point
`$SHIELD_NATIVE_SVC` at it) and `internal/nativesvc` will pick it up.

## Execution gate (issue #82 acceptance)

```bash
native-svc/test/gate.sh
```

Compiles `test/sample.c` to bitcode, flattens it, asserts the dispatcher was
introduced (more basic blocks + a `switchVar`), then compiles and runs both and
diffs the output — proving the transform is functionally identical.

## How it plugs into the pipeline

The worker compiles a recompilable native source/bitcode → `.bc`, pipes it
through `native-svc`, then compiles `.bc` → `.so`. Pre-linked, stripped `.so`
without bitcode cannot be flattened (LLVM passes need IR), which is why this
operates on bitcode, as ADR 0004 anticipated.
