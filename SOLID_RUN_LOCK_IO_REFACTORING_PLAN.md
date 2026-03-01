# SOLID Refactoring Plan: Release Run Lock During External Calls

## Overview

This plan fixes the issue where `RenameRun` and other operations timeout because the per-run lock is held during slow external calls (LLM provider dispatch). The fix ensures locks are only held during actual state mutation, not during I/O operations.

---

## Problem Analysis

### Current Issue

```
RenameRun ──lock──> Acquire run lock (10s timeout starts)
                    │
                    ▼
              Check/run update
                    │
                    ▼
              persistRunSnapshotAsync() ──> Background goroutine (slow)
                    │
                    ▼
              Return (but lock still held!)
```

**Actual flow causing timeout:**
```
RenameRun called
   │
   ▼
s.runLocks.Lock(runID)  ──acquired──
   │
   ▼
... update state ...
   │
   ▼
persistRunSnapshotAsync()  ──returns immediately but lock still held
   │
   ▼
return  ──lock released──
```

The lock IS released on return, but if another operation (like `StartRun`, `ResumeRun`, or `HandleDecision` with approve) is already holding the lock for an LLM call, `RenameRun` blocks waiting for it.

### Root Cause

The per-run lock (`s.runLocks.Lock()`) is held during the **entire operation** including:
- `engine.Advance()` which calls `s.engine.DispatchStepPrompt()`
- All dispatch/persistence I/O

Even though `advanceWithEngineUnlocked` releases `s.mu` during dispatch, the **run lock** is still held.

### Affected Operations

| Operation | Holds Run Lock During |
|-----------|---------------------|
| `RenameRun` | Snapshot persistence (minor) |
| `DismissRun` | Snapshot persistence (minor) |
| `UndismissRun` | Snapshot persistence (minor) |
| `PauseRun` | Snapshot persistence (minor) |
| `StopRun` | Likely short |
| `StartRun` | Dispatch + engine.Advance (CRITICAL) |
| `ResumeRun` | Dispatch + engine.Advance (CRITICAL) |
| `HandleDecision` (approve) | Dispatch + engine.Advance (CRITICAL) |
| `AdvanceRun` | Dispatch + engine.Advance (CRITICAL) |

### SOLID Violation

**SRP Violation**: The run lock guards both:
1. In-memory state mutations (legitimate)
2. Slow external I/O (should not block other operations)

---

## Phase 1: Extract Lock-Releasing Dispatch Pattern (SRP)

**Goal**: Create a helper that releases the run lock during dispatch and re-acquires it for state updates.

### 1.1 Define DispatchResult Type

**File:** `internal/guidedworkflows/service.go`

Add near existing type definitions:

```go
type dispatchOutcome int

const (
	dispatchOutcomeNone    dispatchOutcome = iota
	dispatchOutcomeSuccess
	dispatchOutcomeDeferred
	dispatchOutcomeFailed
)

type dispatchResult struct {
	outcome dispatchOutcome
	err     error
	run     *WorkflowRun
}
```

### 1.2 Create runLockGuardedDispatch Helper

**File:** `internal/guidedworkflows/service.go`

```go
func (s *InMemoryRunService) runLockGuardedDispatch(
	ctx context.Context,
	runID string,
	dispatchFn func(context.Context) (*StepPromptDispatchResponse, error),
) (*WorkflowRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("%w: run id is required", ErrInvalidTransition)
	}

	releaseRunLock := s.runLocks.Lock(runID)

	s.mu.Lock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		s.mu.Unlock()
		releaseRunLock()
		return nil, err
	}

	s.mu.Unlock()
	releaseRunLock()

	result, err := dispatchFn(ctx)

	releaseRunLock = s.runLocks.Lock(runID)
	defer releaseRunLock()

	s.mu.Lock()
	defer s.mu.Unlock()

	run, err = s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}

	return cloneWorkflowRun(run), err
}
```

### 1.3 Refactor RenameRun to Release Lock During Persistence

**File:** `internal/guidedworkflows/service.go`

Current (problematic):
```go
func (s *InMemoryRunService) RenameRun(ctx context.Context, runID, name string) (*WorkflowRun, error) {
	releaseRunLock := s.runLocks.Lock(runID)
	defer releaseRunLock()

	s.mu.Lock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	// ... update state ...
	s.mu.Unlock()
	s.persistRunSnapshotAsync(ctx, snapshot)  // Lock still held!
	return cloneWorkflowRun(run), nil
}
```

