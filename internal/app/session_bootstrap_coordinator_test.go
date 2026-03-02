package app

import (
	"testing"

	"control/internal/types"
)

type fixedBootstrapPolicy struct {
	selection sessionBootstrapPlan
	start     sessionBootstrapPlan
	reconnect sessionBootstrapPlan
}

func (p fixedBootstrapPolicy) SelectionLoadPlan(string, types.SessionStatus) sessionBootstrapPlan {
	return p.selection
}

func (p fixedBootstrapPolicy) SessionStartPlan(string, types.SessionStatus) sessionBootstrapPlan {
	return p.start
}

func (p fixedBootstrapPolicy) SendReconnectPlan(string) sessionBootstrapPlan {
	return p.reconnect
}

func TestDefaultSessionBootstrapCoordinatorBuildsExpectedCommandCounts(t *testing.T) {
	coordinator := NewDefaultSessionBootstrapCoordinator(defaultSessionBootstrapPolicy{})

	itemSelection := coordinator.BuildSelectionLoadCommands(SelectionLoadBootstrapInput{
		Provider:     "kilocode",
		Status:       types.SessionStatusInactive,
		SessionID:    "s1",
		SessionKey:   "sess:s1",
		InitialLines: 100,
	})
	if len(itemSelection) != 2 {
		t.Fatalf("expected approvals + items commands, got %d", len(itemSelection))
	}

	codexStart := coordinator.BuildSessionStartCommands(SessionStartBootstrapInput{
		Provider:     "codex",
		Status:       types.SessionStatusRunning,
		SessionID:    "s1",
		SessionKey:   "sess:s1",
		InitialLines: 100,
	})
	if len(codexStart) != 3 {
		t.Fatalf("expected history + approvals + events commands, got %d", len(codexStart))
	}

	reconnect := coordinator.BuildReconnectCommands(SessionReconnectBootstrapInput{
		Provider:             "codex",
		SessionID:            "s1",
		ItemStreamConnected:  false,
		EventStreamConnected: false,
	})
	if len(reconnect) != 1 {
		t.Fatalf("expected events reconnect command, got %d", len(reconnect))
	}
}

func TestDefaultSessionBootstrapCoordinatorReconnectSkipsConnectedStreams(t *testing.T) {
	coordinator := NewDefaultSessionBootstrapCoordinator(defaultSessionBootstrapPolicy{})

	itemReconnect := coordinator.BuildReconnectCommands(SessionReconnectBootstrapInput{
		Provider:             "claude",
		SessionID:            "s1",
		ItemStreamConnected:  true,
		EventStreamConnected: false,
	})
	if len(itemReconnect) != 0 {
		t.Fatalf("expected no reconnect commands when item stream is connected, got %d", len(itemReconnect))
	}

	codexReconnect := coordinator.BuildReconnectCommands(SessionReconnectBootstrapInput{
		Provider:             "codex",
		SessionID:            "s1",
		ItemStreamConnected:  false,
		EventStreamConnected: true,
	})
	if len(codexReconnect) != 0 {
		t.Fatalf("expected no reconnect commands when events stream is connected, got %d", len(codexReconnect))
	}
}

func TestWithSessionBootstrapCoordinatorOption(t *testing.T) {
	coordinator := NewDefaultSessionBootstrapCoordinator(defaultSessionBootstrapPolicy{})
	model := NewModel(nil, WithSessionBootstrapCoordinator(coordinator))
	if model.sessionBootstrapCoordinator == nil {
		t.Fatalf("expected bootstrap coordinator to be set")
	}

	model2 := NewModel(nil, WithSessionBootstrapCoordinator(nil))
	if model2.sessionBootstrapCoordinator != nil {
		t.Fatalf("expected nil explicit coordinator to keep default fallback path")
	}
	if model2.sessionBootstrapCoordinatorOrDefault() == nil {
		t.Fatalf("expected default coordinator fallback")
	}
}

func TestSessionBootstrapCoordinatorFallbackUsesModelPolicy(t *testing.T) {
	model := NewModel(nil, WithSessionBootstrapPolicy(fixedBootstrapPolicy{
		selection: sessionBootstrapPlan{OpenTail: true},
		start:     sessionBootstrapPlan{FetchHistory: true},
		reconnect: sessionBootstrapPlan{OpenEvents: true},
	}))

	cmds := model.sessionBootstrapCoordinatorOrDefault().BuildSelectionLoadCommands(SelectionLoadBootstrapInput{
		Provider:     "custom",
		Status:       types.SessionStatusInactive,
		SessionID:    "s1",
		SessionKey:   "sess:s1",
		InitialLines: 10,
	})
	if len(cmds) != 1 {
		t.Fatalf("expected fallback coordinator to honor model policy, got %d cmds", len(cmds))
	}
}
