# SOLID Refactoring Plan: Workflow Service Lock Contention

## Overview

This plan addresses mutex contention in `InMemoryRunService` where a single global lock (`s.mu`) blocks all operations during slow I/O (provider dispatch, persistence). The fix involves extracting responsibilities and using finer-grained locking to allow concurrent operations.

---

## Problem Analysis

### Current State

```go
type InMemoryRunService struct {
    mu sync.Mutex  // Single global lock for ALL operations
    runs map[string]*WorkflowRun
    timelines map[string][]RunTimelineEvent
    // ...
}
```

### Critical Contention Points

| Line | Function | I/O Under Lock | Impact |
|------|----------|----------------|--------|
| 691 | `AdvanceRun()` | `DispatchStepPrompt()` | Critical |
| 709 | `HandleDecision()` | `DispatchStepPrompt()` | Critical |
| 779 | `OnTurnCompleted()` | `DispatchStepPrompt()` | Critical |
| 1199 | `transitionAndAdvance()` | `DispatchStepPrompt()` | Critical |
| 1554 | `retryDeferredDispatch()` | `DispatchStepPrompt()` | Critical |
| 885 | `RenameRun()` | `UpsertWorkflowRun()` | Minor |
| 926 | `DismissRun()` | `UpsertWorkflowRun()` | Minor |
| 1053 | `UndismissRun()` | `UpsertWorkflowRun()` | Minor |

### SOLID Violations

1. **SRP**: `InMemoryRunService` handles state management, dispatch orchestration, persistence, and metrics
2. **OCP**: Adding new async operations requires modifying the lock semantics
3. **DIP**: Service directly owns and manages persistence/dispatch dependencies

---

## Phase 1: Extract RunStateStore (SRP)

**Problem:** Service mixes in-memory state management with dispatch orchestration.

### 1.1 Define RunStateStore Interface

**File:** `internal/guidedworkflows/run_state_store.go` (new)

```go
package guidedworkflows

import "sync"

// RunStateStore manages in-memory workflow run state.
// Implementations must be safe for concurrent access.
type RunStateStore interface {
    Get(runID string) (*WorkflowRun, bool)
    Set(runID string, run *WorkflowRun)
    Delete(runID string)
    List() []*WorkflowRun
    GetTimeline(runID string) []RunTimelineEvent
    AppendTimeline(runID string, events ...RunTimelineEvent)
    SetTimeline(runID string, timeline []RunTimelineEvent)
}

// MemoryRunStateStore is an in-memory implementation of RunStateStore.
type MemoryRunStateStore struct {
    mu        sync.RWMutex
    runs      map[string]*WorkflowRun
    timelines map[string][]RunTimelineEvent
}

func NewMemoryRunStateStore() *MemoryRunStateStore {
    return &MemoryRunStateStore{
        runs:      make(map[string]*WorkflowRun),
        timelines: make(map[string][]RunTimelineEvent),
    }
}

func (s *MemoryRunStateStore) Get(runID string) (*WorkflowRun, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    run, ok := s.runs[runID]
    return run, ok
}

func (s *MemoryRunStateStore) Set(runID string, run *WorkflowRun) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.runs[runID] = run
}

func (s *MemoryRunStateStore) Delete(runID string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    delete(s.runs, runID)
}

func (s *MemoryRunStateStore) List() []*WorkflowRun {
    s.mu.RLock()
    defer s.mu.RUnlock()
    out := make([]*WorkflowRun, 0, len(s.runs))
    for _, run := range s.runs {
        out = append(out, run)
    }
    return out
}

func (s *MemoryRunStateStore) GetTimeline(runID string) []RunTimelineEvent {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return append([]RunTimelineEvent(nil), s.timelines[runID]...)
}

func (s *MemoryRunStateStore) AppendTimeline(runID string, events ...RunTimelineEvent) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.timelines[runID] = append(s.timelines[runID], events...)
}

func (s *MemoryRunStateStore) SetTimeline(runID string, timeline []RunTimelineEvent) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.timelines[runID] = timeline
}
```

### 1.2 Update InMemoryRunService to Use RunStateStore

**File:** `internal/guidedworkflows/service.go`

```go
type InMemoryRunService struct {
    state        RunStateStore     // Delegated state management
    mu           sync.Mutex        // For non-state coordination (actions, turnSeen, etc.)
    actions      map[string]struct{}
    turnSeen     map[string]struct{}
    // ... other fields
}
```

**Effort:** 2 hours

---

## Phase 2: Extract RunPersistenceService (SRP)

**Problem:** Persistence is called while holding service lock.

### 2.1 Define RunPersistenceService Interface

**File:** `internal/guidedworkflows/run_persistence.go` (new)

