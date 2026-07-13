// Native anti-tamper (self-checksum) injection (issue #82/#84, ADR 0004).
#ifndef NATIVE_SVC_TAMPER_H
#define NATIVE_SVC_TAMPER_H

#include "llvm/IR/Module.h"
#include <cstdint>

// tamperModule moves the module's functions into a `shieldtext` section and
// injects a load-time constructor that sums that section's bytes (between the
// linker-defined __start_shieldtext/__stop_shieldtext) and compares them to
// __shield_tamper_expected. That expected value is a sentinel here and must be
// written post-link by tools/tamper-patch.py; if the code is patched afterward
// the sum diverges and the process exits. Returns true if injected.
bool tamperModule(llvm::Module &M, uint64_t seed);

#endif
