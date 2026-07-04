// Control-flow flattening for native code (issue #82, ADR 0004).
#ifndef NATIVE_SVC_FLATTEN_H
#define NATIVE_SVC_FLATTEN_H

#include "llvm/IR/Function.h"
#include <cstdint>

// flattenFunction rewrites F into a single dispatch loop driven by a switch on a
// stack "state" variable, hiding the original control-flow graph. It is a no-op
// (returns false) for declarations, trivial functions, and functions using
// exception handling. seed makes the state labels reproducible per build.
bool flattenFunction(llvm::Function &F, uint64_t seed);

#endif
