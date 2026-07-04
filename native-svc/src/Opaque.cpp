// Opaque-predicate insertion (issue #82, ADR 0004).
//
// For each basic block we split before its terminator and guard the continuation
// with an always-true predicate: x*(x+1) is even for every integer x, so
// ((x*(x+1)) & 1) == 0 always holds. x is read with a *volatile* load from an
// internal global, which the optimizer cannot constant-fold, keeping the branch
// alive. The "false" edge targets a bogus block of pure junk that also branches
// to the continuation — so both edges converge and the program is functionally
// identical no matter how the predicate evaluates; the value is the added,
// hard-to-analyze control flow.
#include "Opaque.h"

#include "llvm/IR/Constants.h"
#include "llvm/IR/GlobalVariable.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/Instructions.h"
#include "llvm/IR/Module.h"

#include <vector>

using namespace llvm;

static uint32_t nextRand(uint64_t &s) {
  s = s * 6364136223846793005ULL + 1442695040888963407ULL;
  return static_cast<uint32_t>(s >> 33);
}

// getOpaqueGlobal returns (creating once) an internal i32 global used as the
// non-foldable source x. Its initializer varies with the seed.
static GlobalVariable *getOpaqueGlobal(Module *M, uint64_t seed) {
  const char *name = "__shield_opaque";
  if (GlobalVariable *g = M->getGlobalVariable(name, /*AllowInternal=*/true))
    return g;
  IntegerType *i32 = Type::getInt32Ty(M->getContext());
  return new GlobalVariable(*M, i32, /*isConstant=*/false,
                            GlobalValue::InternalLinkage,
                            ConstantInt::get(i32, static_cast<uint32_t>(seed) | 1u),
                            name);
}

bool opaqueFunction(Function &F, uint64_t seed) {
  if (F.isDeclaration())
    return false;

  Module *M = F.getParent();
  LLVMContext &ctx = F.getContext();
  IntegerType *i32 = Type::getInt32Ty(ctx);

  // Snapshot the original blocks so we don't re-process ones we create.
  std::vector<BasicBlock *> targets;
  for (BasicBlock &bb : F) {
    if (bb.isEHPad())
      continue;
    if (isa<InvokeInst>(bb.getTerminator()))
      continue;
    targets.push_back(&bb);
  }
  if (targets.empty())
    return false;

  GlobalVariable *g = getOpaqueGlobal(M, seed);
  uint64_t rng = seed ^ 0xD1B54A32D192ED03ULL;
  bool changed = false;

  for (BasicBlock *bb : targets) {
    Instruction *term = bb->getTerminator();
    // Continuation holds the original terminator; bb keeps everything before it.
    BasicBlock *cont = bb->splitBasicBlock(term, "op.cont");
    Instruction *autoBr = bb->getTerminator(); // the unconditional br split added

    IRBuilder<> B(autoBr);
    LoadInst *x = B.CreateLoad(i32, g, "op.x");
    x->setVolatile(true);
    Value *prod = B.CreateMul(x, B.CreateAdd(x, ConstantInt::get(i32, 1)));
    Value *even = B.CreateAnd(prod, ConstantInt::get(i32, 1));
    Value *pred = B.CreateICmpEQ(even, ConstantInt::get(i32, 0)); // always true

    // Bogus block: pure junk, then rejoin the continuation.
    BasicBlock *bogus = BasicBlock::Create(ctx, "op.bogus", &F, cont);
    IRBuilder<> BG(bogus);
    Value *j = BG.CreateXor(x, ConstantInt::get(i32, nextRand(rng)));
    j = BG.CreateMul(j, ConstantInt::get(i32, nextRand(rng) | 1u));
    (void)j; // dead by construction; no stores/side effects
    BG.CreateBr(cont);

    B.CreateCondBr(pred, cont, bogus);
    autoBr->eraseFromParent();
    changed = true;
  }
  return changed;
}
