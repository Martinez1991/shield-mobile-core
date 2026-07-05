// Native string-literal encryption (issue #82/#83, ADR 0004).
#ifndef NATIVE_SVC_STRINGS_H
#define NATIVE_SVC_STRINGS_H

#include "llvm/IR/Module.h"
#include <cstdint>

// stringsModule XOR-encrypts eligible local string constants in place and injects
// a global constructor that decrypts them at load time (before main), so static
// analysis of the binary sees ciphertext while the program runs unchanged. A
// module-level transform (globals + one ctor), not per-function. Returns true if
// anything was encrypted.
bool stringsModule(llvm::Module &M, uint64_t seed);

#endif
