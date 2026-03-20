package guidedworkflows

import (
	"context"
	"strings"
)

// GateDispatcher abstracts transport dispatch for async gate kinds.
type GateDispatcher interface {
	DispatchGate(ctx context.Context, req GateDispatchRequest) (GateDispatchResult, error)
}

// GateSignalMatcher determines whether an incoming gate signal applies to an active gate run.
type GateSignalMatcher interface {
	Matches(gate *WorkflowGateRun, signal GateSignal) bool
}

// GateSignalAdapter normalizes external events into gate-native signals.
type GateSignalAdapter interface {
	FromTurnSignal(signal TurnSignal) GateSignal
}

type strictGateSignalMatcher struct{}

func (strictGateSignalMatcher) Matches(gate *WorkflowGateRun, signal GateSignal) bool {
	if gate == nil {
		return false
	}
	expectedSession := ""
	expectedSignal := strings.TrimSpace(gate.SignalID)
	if gate.Execution != nil {
		if expectedSignal == "" {
			expectedSignal = strings.TrimSpace(gate.Execution.SignalID)
		}
		expectedSession = strings.TrimSpace(gate.Execution.SessionID)
	}
	actualSignal := strings.TrimSpace(signal.SignalID)
	actualSession := strings.TrimSpace(signal.SessionID)
	if expectedSignal != "" && expectedSignal != actualSignal {
		return false
	}
	if expectedSession != "" && actualSession != "" && !strings.EqualFold(expectedSession, actualSession) {
		return false
	}
	return true
}

func NewGateSignalMatcher() GateSignalMatcher {
	return strictGateSignalMatcher{}
}

type turnSignalGateSignalAdapter struct{}

func (turnSignalGateSignalAdapter) FromTurnSignal(signal TurnSignal) GateSignal {
	return GateSignal{
		Transport:   "session_turn",
		SignalID:    strings.TrimSpace(signal.TurnID),
		SessionID:   strings.TrimSpace(signal.SessionID),
		WorkspaceID: strings.TrimSpace(signal.WorkspaceID),
		WorktreeID:  strings.TrimSpace(signal.WorktreeID),
		Provider:    strings.TrimSpace(signal.Provider),
		Source:      strings.TrimSpace(signal.Source),
		Status:      strings.TrimSpace(signal.Status),
		Error:       strings.TrimSpace(signal.Error),
		Output:      strings.TrimSpace(signal.Output),
		Terminal:    signal.Terminal,
		Payload:     cloneStringAnyMap(signal.Payload),
	}
}

func NewGateSignalAdapter() GateSignalAdapter {
	return turnSignalGateSignalAdapter{}
}
