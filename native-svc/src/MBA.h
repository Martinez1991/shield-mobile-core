// Mixed boolean-arithmetic substitution for native code (issue #82, ADR 0004).
#ifndef NATIVE_SVC_MBA_H
#define NATIVE_SVC_MBA_H

#include "llvm/IR/Function.h"
#include <cstdint>

// mbaFunction rewrites integer add/sub/and/or/xor into equivalent but more
// complex mixed boolean-arithmetic expressions, hiding the original operation.
// Identities hold in wrapping two's-complement arithmetic, so the low bits are
// always exact. Returns true if anything changed.
bool mbaFunction(llvm::Function &F, uint64_t seed);

#endif
