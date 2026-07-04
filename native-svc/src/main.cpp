// native-svc — the out-of-tree LLVM obfuscation service (issue #82, ADR 0004).
//
// Contract (ADR 0004):
//   native-svc transform --arch <abi> --seed <n> --pass <p> [--pass <p> ...]
//        < in.bc  > out.bc
//
// Reads LLVM bitcode/IR on stdin, applies the selected passes, verifies the
// result, and writes bitcode on stdout. Deterministic given the same input,
// passes and seed. Exit codes: 0 ok, 1 I/O/parse, 2 usage, 3 unknown pass,
// 4 verification failed.
#include "Flatten.h"

#include "llvm/Bitcode/BitcodeWriter.h"
#include "llvm/IR/LLVMContext.h"
#include "llvm/IR/Module.h"
#include "llvm/IR/Verifier.h"
#include "llvm/IRReader/IRReader.h"
#include "llvm/Support/MemoryBuffer.h"
#include "llvm/Support/SourceMgr.h"
#include "llvm/Support/raw_ostream.h"

#include <cstdint>
#include <cstdlib>
#include <string>
#include <vector>

using namespace llvm;

static int usage(const std::string &msg) {
  errs() << "native-svc: " << msg << "\n"
         << "usage: native-svc transform --arch <abi> --seed <n> "
            "--pass flatten [--pass ...]  < in.bc > out.bc\n";
  return 2;
}

int main(int argc, char **argv) {
  if (argc < 2 || std::string(argv[1]) != "transform")
    return usage("expected the 'transform' subcommand");

  std::string arch;
  uint64_t seed = 0;
  std::vector<std::string> passes;

  for (int i = 2; i < argc; i++) {
    std::string a = argv[i];
    auto need = [&](const char *flag) -> std::string {
      if (i + 1 >= argc) {
        std::exit(usage(std::string("missing value for ") + flag));
      }
      return argv[++i];
    };
    if (a == "--arch")
      arch = need("--arch");
    else if (a == "--seed")
      seed = std::strtoull(need("--seed").c_str(), nullptr, 10);
    else if (a == "--pass")
      passes.push_back(need("--pass"));
    else
      return usage("unknown flag " + a);
  }
  (void)arch; // informational for now; passes below are arch-independent IR work
  if (passes.empty())
    return usage("no --pass given");

  LLVMContext ctx;
  ErrorOr<std::unique_ptr<MemoryBuffer>> buf = MemoryBuffer::getSTDIN();
  if (std::error_code ec = buf.getError()) {
    errs() << "native-svc: reading stdin: " << ec.message() << "\n";
    return 1;
  }
  SMDiagnostic err;
  std::unique_ptr<Module> mod = parseIR((*buf)->getMemBufferRef(), err, ctx);
  if (!mod) {
    err.print("native-svc", errs());
    return 1;
  }

  for (const std::string &p : passes) {
    if (p == "flatten") {
      for (Function &F : *mod)
        flattenFunction(F, seed);
    } else {
      errs() << "native-svc: pass '" << p << "' not implemented yet\n";
      return 3;
    }
  }

  if (verifyModule(*mod, &errs())) {
    errs() << "native-svc: transformed module failed verification\n";
    return 4;
  }

  WriteBitcodeToFile(*mod, outs());
  outs().flush();
  return 0;
}
