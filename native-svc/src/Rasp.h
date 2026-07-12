// Native RASP (anti-debug) injection (issue #82/#84, ADR 0004).
#ifndef NATIVE_SVC_RASP_H
#define NATIVE_SVC_RASP_H

#include "llvm/IR/Module.h"
#include <cstdint>

// raspModule injects a runtime anti-debug check (reads /proc/self/status and
// looks at TracerPid) as a global constructor: if a debugger is attached the
// process exits immediately; otherwise it is silent, so an undebugged run is
// functionally identical. A module-level transform. Returns true if injected.
bool raspModule(llvm::Module &M, uint64_t seed);

#endif
