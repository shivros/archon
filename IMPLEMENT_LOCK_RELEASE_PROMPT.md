# Prompt for Implementing Run Lock Release During Dispatch

## Context

We're fixing a concurrency issue in the guided workflow service (`internal/guidedworkflows/service.go`) where the per-run lock is held during slow I/O operations (LLM provider dispatch), causing other operations like `RenameRun` to timeout.

**What was already done:**
- `RenameRun` was modified to release the run lock before async persistence
- Tests pass

**What's still needed:**
The main bottleneck - `StartRun`, `ResumeRun`, `HandleDecision`, and related functions - still hold the run lock during LLM dispatch. This is the critical path that causes timeouts.

---

## The Problem

Looking at `service.go`, the per-run lock (`s.runLocks.Lock()`) is acquired at the start of operations and held until they return. For operations that do dispatch, this includes the entire LLM call which can take 10+ seconds.

Key functions that hold the lock during dispatch:
1. `StartRun` → `transitionAndAdvance` → `advanceViaQueue` → `advanceOnceWithDispatch` / dispatch
2. `ResumeRun` → same path
3. `HandleDecision` (with approve/continue) → `resumeAndAdvanceWithoutPolicyLocked` → `advanceWithEngineUnlocked`
4. `AdvanceRun` → `advanceOnceLocked` → dispatch / `advanceWithEngineUnlocked`
5. `advanceOnceWithDispatch` (called by dispatch queue) - acquires run lock at start

The lock pattern: The service has TWO locks:
- `s.mu` - global service mutex (sometimes released during dispatch)
- `s.runLocks` - per-run lock (ALWAYS held during dispatch)

---

## Approach

The existing code already has a pattern where `s.mu` is released during dispatch (see `advanceOnceLocked` around line 1460). The run lock should follow the same pattern.

**Recommended approach:**

1. **Create a helper function** that manages the lock-release-reacquire pattern:
   - Acquire run lock at start
   - Do quick state validation
   - Release run lock before slow I/O
   - Re-acquire run lock after I/O
   - Do quick state update
   - Return

2. **Focus on `advanceOnceLocked`** (line ~1454) since this is the central dispatch function called by multiple paths. Modify it to release the run lock during dispatch similar to how it already releases `s.mu`.

3. **Key insight from existing code:** The existing `advanceOnceLocked` already releases `s.mu` during dispatch but keeps `runLock` held. We need to also release `runLock`.

4. **Alternative simpler approach:** If that's too complex, consider removing the run lock from the dispatch path entirely and rely on `s.mu` + the state management (since dispatch already releases `s.mu`).

---

## Important Notes

- The codebase has existing patterns for lock management - look at how `s.mu` is handled in `advanceOnceLocked` and `advanceWithEngineUnlocked`
- There are tests in `service_test.go` that verify concurrency behavior
- The `run_lock_manager.go` file shows the per-run lock implementation
- Be careful about the order of lock releases to avoid deadlocks (always release in reverse order of acquisition)

---

## Flexibility

If you encounter issues or find a cleaner approach, you're encouraged to:
- Simplify if possible (e.g., removing unnecessary locking)
- Split into smaller steps if the full change is too risky
- Add more tests if needed
- Skip or defer parts that are too complex

The goal is to make `RenameRun` and similar quick operations NOT block on slow dispatch operations. There may be multiple valid solutions.

---

## Files to Focus On

- `internal/guidedworkflows/service.go` - main service implementation
- `internal/guidedworkflows/run_lock_manager.go` - lock implementation
- `internal/guidedworkflows/service_test.go` - tests (may need updates)