Refactored:
```go
func (s *InMemoryRunService) RenameRun(ctx context.Context, runID, name string) (*WorkflowRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("%w: run id is required", ErrInvalidTransition)
	}
	normalizedName := strings.TrimSpace(name)
	if normalizedName == "" {
		return nil, fmt.Errorf("%w: run name is required", ErrInvalidTransition)
	}

	releaseRunLock := s.runLocks.Lock(runID)

	s.mu.Lock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		s.mu.Unlock()
		releaseRunLock()
		return nil, err
	}
	if strings.TrimSpace(run.TemplateName) == normalizedName {
		clone := cloneWorkflowRun(run)
		s.mu.Unlock()
		releaseRunLock()
		return clone, nil
	}
	now := s.engine.now()
	run.TemplateName = normalizedName
	appendRunAudit(run, RunAuditEntry{...})
	s.appendTimelineEventLocked(run.ID, RunTimelineEvent{...})
	snapshot := s.captureRunSnapshot(run.ID)
	s.mu.Unlock()

	releaseRunLock()

	s.persistRunSnapshotAsync(ctx, snapshot)

	releaseRunLock = s.runLocks.Lock(runID)
	clone := cloneWorkflowRun(run)
	releaseRunLock()

	return clone, nil
}
```

**Effort:** 1 hour

---

## Phase 2: Refactor StartRun/ResumeRun to Release Lock During Dispatch (SRP)

**Goal**: These are the critical paths that hold the lock during LLM calls.

### 2.1 Create advanceWithRunLockRelease

**File:** `internal/guidedworkflows/service.go`

```go
func (s *InMemoryRunService) advanceWithRunLockRelease(
	ctx context.Context,
	runID string,
	beforeFn func(*WorkflowRun) error,
	dispatchFn func(context.Context) (*StepPromptDispatchResponse, error),
	afterFn func(*WorkflowRun, *StepPromptDispatchResponse, error) error,
) (*WorkflowRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("%w: run id is required", ErrInvalidTransition)
	}

	releaseRunLock := s.runLocks.Lock(runID)

	s.mu.Lock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		s.mu.Unlock()
		releaseRunLock()
		return nil, err
	}

	if err := beforeFn(run); err != nil {
		s.mu.Unlock()
		releaseRunLock()
		return nil, err
	}

	snapshot := s.captureRunSnapshot(run.ID)
	s.mu.Unlock()

	releaseRunLock()

	result, err := dispatchFn(ctx)

	releaseRunLock = s.runLocks.Lock(runID)
	defer releaseRunLock()

	s.mu.Lock()
	defer s.mu.Unlock()

	run, err = s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}

	if err := afterFn(run, result, err); err != nil {
		return nil, err
	}

	snapshot = s.captureRunSnapshot(run.ID)
	s.persistRunSnapshotAsync(ctx, snapshot)

	return cloneWorkflowRun(run), nil
}
```

### 2.2 Refactor AdvanceRun

**File:** `internal/guidedworkflows/service.go`

```go
func (s *InMemoryRunService) AdvanceRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	return s.advanceWithRunLockRelease(
		ctx,
		runID,
		func(run *WorkflowRun) error {
			if run.Status != WorkflowRunStatusRunning {
				return invalidTransitionError("advance", run.Status)
			}
			return nil
		},
		func(ctx context.Context) (*StepPromptDispatchResponse, error) {
			return nil, s.advanceOnceWithDispatchNoLock(ctx, runID)
		},
		func(run *WorkflowRun, _ *StepPromptDispatchResponse, _ error) error {
			return nil
		},
	)
}
```

### 2.3 Create advanceOnceWithDispatchNoLock

Extract the core advancement logic into a method that doesn't acquire locks:

