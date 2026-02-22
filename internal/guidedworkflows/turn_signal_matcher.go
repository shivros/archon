package guidedworkflows

import "strings"

// TurnSignalMatcher determines whether a run should consume a turn-completed signal.
type TurnSignalMatcher interface {
	Matches(run *WorkflowRun, signal TurnSignal) bool
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
