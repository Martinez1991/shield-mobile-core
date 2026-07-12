// Native string-literal encryption (issue #82/#83, ADR 0004).
//
// Each eligible local string constant (`private`/`internal` `[N x i8]`, e.g. the
// `.str` literals clang emits) is XOR-encrypted in place with a seed-derived key
// and made writable. A synthesized global constructor decrypts every one at load
// time (before main), so `strings`/hexdump on the binary sees ciphertext while
// the running program is byte-for-byte unchanged. The decryptor is unrolled per
// byte, so the key lives as immediates in the code, not as a plaintext blob.
#include "Strings.h"

#include "llvm/IR/Constants.h"
#include "llvm/IR/DerivedTypes.h"
#include "llvm/IR/Function.h"
#include "llvm/IR/GlobalVariable.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/Transforms/Utils/ModuleUtils.h"

#include <cstdint>
#include <string>
#include <vector>

using namespace llvm;

static uint32_t nextRand(uint64_t &s) {
  s = s * 6364136223846793005ULL + 1442695040888963407ULL;
  return static_cast<uint32_t>(s >> 33);
}

namespace {
struct Enc {
  GlobalVariable *g;
  std::vector<uint8_t> key;
  uint64_t len;
};
} // namespace

// Cap so a pathologically large blob doesn't explode the unrolled decryptor.
static const uint64_t kMaxLen = 4096;

bool stringsModule(Module &M, uint64_t seed) {
  LLVMContext &ctx = M.getContext();
  IntegerType *i8 = Type::getInt8Ty(ctx);
  uint64_t rng = seed ^ 0x5DEECE66DULL;

  std::vector<Enc> encs;
  for (GlobalVariable &G : M.globals()) {
    if (!G.hasInitializer() || !G.isConstant() || !G.hasLocalLinkage())
      continue;
    if (G.getName().starts_with("llvm.") || G.getName().starts_with("__shield"))
      continue; // skip LLVM metadata and our own runtime globals (e.g. RASP)
    auto *init = dyn_cast<ConstantDataArray>(G.getInitializer());
    if (!init || !init->getElementType()->isIntegerTy(8))
      continue;
    StringRef data = init->getRawDataValues();
    if (data.empty() || data.size() > kMaxLen)
      continue;

    // Seed-derived key (4..11 bytes), guaranteed non-trivial.
    std::vector<uint8_t> key(4 + (nextRand(rng) % 8));
    for (uint8_t &k : key)
      k = static_cast<uint8_t>(nextRand(rng));
    key[0] |= 1u;

    std::string enc(data.size(), '\0');
    for (size_t i = 0; i < data.size(); i++)
      enc[i] = static_cast<char>(static_cast<uint8_t>(data[i]) ^ key[i % key.size()]);

    G.setInitializer(ConstantDataArray::getRaw(enc, data.size(), i8));
    G.setConstant(false); // writable so the decryptor can work in place
    encs.push_back({&G, std::move(key), data.size()});
  }
  if (encs.empty())
    return false;

  // Decryptor: for each global, XOR every byte back with its key.
  Function *ctor = Function::Create(FunctionType::get(Type::getVoidTy(ctx), false),
                                    GlobalValue::InternalLinkage,
                                    "__shield_decrypt_strings", &M);
  IRBuilder<> B(BasicBlock::Create(ctx, "entry", ctor));
  for (const Enc &e : encs) {
    Type *arrTy = e.g->getValueType();
    for (uint64_t i = 0; i < e.len; i++) {
      Value *p = B.CreateInBoundsGEP(arrTy, e.g, {B.getInt64(0), B.getInt64(i)});
      Value *b = B.CreateLoad(i8, p);
      Value *x = B.CreateXor(b, ConstantInt::get(i8, e.key[i % e.key.size()]));
      B.CreateStore(x, p);
    }
  }
  B.CreateRetVoid();

  appendToGlobalCtors(M, ctor, /*Priority=*/0);
  return true;
}
