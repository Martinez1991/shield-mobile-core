// Native anti-tamper (self-checksum) injection (issue #82/#84, ADR 0004).
//
// The checksum of the code can only be known after linking, so this is a two-
// step scheme (post-link patch). At the IR level we:
//   1. move every real function into a section named `shieldtext` (a valid C
//      identifier, so the linker auto-defines __start_shieldtext/__stop_shieldtext
//      bracketing it exactly);
//   2. add a global __shield_tamper_expected initialized to a sentinel;
//   3. inject a constructor that sums the bytes in [__start, __stop) and, if the
//      sum != expected, _exit(67).
// tools/tamper-patch.py then writes the real sum into __shield_tamper_expected in
// the linked binary. If anyone patches the code afterward, the runtime sum no
// longer matches and the process bails. The expected value is read with a
// volatile load so the compiler can't fold the sentinel into the comparison.
#include "Tamper.h"

#include "llvm/IR/Constants.h"
#include "llvm/IR/DerivedTypes.h"
#include "llvm/IR/Function.h"
#include "llvm/IR/GlobalVariable.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/Transforms/Utils/ModuleUtils.h"

using namespace llvm;

static const int kTamperExit = 67;
static const uint64_t kSentinel = 0xD1AB011CA1147A11ULL; // patched post-link

bool tamperModule(Module &M, uint64_t seed) {
  (void)seed;
  if (M.getFunction("__shield_tamper_check"))
    return false; // idempotent

  LLVMContext &ctx = M.getContext();
  IntegerType *i8 = Type::getInt8Ty(ctx);
  IntegerType *i32 = Type::getInt32Ty(ctx);
  IntegerType *i64 = Type::getInt64Ty(ctx);
  PointerType *ptr = PointerType::get(ctx, 0);
  Type *voidTy = Type::getVoidTy(ctx);

  // Move real functions into the protected section.
  unsigned moved = 0;
  for (Function &F : M) {
    if (F.isDeclaration() || F.getName().starts_with("__shield"))
      continue;
    F.setSection("shieldtext");
    moved++;
  }
  if (moved == 0)
    return false;

  // Linker-defined section bounds + the (post-link patched) expected sum.
  ArrayType *a0 = ArrayType::get(i8, 0);
  auto *gStart = new GlobalVariable(M, a0, false, GlobalValue::ExternalLinkage, nullptr, "__start_shieldtext");
  auto *gStop = new GlobalVariable(M, a0, false, GlobalValue::ExternalLinkage, nullptr, "__stop_shieldtext");
  auto *gExpected = new GlobalVariable(M, i64, false, GlobalValue::InternalLinkage,
                                       ConstantInt::get(i64, kSentinel), "__shield_tamper_expected");

  FunctionCallee fExit = M.getOrInsertFunction("_exit", FunctionType::get(voidTy, {i32}, false));

  // int __shield_tamper_check(void): 1 if the section sum != expected.
  Function *check = Function::Create(FunctionType::get(i32, {}, false),
                                     GlobalValue::InternalLinkage, "__shield_tamper_check", &M);
  BasicBlock *entry = BasicBlock::Create(ctx, "entry", check);
  BasicBlock *loop = BasicBlock::Create(ctx, "loop", check);
  BasicBlock *body = BasicBlock::Create(ctx, "body", check);
  BasicBlock *done = BasicBlock::Create(ctx, "done", check);

  IRBuilder<> B(entry);
  B.CreateBr(loop);

  B.SetInsertPoint(loop);
  PHINode *p = B.CreatePHI(ptr, 2, "p");
  PHINode *s = B.CreatePHI(i64, 2, "s");
  p->addIncoming(gStart, entry);
  s->addIncoming(ConstantInt::get(i64, 0), entry);
  B.CreateCondBr(B.CreateICmpULT(p, gStop), body, done);

  B.SetInsertPoint(body);
  Value *bt = B.CreateZExt(B.CreateLoad(i8, p), i64);
  Value *sn = B.CreateAdd(s, bt);
  Value *pn = B.CreateInBoundsGEP(i8, p, {ConstantInt::get(i64, 1)});
  s->addIncoming(sn, body);
  p->addIncoming(pn, body);
  B.CreateBr(loop);

  B.SetInsertPoint(done);
  LoadInst *exp = B.CreateLoad(i64, gExpected, "exp");
  exp->setVolatile(true); // don't fold the sentinel into the compare
  Value *tampered = B.CreateZExt(B.CreateICmpNE(s, exp), i32);
  B.CreateRet(tampered);

  // constructor: if (check()) _exit(67);
  Function *ctor = Function::Create(FunctionType::get(voidTy, {}, false),
                                    GlobalValue::InternalLinkage, "__shield_tamper_ctor", &M);
  BasicBlock *cEntry = BasicBlock::Create(ctx, "entry", ctor);
  BasicBlock *cDet = BasicBlock::Create(ctx, "detected", ctor);
  BasicBlock *cClean = BasicBlock::Create(ctx, "clean", ctor);
  IRBuilder<> C(cEntry);
  C.CreateCondBr(C.CreateICmpNE(C.CreateCall(check, {}), ConstantInt::get(i32, 0)), cDet, cClean);
  C.SetInsertPoint(cDet);
  C.CreateCall(fExit, {ConstantInt::get(i32, kTamperExit)});
  C.CreateUnreachable();
  C.SetInsertPoint(cClean);
  C.CreateRetVoid();

  appendToGlobalCtors(M, ctor, /*Priority=*/0);
  return true;
}