```go
package guidedworkflows

import "context"

// RunPersistenceService handles async persistence of workflow run snapshots.
type RunPersistenceService interface {
    Persist(ctx context.Context, snapshot RunStatusSnapshot) error
    PersistAsync(ctx context.Context, snapshot RunStatusSnapshot)
}

// DefaultRunPersistenceService persists runs to the run store.
type DefaultRunPersistenceService struct {
    store RunStore
}

func NewRunPersistenceService(store RunStore) *DefaultRunPersistenceService {
    return &DefaultRunPersistenceService{store: store}
}

func (s *DefaultRunPersistenceService) Persist(ctx context.Context, snapshot RunStatusSnapshot) error {
    if s.store == nil {
        return nil
    }
    return s.store.UpsertWorkflowRun(ctx, snapshot)
}

func (s *DefaultRunPersistenceService) PersistAsync(ctx context.Context, snapshot RunStatusSnapshot) {
    go func() {
        _ = s.Persist(ctx, snapshot)
    }()
}
```

### 2.2 Update Service to Use Async Persistence

**File:** `internal/guidedworkflows/service.go`

```go
func (s *InMemoryRunService) RenameRun(ctx context.Context, runID, name string) (*WorkflowRun, error) {
    // Get current state (read lock internally)
    run, ok := s.state.Get(runID)
    if !ok {
        return nil, ErrRunNotFound
    }

    // Update state (write lock internally)
    run.TemplateName = name

    // Persist async - no lock held during I/O
    s.persistence.PersistAsync(ctx, RunStatusSnapshot{
        Run:      cloneWorkflowRun(run),
        Timeline: s.state.GetTimeline(runID),
    })

    return cloneWorkflowRun(run), nil
}
```

**Effort:** 2 hours

---

## Phase 3: Extract DispatchOrchestrator (SRP)

**Problem:** Dispatch I/O blocks all other operations.

### 3.1 Define DispatchOrchestrator Interface

**File:** `internal/guidedworkflows/dispatch_orchestrator.go` (new)

```go
package guidedworkflows

import "context"

// DispatchOrchestrator manages async dispatch with state coordination.
type DispatchOrchestrator interface {
    DispatchNextStep(ctx context.Context, run *WorkflowRun) (*DispatchResult, error)
}

// DispatchResult captures the outcome of a dispatch operation.
type DispatchResult struct {
    Dispatched bool
    Deferred   bool
    Err        error
}

// DefaultDispatchOrchestrator coordinates dispatch without blocking service lock.
type DefaultDispatchOrchestrator struct {
    dispatcher StepDispatcher
    state      RunStateStore
    persistence RunPersistenceService
}

func NewDispatchOrchestrator(dispatcher StepDispatcher, state RunStateStore, persistence RunPersistenceService) *DefaultDispatchOrchestrator {
    return &DefaultDispatchOrchestrator{
        dispatcher:  dispatcher,
        state:       state,
        persistence: persistence,
    }
}

func (o *DefaultDispatchOrchestrator) DispatchNextStep(ctx context.Context, run *WorkflowRun) (*DispatchResult, error) {
    // Find next step to dispatch (read-only)
    phaseIndex, stepIndex, ok := findNextPending(run)
    if !ok {
        return &DispatchResult{}, nil
    }

    phase := &run.Phases[phaseIndex]
    step := &phase.Steps[stepIndex]

    // Prepare dispatch request (no lock needed)
    req := StepPromptDispatchRequest{
        RunID:      run.ID,
        TemplateID: run.TemplateID,
        // ... build request
    }

    // Perform dispatch I/O (no lock held)
    result, err := o.dispatcher.DispatchStepPrompt(ctx, req)
    if err != nil {
        return &DispatchResult{Err: err}, err
    }

    return &DispatchResult{Dispatched: true}, nil
}
```

### 3.2 Refactor advanceOnceLocked to Use Orchestrator

**File:** `internal/guidedworkflows/service.go`

```go
func (s *InMemoryRunService) advanceOnceLocked(ctx context.Context, run *WorkflowRun) error {
    // Check if awaiting turn (fast, no I/O)
    if isRunAwaitingTurn(run) {
        return nil
    }

    // Apply policy (fast, no I/O)
    if paused := s.applyPolicyDecisionLocked(run, defaultPolicyEvaluationInput(run)); paused {
        return nil
    }

    // Capture state needed for dispatch
    runID := run.ID

    // Release lock during dispatch I/O
    s.mu.Unlock()

    // Dispatch (slow I/O)
    result, err := s.dispatchOrchestrator.DispatchNextStep(ctx, run)

    // Re-acquire lock
    s.mu.Lock()

    // Reload run state (may have changed during dispatch)
    run, _ = s.state.Get(runID)

    // Update state based on result
    if err != nil {
        // Handle error
        return err
    }

    // Continue with engine advance...
    return s.engine.Advance(ctx, run, &timeline)
}
```

