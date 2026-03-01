# SOLID Refactoring Plan: TurnID Mismatch Handling for Guided Workflows

## Overview

This plan addresses the issue where guided workflows stall permanently when a turn completion signal's TurnID doesn't match the expected TurnID stored in the step. Currently, mismatched signals are silently ignored with no logging, metrics, or recovery path.

---

## Problem Analysis

### Current Issue

```
Workflow Step Dispatched
         │
         ▼
   step.TurnID = "turn-123"  (stored in step)
         │
         ▼
   Provider executes turn
         │
         ▼
   Turn completes, but TurnID mismatch
   (session restart, TurnID reset, etc.)
         │
         ▼
   Signal IGNORED silently  ← No logging, no metrics
         │
         ▼
   Step stuck in "awaiting_turn" forever
```

### Code Location

**Signal matching logic** (`internal/guidedworkflows/service.go:2008-2050`):

```go
expectedTurnID := strings.TrimSpace(step.TurnID)
signalTurnID := strings.TrimSpace(signal.TurnID)

// Case 1: Step expects TurnID but signal has none
if expectedTurnID != "" && signalTurnID == "" {
    // SILENTLY IGNORED - no logging
    return false, nil
}

// Case 2: TurnIDs don't match
if expectedTurnID != "" && signalTurnID != "" && signalTurnID != expectedTurnID {
    // SILENTLY IGNORED - no logging
    return false, nil
}
```

### Impact

- Workflow permanently stalls in `awaiting_turn` state
- No user notification that something went wrong
- No way to recover automatically
- Difficult to diagnose (no logs/metrics)

---

## SOLID Violations

### 1. SRP Violation: Single Responsibility

The `InMemoryRunService.completeAwaitingTurnStepLocked()` method handles both:
1. Signal matching and validation
2. Step outcome evaluation and state transitions

The TurnID mismatch handling logic should be separated into its own concern.

### 2. OCP Violation: Open/Closed Principle

The `TurnSignalMatcher` interface (`turn_signal_matcher.go:6-8`) only has a `Matches()` method:

```go
type TurnSignalMatcher interface {
    Matches(run *WorkflowRun, signal TurnSignal) bool
}
```

Adding TurnID mismatch handling requires modifying the matcher implementation, not extending it.

### 3. ISP Violation: Interface Segregation

`TurnSignalMatcher` forces implementations to handle all matching concerns. A consumer that only cares about SessionID matching must also handle TurnID validation.

---

## Proposed Solution

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     TurnSignalMatcher                           │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  StrictSessionTurnSignalMatcher (current)                │  │
│  │  - SessionID matching only                                │  │
│  │  - No TurnID validation                                  │  │
│  └───────────────────────────────────────────────────────────┘  │
│                            │                                    │
│                            ▼                                    │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  TurnSignalMismatchedHandler (NEW)                        │  │
│  │  - Observability: Log/metric TurnID mismatches           │  │
│  │  - Fallback: Allow progression with session-only match   │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

### Design Principles

1. **Observability First**: Always log/metric when TurnID mismatch occurs
2. **Safe Fallback**: Allow session-only matching as recovery (with flag)
3. **Extensible**: New handlers can be composed without modifying core logic

---

## Phase 1: Observability (SRP, ISP)

### Goal

Add logging and metrics when TurnID mismatches occur, without changing behavior.

### 1.1 Create TurnSignalMismatchHandler Interface

**File:** `internal/guidedworkflows/turn_signal_matcher.go`

