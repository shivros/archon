package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type stubSelectionTransitionService struct {
	calls  int
	delay  time.Duration
	source selectionChangeSource
	cmd    tea.Cmd
}

func (s *stubSelectionTransitionService) SelectionChanged(_ *Model, delay time.Duration, source selectionChangeSource) tea.Cmd {
	s.calls++
	s.delay = delay
	s.source = source
	return s.cmd
}

func TestOnSelectionChangedDelegatesToSelectionTransitionService(t *testing.T) {
	stub := &stubSelectionTransitionService{}
	m := NewModel(nil, WithSelectionTransitionService(stub))

	cmd := m.onSelectionChangedWithDelayAndSource(25*time.Millisecond, selectionChangeSourceSystem)
	if stub.calls != 1 {
		t.Fatalf("expected transition service call count 1, got %d", stub.calls)
	}
	if stub.delay != 25*time.Millisecond {
		t.Fatalf("expected delay to be propagated, got %s", stub.delay)
	}
	if stub.source != selectionChangeSourceSystem {
		t.Fatalf("expected source %v, got %v", selectionChangeSourceSystem, stub.source)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd from stub transition service")
	}
}

func TestSelectionOriginPolicyDefaults(t *testing.T) {
	policy := DefaultSelectionOriginPolicy()
	if got := policy.HistoryActionForSource(selectionChangeSourceUser); got != SelectionHistoryActionVisit {
		t.Fatalf("expected user source to visit history, got %v", got)
	}
	if got := policy.HistoryActionForSource(selectionChangeSourceSystem); got != SelectionHistoryActionSyncCurrent {
		t.Fatalf("expected system source to sync current history, got %v", got)
	}
	if got := policy.HistoryActionForSource(selectionChangeSourceHistory); got != SelectionHistoryActionSyncCurrent {
		t.Fatalf("expected history source to sync current history, got %v", got)
	}
}

func TestSelectionOriginPolicySupportsFallback(t *testing.T) {
	policy := NewSelectionOriginPolicy(map[selectionChangeSource]SelectionHistoryAction{
		selectionChangeSourceUser: SelectionHistoryActionIgnore,
	}, SelectionHistoryActionVisit)
	if got := policy.HistoryActionForSource(selectionChangeSourceUser); got != SelectionHistoryActionIgnore {
		t.Fatalf("expected user source override, got %v", got)
	}
	if got := policy.HistoryActionForSource(selectionChangeSourceSystem); got != SelectionHistoryActionVisit {
		t.Fatalf("expected fallback action for system source, got %v", got)
	}
}
