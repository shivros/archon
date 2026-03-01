package guidedworkflows

import (
	"context"
	"strings"
)

// TurnSignalMatcher determines whether a run should consume a turn-completed signal.
type TurnSignalMatcher interface {
	Matches(run *WorkflowRun, signal TurnSignal) bool
}

// TurnSignalMismatchHandler handles cases where signal doesn't match step expectations.
type TurnSignalMismatchHandler interface {
	HandleMismatch(ctx context.Context, run *WorkflowRun, step *StepRun, signal TurnSignal) TurnSignalMismatchResult
}

// TurnSignalMismatchResult indicates how to proceed after mismatch handling.
type TurnSignalMismatchResult struct {
	Proceed  bool
	Reason   string
	Recovery bool
}

// defaultTurnSignalMismatchHandler logs mismatches but doesn't allow progression by default.
type defaultTurnSignalMismatchHandler struct{}

func (defaultTurnSignalMismatchHandler) HandleMismatch(_ context.Context, _ *WorkflowRun, _ *StepRun, _ TurnSignal) TurnSignalMismatchResult {
	return TurnSignalMismatchResult{
		Proceed: false,
		Reason:  "mismatch_logged_no_recovery",
	}
}

// NewTurnSignalMismatchHandler creates a handler (currently default, can be extended).
func NewTurnSignalMismatchHandler() TurnSignalMismatchHandler {
	return defaultTurnSignalMismatchHandler{}
}

// StrictSessionTurnSignalMatcher only matches by session id.
type StrictSessionTurnSignalMatcher struct{}

func (StrictSessionTurnSignalMatcher) Matches(run *WorkflowRun, signal TurnSignal) bool {
	if run == nil {
		return false
	}
	runSessionID := strings.TrimSpace(run.SessionID)
	signalSessionID := strings.TrimSpace(signal.SessionID)
	if runSessionID == "" || signalSessionID == "" {
		return false
	}
	return runSessionID == signalSessionID
}

// LegacyContextTurnSignalMatcher preserves historical context-based fallback matching.
type LegacyContextTurnSignalMatcher struct{}

func (LegacyContextTurnSignalMatcher) Matches(run *WorkflowRun, signal TurnSignal) bool {
	return runMatchesTurnSignal(run, signal)
}

// recoveryTurnSignalMismatchHandler allows session-only matching as recovery when TurnID mismatches.
type recoveryTurnSignalMismatchHandler struct {
	enabled bool
}

func (h *recoveryTurnSignalMismatchHandler) HandleMismatch(
	_ context.Context,
	run *WorkflowRun,
	_ *StepRun,
	signal TurnSignal,
) TurnSignalMismatchResult {
	if !h.enabled {
		return TurnSignalMismatchResult{
			Proceed: false,
			Reason:  "turn_id_mismatch_recovery_disabled",
		}
	}

	runSessionID := strings.TrimSpace(run.SessionID)
	signalSessionID := strings.TrimSpace(signal.SessionID)
	if runSessionID == "" || signalSessionID == "" {
		return TurnSignalMismatchResult{
			Proceed: false,
			Reason:  "session_id_missing",
		}
	}

	if runSessionID != signalSessionID {
		return TurnSignalMismatchResult{
			Proceed: false,
			Reason:  "session_id_mismatch",
		}
	}

	return TurnSignalMismatchResult{
		Proceed:  true,
		Reason:   "turn_id_mismatch_recovered_session_match",
		Recovery: true,
	}
}

// NewRecoveryTurnSignalMismatchHandler creates a handler that allows recovery.
func NewRecoveryTurnSignalMismatchHandler(enabled bool) TurnSignalMismatchHandler {
	return &recoveryTurnSignalMismatchHandler{enabled: enabled}
}