```go
// TurnSignalMismatchHandler handles cases where signal doesn't match step expectations.
type TurnSignalMismatchHandler interface {
    // HandleMismatch processes a mismatched signal and returns true if workflow should proceed.
    HandleMismatch(ctx context.Context, run *WorkflowRun, step *StepRun, signal TurnSignal) TurnSignalMismatchResult
}

// TurnSignalMismatchResult indicates how to proceed after mismatch handling.
type TurnSignalMismatchResult struct {
    Proceed  bool   // If true, treat as valid completion
    Reason   string // Explanation for the decision
    Recovery bool   // If true, this was a recovery from mismatch
}

// defaultTurnSignalMismatchHandler logs mismatches but doesn't allow progression.
type defaultTurnSignalMismatchHandler struct {
    logger Logger  // Inject for testability
}

// NewTurnSignalMismatchHandler creates a handler with optional logger.
func NewTurnSignalMismatchHandler(logger Logger) TurnSignalMismatchHandler {
    // ...
}
```

### 1.2 Add Metrics for Mismatch Events

**File:** `internal/guidedworkflows/service.go`

Add to `runServiceMetrics`:

```go
type runServiceMetrics struct {
    // ... existing fields
    TurnSignalMismatches int `json:"turn_signal_mismatches"`
    TurnSignalRecovered  int `json:"turn_signal_recovered"`  // Only if fallback enabled
}
```

### 1.3 Update completeAwaitingTurnStepLocked

**File:** `internal/guidedworkflows/service.go`

Replace silent ignore with handler call:

```go
// Before: silent ignore
if expectedTurnID != "" && signalTurnID == "" {
    return false, nil  // Silent ignore
}

// After: handler call
if expectedTurnID != "" && (signalTurnID == "" || signalTurnID != expectedTurnID) {
    result := s.turnMismatchHandler.HandleMismatch(ctx, run, step, signal)
    if result.Proceed {
        s.recordTurnSignalRecoveredLocked()
        // Continue with step completion
    }
    s.recordTurnSignalMismatchLocked()
    return false, nil
}
```

### 1.4 Add Logging

**File:** `internal/guidedworkflows/service.go`

```go
func (s *InMemoryRunService) recordTurnSignalMismatchLocked() {
    if s == nil || !s.telemetryEnabled {
        return
    }
    s.metrics.TurnSignalMismatches++
    // Optional: structured logging if logger available
}
```

---

## Phase 2: Safe Fallback Recovery (OCP)

### Goal

Allow session-only matching as a recovery mechanism when TurnID mismatches occur.

### 2.1 Add Configuration Flag

**File:** `internal/guidedworkflows/orchestrator.go` (Config struct)

```go
type Config struct {
    // ... existing fields
    AllowTurnIDMismatchRecovery bool `json:"allow_turn_id_mismatch_recovery"`
}
```

Default to `false` for safety. Enable via config or environment for recovery scenarios.

### 2.2 Implement Recovery Handler

**File:** `internal/guidedworkflows/turn_signal_matcher.go`

```go
type recoveryTurnSignalMismatchHandler struct {
    enabled bool
    logger  Logger
}

func (h *recoveryTurnSignalMismatchHandler) HandleMismatch(
    ctx context.Context,
    run *WorkflowRun,
    step *StepRun,
    signal TurnSignal,
) TurnSignalMismatchResult {
    if !h.enabled {
        return TurnSignalMismatchResult{
            Proceed: false,
            Reason:  "turn_id_mismatch_recovery_disabled",
        }
    }

    // Validate session still matches
    if strings.TrimSpace(run.SessionID) != strings.TrimSpace(signal.SessionID) {
        return TurnSignalMismatchResult{
            Proceed: false,
            Reason:  "session_id_mismatch",
        }
    }

    // Recovery path: allow progression based on session-only match
    return TurnSignalMismatchResult{
        Proceed:  true,
        Reason:   "turn_id_mismatch_recovered_session_match",
        Recovery: true,
    }
}
```

### 2.3 Wire Handler to Service

**File:** `internal/guidedworkflows/service.go`

```go
type InMemoryRunService struct {
    // ... existing fields
    turnMismatchHandler TurnSignalMismatchHandler
    // ...
}

func NewRunService(cfg Config, opts ...RunServiceOption) *InMemoryRunService {
    // ... existing initialization
    service.turnMismatchHandler = NewTurnSignalMismatchHandler(nil) // Default: no recovery
    // ... apply options
}
```

