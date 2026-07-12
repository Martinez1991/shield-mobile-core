// Native RASP (anti-debug) injection (issue #82/#84, ADR 0004).
//
// We synthesize a runtime check equivalent to:
//
//   static int __shield_rasp_check(void) {
//     char buf[4096];
//     int fd = open("/proc/self/status", O_RDONLY);
//     if (fd < 0) return 0;                       // can't tell -> don't react
//     long n = read(fd, buf, 4095);
//     close(fd);
//     buf[n < 0 ? 0 : n] = 0;
//     char *p = strstr(buf, "TracerPid:");
//     if (!p) return 0;
//     return atoi(p + 10) != 0;                   // nonzero TracerPid => debugger
//   }
//   __attribute__((constructor)) static void ctor(void){ if (check()) _exit(66); }
//
// Leaning on libc (open/read/close/strstr/atoi) keeps the emitted IR small. The
// check runs before main; a clean process is unaffected (functionally identical),
// a traced one exits with 66. Its globals are named __shield_* so the strings
// pass skips them (their order-of-decryption would otherwise disable the check).
#include "Rasp.h"

#include "llvm/IR/Constants.h"
#include "llvm/IR/DerivedTypes.h"
#include "llvm/IR/Function.h"
#include "llvm/IR/GlobalVariable.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/Transforms/Utils/ModuleUtils.h"

using namespace llvm;

// The exit code a detected debugger triggers (distinctive, for the gate).
static const int kRaspExit = 66;

static GlobalVariable *cstr(Module &M, StringRef name, StringRef text) {
  Constant *init = ConstantDataArray::getString(M.getContext(), text, /*AddNull=*/true);
  return new GlobalVariable(M, init->getType(), /*isConstant=*/true,
                            GlobalValue::PrivateLinkage, init, name);
}

bool raspModule(Module &M, uint64_t seed) {
  (void)seed;
  // Idempotent: don't inject twice.
  if (M.getFunction("__shield_rasp_check"))
    return false;

  LLVMContext &ctx = M.getContext();
  IntegerType *i32 = Type::getInt32Ty(ctx);
  IntegerType *i64 = Type::getInt64Ty(ctx);
  IntegerType *i8 = Type::getInt8Ty(ctx);
  PointerType *ptr = PointerType::get(ctx, 0);
  Type *voidTy = Type::getVoidTy(ctx);

  // libc declarations.
  FunctionCallee fOpen = M.getOrInsertFunction("open", FunctionType::get(i32, {ptr, i32}, /*vararg=*/true));
  FunctionCallee fRead = M.getOrInsertFunction("read", FunctionType::get(i64, {i32, ptr, i64}, false));
  FunctionCallee fClose = M.getOrInsertFunction("close", FunctionType::get(i32, {i32}, false));
  FunctionCallee fStrstr = M.getOrInsertFunction("strstr", FunctionType::get(ptr, {ptr, ptr}, false));
  FunctionCallee fAtoi = M.getOrInsertFunction("atoi", FunctionType::get(i32, {ptr}, false));
  FunctionCallee fExit = M.getOrInsertFunction("_exit", FunctionType::get(voidTy, {i32}, false));

  GlobalVariable *gPath = cstr(M, "__shield_rasp_path", "/proc/self/status");
  GlobalVariable *gNeedle = cstr(M, "__shield_rasp_needle", "TracerPid:");

  // int __shield_rasp_check(void)
  Function *check = Function::Create(FunctionType::get(i32, {}, false),
                                     GlobalValue::InternalLinkage, "__shield_rasp_check", &M);
  BasicBlock *entry = BasicBlock::Create(ctx, "entry", check);
  BasicBlock *ret0 = BasicBlock::Create(ctx, "ret0", check);
  BasicBlock *fdok = BasicBlock::Create(ctx, "fdok", check);
  BasicBlock *found = BasicBlock::Create(ctx, "found", check);

  IRBuilder<> B(entry);
  ArrayType *bufTy = ArrayType::get(i8, 4096);
  AllocaInst *buf = B.CreateAlloca(bufTy, nullptr, "buf");
  Value *fd = B.CreateCall(fOpen, {gPath, ConstantInt::get(i32, 0)}); // O_RDONLY
  B.CreateCondBr(B.CreateICmpSLT(fd, ConstantInt::get(i32, 0)), ret0, fdok);

  B.SetInsertPoint(ret0);
  B.CreateRet(ConstantInt::get(i32, 0));

  B.SetInsertPoint(fdok);
  Value *n = B.CreateCall(fRead, {fd, buf, ConstantInt::get(i64, 4095)});
  B.CreateCall(fClose, {fd});
  Value *ncl = B.CreateSelect(B.CreateICmpSLT(n, ConstantInt::get(i64, 0)),
                              ConstantInt::get(i64, 0), n);
  Value *endp = B.CreateInBoundsGEP(bufTy, buf, {ConstantInt::get(i64, 0), ncl});
  B.CreateStore(ConstantInt::get(i8, 0), endp);
  Value *p = B.CreateCall(fStrstr, {buf, gNeedle});
  Value *isNull = B.CreateICmpEQ(p, ConstantPointerNull::get(ptr));
  B.CreateCondBr(isNull, ret0, found);

  B.SetInsertPoint(found);
  Value *q = B.CreateInBoundsGEP(i8, p, {ConstantInt::get(i64, 10)}); // past "TracerPid:"
  Value *tracer = B.CreateCall(fAtoi, {q});
  Value *det = B.CreateZExt(B.CreateICmpNE(tracer, ConstantInt::get(i32, 0)), i32);
  B.CreateRet(det);

  // constructor: if (check()) _exit(66);
  Function *ctor = Function::Create(FunctionType::get(voidTy, {}, false),
                                    GlobalValue::InternalLinkage, "__shield_rasp_ctor", &M);
  BasicBlock *cEntry = BasicBlock::Create(ctx, "entry", ctor);
  BasicBlock *cDet = BasicBlock::Create(ctx, "detected", ctor);
  BasicBlock *cClean = BasicBlock::Create(ctx, "clean", ctor);
  IRBuilder<> C(cEntry);
  Value *d = C.CreateCall(check, {});
  C.CreateCondBr(C.CreateICmpNE(d, ConstantInt::get(i32, 0)), cDet, cClean);
  C.SetInsertPoint(cDet);
  C.CreateCall(fExit, {ConstantInt::get(i32, kRaspExit)});
  C.CreateUnreachable();
  C.SetInsertPoint(cClean);
  C.CreateRetVoid();

  appendToGlobalCtors(M, ctor, /*Priority=*/0);
  return true;
}
