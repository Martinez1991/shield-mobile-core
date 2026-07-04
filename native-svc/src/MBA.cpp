// Mixed boolean-arithmetic (MBA) substitution (issue #82, ADR 0004).
//
// Each integer add/sub/and/or/xor is replaced by an equivalent expression that
// mixes boolean and arithmetic operators, so the plain operation disappears from
// the code. The identities below hold in modulo-2^n (wrapping) arithmetic — the
// low n bits are always exact regardless of overflow — so the new instructions
// are created WITHOUT nsw/nuw flags (dropping them is conservative and correct):
//
//   a + b  =  (a ^ b)  + 2*(a & b)
//   a - b  =  (a ^ ~b) + 2*(a & ~b) + 1        // since a - b = a + (~b) + 1
//   a ^ b  =  (a | b)  - (a & b)
//   a & b  =  (a | b)  - (a ^ b)
//   a | b  =  (a & b)  + (a ^ b)
#include "MBA.h"

#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/Instructions.h"

#include <vector>

using namespace llvm;

bool mbaFunction(Function &F, uint64_t seed) {
  (void)seed; // identities are fixed; polymorphism comes from pass composition

  // Collect targets first so we never rewrite the instructions we just created.
  std::vector<BinaryOperator *> targets;
  for (BasicBlock &bb : F) {
    for (Instruction &i : bb) {
      auto *bo = dyn_cast<BinaryOperator>(&i);
      if (!bo || !bo->getType()->isIntegerTy())
        continue;
      if (bo->getType()->getIntegerBitWidth() < 8)
        continue; // skip i1/booleans — the *2 term degenerates
      switch (bo->getOpcode()) {
      case Instruction::Add:
      case Instruction::Sub:
      case Instruction::And:
      case Instruction::Or:
      case Instruction::Xor:
        targets.push_back(bo);
        break;
      default:
        break;
      }
    }
  }
  if (targets.empty())
    return false;

  for (BinaryOperator *bo : targets) {
    IRBuilder<> B(bo);
    Value *a = bo->getOperand(0);
    Value *b = bo->getOperand(1);
    Constant *two = ConstantInt::get(bo->getType(), 2);
    Value *r = nullptr;

    switch (bo->getOpcode()) {
    case Instruction::Add:
      r = B.CreateAdd(B.CreateXor(a, b), B.CreateMul(two, B.CreateAnd(a, b)));
      break;
    case Instruction::Sub: {
      Value *nb = B.CreateNot(b);
      Value *sum = B.CreateAdd(B.CreateXor(a, nb),
                               B.CreateMul(two, B.CreateAnd(a, nb)));
      r = B.CreateAdd(sum, ConstantInt::get(bo->getType(), 1));
      break;
    }
    case Instruction::Xor:
      r = B.CreateSub(B.CreateOr(a, b), B.CreateAnd(a, b));
      break;
    case Instruction::And:
      r = B.CreateSub(B.CreateOr(a, b), B.CreateXor(a, b));
      break;
    case Instruction::Or:
      r = B.CreateAdd(B.CreateAnd(a, b), B.CreateXor(a, b));
      break;
    default:
      continue;
    }
    bo->replaceAllUsesWith(r);
    bo->eraseFromParent();
  }
  return true;
}