Add option:

```go
func WithTurnMismatchRecovery(enabled bool) RunServiceOption {
    return func(s *InMemoryRunService) {
        if s == nil {
            return
        }
        s.turnMismatchHandler = &recoveryTurnSignalMismatchHandler{
            enabled: enabled,
            logger:  nil,
        }
    }
}
```

---

## Phase 3: Extensible Matcher Composition (ISP)

### Goal

Allow composing multiple signal matching strategies without modifying core logic.

### 3.1 Create Composable Matcher

**File:** `internal/guidedworkflows/turn_signal_matcher.go`

```go
// CompositeTurnSignalMatcher applies multiple matchers in sequence.
type CompositeTurnSignalMatcher struct {
    matchers    []TurnSignalMatcher
    mismatchHandler TurnSignalMismatchHandler
}

// NewCompositeTurnSignalMatcher creates a matcher with fallback chain.
func NewCompositeTurnSignalMatcher(
    matchers []TurnSignalMatcher,
    mismatchHandler TurnSignalMismatchHandler,
) *CompositeTurnSignalMatcher {
    return &CompositeTurnSignalMatcher{
        matchers:         matchers,
        mismatchHandler: mismatchHandler,
    }
}

func (m *CompositeTurnSignalMatcher) Matches(run *WorkflowRun, signal TurnSignal) bool {
    for _, matcher := range m.matchers {
        if matcher.Matches(run, signal) {
            return true
        }
    }
    // Only reached if no matcher matched - try recovery
    if m.mismatchHandler != nil {
        // This would require access to step state - see note below
    }
    return false
}
```

**Note:** The composite matcher doesn't have access to step state. Instead, the mismatch handling happens in `completeAwaitingTurnStepLocked` where step context is available.

### 3.2 Segregate Matcher Interfaces (ISP)

**File:** `internal/guidedworkflows/turn_signal_matcher.go`

```go
// SessionMatcher handles session-level signal matching.
type SessionMatcher interface {
    MatchesSession(run *WorkflowRun, signal TurnSignal) bool
}

// TurnIDMatcher handles TurnID-level signal matching.
type TurnIDMatcher interface {
    MatchesTurnID(step *StepRun, signal TurnSignal) bool
}

// FullMatcher combines both (existing interface).
type FullMatcher interface {
    Matches(run *WorkflowRun, signal TurnSignal) bool
}
```

---

## Implementation Checklist

| Phase | Task | File | SOLID Principle |
|-------|------|------|-----------------|
| 1.1 | Define `TurnSignalMismatchHandler` interface | `turn_signal_matcher.go` | OCP |
| 1.2 | Add `TurnSignalMismatches` metric | `service.go` | SRP |
| 1.3 | Wire handler to service | `service.go` | SRP |
| 1.4 | Add `recordTurnSignalMismatchLocked()` | `service.go` | SRP |
| 2.1 | Add `AllowTurnIDMismatchRecovery` config | `orchestrator.go` | OCP |
| 2.2 | Implement `recoveryTurnSignalMismatchHandler` | `turn_signal_matcher.go` | OCP |
| 2.3 | Add `WithTurnMismatchRecovery` option | `service.go` | DIP |
| 3.1 | Create `CompositeTurnSignalMatcher` | `turn_signal_matcher.go` | OCP |
| 3.2 | Segregate matcher interfaces | `turn_signal_matcher.go` | ISP |

---

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Recovery could complete wrong step | Default: recovery disabled. Enable only with explicit config. |
| Log spam on mismatch | Add rate limiting to mismatch logging |
| Metric cardinality explosion | Use bounded counter, not per-turnID metrics |

---

## Testing Strategy

1. **Unit Tests**: Test mismatch handler with various TurnID combinations
2. **Integration Tests**: Verify metrics logged when mismatch occurs
3. **Recovery Tests**: Verify workflow progresses when recovery enabled and session matches
4. **Negative Tests**: Verify workflow still fails when recovery disabled
