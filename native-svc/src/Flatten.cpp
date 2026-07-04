// Control-flow flattening pass (issue #82, ADR 0004).
//
// Classic dispatcher-loop flattening (the technique popularized by OLLVM),
// adapted to the LLVM 18 API: every original basic block becomes a case of a
// central switch on a stack-held "state" variable, so the linear CFG is replaced
// by a star topology through one dispatch loop. SSA is restored afterwards by
// demoting cross-block values and PHIs to stack slots.
#include "Flatten.h"

#include "llvm/IR/BasicBlock.h"
#include "llvm/IR/Constants.h"
#include "llvm/IR/Instructions.h"
#include "llvm/IR/Type.h"
#include "llvm/Transforms/Utils/Local.h"

#include <utility>
#include <vector>

using namespace llvm;

// Deterministic SplitMix64-ish step: reproducible state labels from the seed.
static uint32_t nextRand(uint64_t &s) {
  s = s * 6364136223846793005ULL + 1442695040888963407ULL;
  return static_cast<uint32_t>(s >> 33);
}

// fixStack restores valid SSA after flattening: any value used outside its
// defining block, and every PHI, is demoted to a stack slot (loads/stores),
// since the dispatcher destroys the original dominance relationships.
static void fixStack(Function &F) {
  BasicBlock *entry = &F.getEntryBlock();
  std::vector<PHINode *> phis;
  std::vector<Instruction *> regs;
  do {
    phis.clear();
    regs.clear();
    for (BasicBlock &bb : F) {
      for (Instruction &i : bb) {
        if (auto *p = dyn_cast<PHINode>(&i)) {
          phis.push_back(p);
          continue;
        }
        if (i.isUsedOutsideOfBlock(&bb) &&
            !(isa<AllocaInst>(&i) && i.getParent() == entry)) {
          regs.push_back(&i);
        }
      }
    }
    for (Instruction *i : regs)
      DemoteRegToStack(*i);
    for (PHINode *p : phis)
      DemotePHIToStack(p);
  } while (!regs.empty() || !phis.empty());
}

bool flattenFunction(Function &F, uint64_t seed) {
  if (F.isDeclaration())
    return false;

  // Bail on exception handling — flattening EH edges is out of scope.
  for (BasicBlock &bb : F) {
    if (isa<InvokeInst>(bb.getTerminator()))
      return false;
    for (Instruction &i : bb)
      if (i.isEHPad() || isa<LandingPadInst>(&i))
        return false;
  }

  std::vector<BasicBlock *> blocks;
  for (BasicBlock &bb : F)
    blocks.push_back(&bb);
  if (blocks.size() <= 1)
    return false; // nothing to hide

  // The entry block stays out of the switch; the rest become cases.
  blocks.erase(blocks.begin());
  BasicBlock *entry = &F.getEntryBlock();

  // If the entry branches conditionally, split so the branch lives in a case.
  if (entry->getTerminator()->getNumSuccessors() > 1) {
    BasicBlock::iterator it = std::prev(entry->end()); // terminator
    if (entry->size() > 1)
      it = std::prev(it);
    BasicBlock *split = entry->splitBasicBlock(it, "entrySplit");
    blocks.insert(blocks.begin(), split);
  }
  entry->getTerminator()->eraseFromParent();

  LLVMContext &ctx = F.getContext();
  IntegerType *i32 = Type::getInt32Ty(ctx);

  // Assign each block a unique, seed-derived state label.
  uint64_t rng = seed ^ 0x9E3779B97F4A7C15ULL;
  std::vector<uint32_t> used;
  auto uniqueNum = [&]() -> uint32_t {
    for (;;) {
      uint32_t v = nextRand(rng);
      bool dup = false;
      for (uint32_t x : used)
        if (x == v) {
          dup = true;
          break;
        }
      if (!dup) {
        used.push_back(v);
        return v;
      }
    }
  };
  std::vector<std::pair<BasicBlock *, ConstantInt *>> caseMap;
  caseMap.reserve(blocks.size());
  for (BasicBlock *bb : blocks)
    caseMap.push_back({bb, ConstantInt::get(i32, uniqueNum())});
  auto caseOf = [&](BasicBlock *bb) -> ConstantInt * {
    for (auto &pr : caseMap)
      if (pr.first == bb)
        return pr.second;
    return nullptr;
  };

  // State variable, initialized to the first real block's label.
  auto *switchVar = new AllocaInst(i32, 0, "switchVar", entry);
  new StoreInst(caseOf(blocks.front()), switchVar, entry);

  // Dispatch loop.
  BasicBlock *loopEntry = BasicBlock::Create(ctx, "loopEntry", &F, entry);
  BasicBlock *loopEnd = BasicBlock::Create(ctx, "loopEnd", &F, entry);
  auto *load = new LoadInst(i32, switchVar, "sv", loopEntry);

  entry->moveBefore(loopEntry);
  BranchInst::Create(loopEntry, entry);   // entry  -> dispatch
  BranchInst::Create(loopEntry, loopEnd); // loopEnd -> dispatch

  BasicBlock *swDefault = BasicBlock::Create(ctx, "switchDefault", &F, loopEnd);
  BranchInst::Create(loopEnd, swDefault);

  SwitchInst *sw = SwitchInst::Create(load, swDefault, blocks.size(), loopEntry);
  for (BasicBlock *bb : blocks) {
    bb->moveBefore(loopEnd);
    sw->addCase(caseOf(bb), bb);
  }

  // Rewrite each block's terminator to set the next state and jump to loopEnd.
  for (BasicBlock *bb : blocks) {
    Instruction *term = bb->getTerminator();
    unsigned n = term->getNumSuccessors();
    if (n == 0)
      continue; // ret / unreachable — leave as an exit
    if (n == 1) {
      ConstantInt *num = caseOf(term->getSuccessor(0));
      term->eraseFromParent();
      if (num)
        new StoreInst(num, switchVar, bb);
      BranchInst::Create(loopEnd, bb);
    } else if (n == 2) {
      auto *br = dyn_cast<BranchInst>(term);
      if (!br || !br->isConditional())
        continue;
      ConstantInt *t = caseOf(br->getSuccessor(0));
      ConstantInt *f = caseOf(br->getSuccessor(1));
      if (!t || !f)
        continue;
      auto *sel = SelectInst::Create(br->getCondition(), t, f, "sv.next", term);
      term->eraseFromParent();
      new StoreInst(sel, switchVar, bb);
      BranchInst::Create(loopEnd, bb);
    }
    // n > 2 (a switch) is left intact for this MVP.
  }

  fixStack(F);
  return true;
}