**Effort:** 4 hours

---

## Phase 4: Introduce Operation-Level Locking (OCP/DIP)

**Problem:** Single mutex forces all operations to serialize.

### 4.1 Define Per-Run Lock Manager

**File:** `internal/guidedworkflows/run_lock_manager.go` (new)

```go
package guidedworkflows

import "sync"

// RunLockManager provides per-run locking for finer granularity.
type RunLockManager interface {
    Lock(runID string) func()
    RLock(runID string) func()
}

// PerRunLockManager manages individual locks per workflow run.
type PerRunLockManager struct {
    mu    sync.Mutex
    locks map[string]*sync.RWMutex
}

func NewPerRunLockManager() *PerRunLockManager {
    return &PerRunLockManager{
        locks: make(map[string]*sync.RWMutex),
    }
}

func (m *PerRunLockManager) getLock(runID string) *sync.RWMutex {
    m.mu.Lock()
    defer m.mu.Unlock()
    if m.locks[runID] == nil {
        m.locks[runID] = &sync.RWMutex{}
    }
    return m.locks[runID]
}

func (m *PerRunLockManager) Lock(runID string) func() {
    lock := m.getLock(runID)
    lock.Lock()
    return func() { lock.Unlock() }
}

func (m *PerRunLockManager) RLock(runID string) func() {
    lock := m.getLock(runID)
    lock.RLock()
    return func() { lock.RUnlock() }
}
```

### 4.2 Update Service to Use Per-Run Locks

**File:** `internal/guidedworkflows/service.go`

```go
func (s *InMemoryRunService) RenameRun(ctx context.Context, runID, name string) (*WorkflowRun, error) {
    // Lock only this specific run
    defer s.lockManager.Lock(runID)()

    run, ok := s.state.Get(runID)
    if !ok {
        return nil, ErrRunNotFound
    }

    run.TemplateName = name
    s.persistence.PersistAsync(ctx, s.snapshot(run))

    return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) OnTurnCompleted(ctx context.Context, signal TurnSignal) ([]*WorkflowRun, error) {
    // Find matching runs (read-only)
    matchingRuns := s.findMatchingRuns(signal)

    var updated []*WorkflowRun
    for _, run := range matchingRuns {
        // Lock each run individually
        defer s.lockManager.Lock(run.ID)()

        // Process turn for this run
        if completed, err := s.completeAwaitingTurnStep(ctx, run, signal); err != nil {
            return nil, err
        } else if completed {
            updated = append(updated, cloneWorkflowRun(run))
        }
    }

    return updated, nil
}
```

**Effort:** 3 hours

---

## Phase 5: Implement Async Dispatch Pattern (DIP)

**Problem:** Dispatch blocks the caller until provider responds.

### 5.1 Define DispatchQueue Interface

**File:** `internal/guidedworkflows/dispatch_queue.go` (new)

```go
package guidedworkflows

import "context"

// DispatchQueue manages async dispatch operations.
type DispatchQueue interface {
    Enqueue(req DispatchRequest) error
    Start(ctx context.Context) error
    Stop()
}

// DispatchRequest represents a queued dispatch operation.
type DispatchRequest struct {
    RunID      string
    PhaseIndex int
    StepIndex  int
    Prompt     string
}

// ChannelDispatchQueue processes dispatch requests asynchronously.
type ChannelDispatchQueue struct {
    requests    chan DispatchRequest
    dispatcher  StepDispatcher
    state       RunStateStore
    persistence RunPersistenceService
    workers     int
}

func NewChannelDispatchQueue(dispatcher StepDispatcher, state RunStateStore, persistence RunPersistenceService, workers int) *ChannelDispatchQueue {
    return &ChannelDispatchQueue{
        requests:    make(chan DispatchRequest, 100),
        dispatcher:  dispatcher,
        state:       state,
        persistence: persistence,
        workers:     workers,
    }
}

func (q *ChannelDispatchQueue) Start(ctx context.Context) error {
    for i := 0; i < q.workers; i++ {
        go q.worker(ctx)
    }
    return nil
}

func (q *ChannelDispatchQueue) worker(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case req := <-q.requests:
            q.process(ctx, req)
        }
    }
}

func (q *ChannelDispatchQueue) process(ctx context.Context, req DispatchRequest) {
    // Load run state
    run, ok := q.state.Get(req.RunID)
    if !ok {
        return
    }

    // Dispatch (slow I/O - but doesn't block service lock)
    _, err := q.dispatcher.DispatchStepPrompt(ctx, StepPromptDispatchRequest{
        RunID: req.RunID,
        Prompt: req.Prompt,
        // ...
    })

    // Update state after dispatch
    if err != nil {
        // Handle error
    }

    // Persist async
    q.persistence.PersistAsync(ctx, q.snapshot(run))
}

func (q *ChannelDispatchQueue) Enqueue(req DispatchRequest) error {
    select {
    case q.requests <- req:
        return nil
    default:
        return ErrStepDispatch
    }
}
```

