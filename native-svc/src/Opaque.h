// Opaque-predicate insertion for native code (issue #82, ADR 0004).
#ifndef NATIVE_SVC_OPAQUE_H
#define NATIVE_SVC_OPAQUE_H

#include "llvm/IR/Function.h"
#include <cstdint>

// opaqueFunction guards each block with an always-true opaque predicate
// (x*(x+1) is always even, read through a volatile global so the optimizer can't
// fold it) branching to the real path or a bogus junk block. Both edges converge
// on the same continuation, so semantics are preserved regardless of the
// predicate — the obfuscation is in the extra, opaque control flow. Returns true
// if anything changed.
bool opaqueFunction(llvm::Function &F, uint64_t seed);

#endif