```go
func (s *InMemoryRunService) advanceOnceWithDispatchNoLock(ctx context.Context, runID string) error {
	s.mu.Lock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		s.mu.Unlock()
		return err
	}

	if isRunAwaitingTurn(run) {
		s.mu.Unlock()
		return nil
	}

	beforeStatus := run.Status
	if paused := s.applyPolicyDecisionLocked(run, defaultPolicyEvaluationInput(run)); paused {
		s.mu.Unlock()
		return nil
	}

	dispatchCtx, hasDispatch := s.prepareStepDispatchContext(run)
	s.mu.Unlock()

	if hasDispatch {
		result, err := s.dispatchStepPrompt(ctx, dispatchCtx.req)

		s.mu.Lock()
		run, _ = s.getRunByIDLocked(dispatchCtx.runID)
		if run == nil {
			s.mu.Unlock()
			return ErrRunNotFound
		}
		_, applyErr := s.applyStepDispatchResult(ctx, dispatchCtx, result, err)
		s.recordTerminalTransitionLocked(beforeStatus, run.Status)
		s.mu.Unlock()

		if applyErr != nil {
			return applyErr
		}
		return nil
	}

	return s.advanceWithEngineUnlocked(ctx, run, beforeStatus)
}
```

**Effort:** 3 hours

---

## Phase 3: Refactor HandleDecision (SRP)

**Goal**: Release lock during dispatch when approving/continuing.

### 3.1 Refactor HandleDecision

**File:** `internal/guidedworkflows/service.go`

```go
func (s *InMemoryRunService) HandleDecision(ctx context.Context, runID string, req DecisionActionRequest) (*WorkflowRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("%w: run id is required", ErrInvalidTransition)
	}

	releaseRunLock := s.runLocks.Lock(runID)
	defer releaseRunLock()

	s.mu.Lock()
	defer s.persistMetrics(ctx)
	defer s.mu.Unlock()

	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}

	action, ok := normalizeDecisionAction(req.Action)
	if !ok {
		return nil, fmt.Errorf("%w: unknown decision action %q", ErrInvalidTransition, strings.TrimSpace(string(req.Action)))
	}

	decisionID := strings.TrimSpace(req.DecisionID)
	if decisionID == "" && run.LatestDecision != nil {
		decisionID = strings.TrimSpace(run.LatestDecision.ID)
	}

	decisionRef := s.lookupDecisionLocked(run, decisionID)
	key := decisionActionReceiptKey(run.ID, decisionID, action)
	if _, seen := s.actions[key]; seen {
		return cloneWorkflowRun(run), nil
	}

	switch action {
	case DecisionActionApproveContinue:
		if run.Status != WorkflowRunStatusPaused {
			return nil, invalidTransitionError(string(action), run.Status)
		}

		s.mu.Unlock()
		releaseRunLock()

		err := s.resumeAndAdvanceWithoutPolicyLocked(ctx, run, strings.TrimSpace(req.Note))

		releaseRunLock = s.runLocks.Lock(runID)
		s.mu.Lock()

		run, _ = s.mustRunLocked(runID)
		if err != nil {
			return nil, err
		}
		s.recordApprovalLatencyLocked(decisionRef)
		// ... rest of handling

	case DecisionActionRequestRevision:
		// ... (no dispatch, keep lock)

	case DecisionActionPauseRun:
		// ... (no dispatch, keep lock)

	// ... other cases
	}

	// ...
}
```

**Effort:** 2 hours

---

## Phase 4: Refactor Remaining Operations (SRP)

### 4.1 DismissRun

```go
func (s *InMemoryRunService) DismissRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("%w: run id is required", ErrInvalidTransition)
	}

	releaseRunLock := s.runLocks.Lock(runID)

	s.mu.Lock()
	run, err := s.prepareRunForDismissalLocked(runID, ctx, ctx != nil)
	if err != nil {
		s.mu.Unlock()
		releaseRunLock()
		return nil, err
	}
	if run.DismissedAt != nil {
		clone := cloneWorkflowRun(run)
		s.mu.Unlock()
		releaseRunLock()
		return clone, nil
	}
	now := s.engine.now()
	s.applyDismissedStateLocked(run, now)
	snapshot := s.captureRunSnapshot(run.ID)
	s.mu.Unlock()

	releaseRunLock()

	s.persistRunSnapshotAsync(ctx, snapshot)

	releaseRunLock = s.runLocks.Lock(runID)
	clone := cloneWorkflowRun(run)
	releaseRunLock()

	return clone, nil
}
```

### 4.2 UndismissRun, PauseRun, StopRun