### 5.2 Update Service to Use Async Dispatch

**File:** `internal/guidedworkflows/service.go`

```go
func (s *InMemoryRunService) StartRun(ctx context.Context, runID string) (*WorkflowRun, error) {
    defer s.lockManager.Lock(runID)()

    run, ok := s.state.Get(runID)
    if !ok {
        return nil, ErrRunNotFound
    }

    // Update state (fast)
    run.Status = WorkflowRunStatusRunning
    now := s.engine.now()
    run.StartedAt = &now

    // Queue dispatch (returns immediately)
    s.dispatchQueue.Enqueue(DispatchRequest{
        RunID: runID,
    })

    // Persist async
    s.persistence.PersistAsync(ctx, s.snapshot(run))

    return cloneWorkflowRun(run), nil
}
```

**Effort:** 4 hours

---

## Phase 6: Refactor Remaining Operations (Cleanup)

### 6.1 Update All Public Methods

| Method | Before | After |
|--------|--------|-------|
| `CreateRun` | Global lock | Global lock (creates new run) |
| `RenameRun` | Global lock | Per-run lock, async persist |
| `DismissRun` | Global lock | Per-run lock, async persist |
| `UndismissRun` | Global lock | Per-run lock, async persist |
| `PauseRun` | Global lock | Per-run lock, async persist |
| `StopRun` | Global lock | Per-run lock, async persist |
| `StartRun` | Global lock + dispatch I/O | Per-run lock, async dispatch |
| `ResumeRun` | Global lock + dispatch I/O | Per-run lock, async dispatch |
| `HandleDecision` | Global lock + dispatch I/O | Per-run lock, async dispatch |
| `OnTurnCompleted` | Global lock + dispatch I/O | Per-run lock, async dispatch |
| `GetRun` | Global RLock | Per-run RLock |
| `GetRunTimeline` | Global RLock | Per-run RLock |

### 6.2 Update Test Coverage

- Add tests for concurrent operations on different runs
- Add tests for async dispatch completion
- Add tests for persistence failures (should not block)

**Effort:** 3 hours

---

## Implementation Order

| Phase | Description | Effort | Dependencies |
|-------|-------------|--------|--------------|
| 1 | Extract RunStateStore (SRP) | 2h | None |
| 2 | Extract RunPersistenceService (SRP) | 2h | Phase 1 |
| 3 | Extract DispatchOrchestrator (SRP) | 4h | Phase 1, 2 |
| 4 | Introduce Per-Run Locking (OCP/DIP) | 3h | Phase 1 |
| 5 | Implement Async Dispatch Queue (DIP) | 4h | Phase 3, 4 |
| 6 | Refactor Remaining Operations | 3h | Phase 4, 5 |

**Total Effort:** 18 hours

---

## Testing Strategy

### Unit Tests

1. **RunStateStore** - Verify concurrent read/write safety
2. **PerRunLockManager** - Verify lock isolation between runs
3. **DispatchQueue** - Verify async processing and error handling
4. **Persistence** - Verify async persist doesn't block

### Integration Tests

1. **Concurrent Rename** - Rename run A while run B is dispatching
2. **Concurrent Decision** - Approve run A while run B is being created
3. **Dispatch Under Load** - Multiple dispatches don't block reads

### Performance Tests

1. **Latency** - Measure p50/p99 latency for RenameRun with concurrent dispatch
2. **Throughput** - Measure operations/second with multiple runs

---

## Rollback Plan

Each phase is independently deployable:

1. **Phase 1-2** - New interfaces can be removed, revert to inline map access
2. **Phase 3** - Orchestrator can be bypassed, dispatch inline
3. **Phase 4** - Per-run locks can be replaced with global lock
4. **Phase 5** - Async queue can be made synchronous

---

## Success Criteria

1. `RenameRun` completes in <100ms even during concurrent dispatch operations
2. All existing tests pass
3. No regressions in guided workflow behavior
4. Provider timeout does not affect unrelated operations
5. Each interface has single responsibility
6. Adding new async operations doesn't require modifying lock semantics