Apply same pattern: release lock before async persistence.

**Effort:** 1 hour

---

## Phase 5: Add Concurrency Safety (SRP)

### 5.1 Detect Stale State After Lock Re-acquisition

When re-acquiring the lock after release, the run state may have changed. Add validation:

```go
func (s *InMemoryRunService) validateRunStateChanged(before *WorkflowRun, after *WorkflowRun) error {
	if before.Status != after.Status {
		return fmt.Errorf("%w: run status changed from %v to %v", ErrInvalidTransition, before.Status, after.Status)
	}
	if before.CurrentPhaseIndex != after.CurrentPhaseIndex || before.CurrentStepIndex != after.CurrentStepIndex {
		return fmt.Errorf("%w: run position changed", ErrInvalidTransition)
	}
	return nil
}
```

### 5.2 Handle Deleted Runs

If run is deleted during dispatch, return appropriate error:

```go
run, ok := s.getRunByIDLocked(runID)
if !ok || run == nil {
	return nil, ErrRunNotFound
}
```

**Effort:** 1 hour

---

## Phase 6: Update Tests (Verification)

### 6.1 Add Concurrency Tests

**File:** `internal/guidedworkflows/service_lock_test.go` (new)

```go
func TestRenameRunDoesNotBlockDuringDispatch(t *testing.T) {
	svc := NewInMemoryRunService(...)
	run := createTestRun(svc, "test-run")

	var dispatchBlocker sync.Mutex
	dispatchBlocker.Lock()
	dispatchCalled := atomic.Bool{}

	originalDispatch := svc.dispatchStepPrompt
	svc.dispatchStepPrompt = func(ctx context.Context, req StepPromptDispatchRequest) (*StepPromptDispatchResponse, error) {
		dispatchCalled.Store(true)
		dispatchBlocker.Lock()
		defer dispatchBlocker.Unlock()
		return &StepPromptDispatchResponse{}, nil
	}
	defer func() { svc.dispatchStepPrompt = originalDispatch }()

	go func() {
		time.Sleep(50 * time.Millisecond)
		dispatchBlocker.Unlock()
	}()

	start := time.Now()
	_, err := svc.RenameRun(context.Background(), run.ID, "New Name")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.True(t, dispatchCalled.Load())
	require.Less(t, elapsed, 5*time.Second, "Rename should not wait for dispatch")
}
```

### 6.2 Add Timeout Tests

Verify operations complete within reasonable time even during concurrent dispatch.

**Effort:** 2 hours

---

## Implementation Order

| Phase | Description | Effort | Dependencies |
|-------|-------------|--------|--------------|
| 1 | Extract lock-releasing pattern for RenameRun | 1h | None |
| 2 | Refactor StartRun/ResumeRun/AdvanceRun | 3h | Phase 1 |
| 3 | Refactor HandleDecision | 2h | Phase 1 |
| 4 | Refactor remaining operations | 1h | Phase 1 |
| 5 | Add concurrency safety checks | 1h | Phase 2 |
| 6 | Add tests | 2h | Phase 2-5 |

**Total Effort:** 10 hours

---

## Testing Strategy

### Unit Tests

1. **Lock Release Verification** - Verify lock is released during dispatch
2. **State Validation** - Verify stale state is detected
3. **Error Handling** - Verify proper errors when run deleted during dispatch

### Integration Tests

1. **Concurrent Rename + Dispatch** - Rename run A while run B is dispatching
2. **Concurrent Decisions** - Multiple decisions on different runs
3. **Timeout Verification** - Operations complete within timeout

### Regression Tests

1. Run all existing service tests
2. Verify workflow execution still works end-to-end

---

## Rollback Plan

1. **Phase 1** - Helper function can be removed, restore direct lock usage
2. **Phase 2-4** - Restore lock acquisition at method start
3. **Phase 5** - Remove validation checks
4. **Phase 6** - Remove new tests

---

## Success Criteria

1. ✅ `RenameRun` completes in <100ms even during concurrent dispatch
2. ✅ All existing tests pass
3. ✅ No regressions in guided workflow behavior
4. ✅ Operations that need dispatch don't block operations that don't
5. ✅ Run lock is only held during state mutation, not I/O
6. ✅ Each refactored method follows SRP (lock management separated from business logic)
