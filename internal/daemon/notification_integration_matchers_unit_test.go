package daemon

import (
	"testing"
	"time"

	"control/internal/types"
)

type stubNotificationEventProbe struct {
	waitEvent types.NotificationEvent
	waitOK    bool
	matching  []types.NotificationEvent
	snapshot  []types.NotificationEvent
}

func (s stubNotificationEventProbe) WaitForMatch(target NotificationMatchTarget, policy NotificationMatchPolicy, timeout time.Duration) (types.NotificationEvent, bool) {
	if s.waitOK {
		return s.waitEvent, true
	}
	return types.NotificationEvent{}, false
}

func (s stubNotificationEventProbe) MatchingEvents(target NotificationMatchTarget, policy NotificationMatchPolicy) []types.NotificationEvent {
	out := make([]types.NotificationEvent, len(s.matching))
	copy(out, s.matching)
	return out
}

func (s stubNotificationEventProbe) Snapshot() []types.NotificationEvent {
	if len(s.snapshot) > 0 {
		out := make([]types.NotificationEvent, len(s.snapshot))
		copy(out, s.snapshot)
		return out
	}
	out := make([]types.NotificationEvent, len(s.matching))
	copy(out, s.matching)
	return out
}

func TestRequireScenarioEventForPhaseUsesStatusMatchingWhenEnabled(t *testing.T) {
	t.Parallel()
	profile := providerNotificationProfileForTestCase(providerTestCase{name: "opencode"}, nil)
	scenario, ok := providerTerminalNotificationScenarioByName(providerNotificationScenarioFailed)
	if !ok {
		t.Fatalf("expected failed scenario")
	}

	target := NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		Provider:  "opencode",
		TurnID:    "turn-1",
	}
	completed := types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		Provider:  "opencode",
		TurnID:    "turn-1",
		Status:    "completed",
	}
	failed := types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		Provider:  "opencode",
		TurnID:    "turn-1",
		Status:    "failed",
	}
	probe := stubNotificationEventProbe{
		waitEvent: completed,
		waitOK:    true,
		matching:  []types.NotificationEvent{completed, failed},
	}

	event := requireScenarioEventForPhase(
		t,
		probe,
		profile,
		scenario,
		notificationAssertionPhasePublished,
		target,
		newProviderNotificationMatchPolicy(),
		"",
		time.Second,
		"",
	)
	if got := profile.normalizeExpectedTurnStatus(notificationTurnStatus(event)); got != "failed" {
		t.Fatalf("expected status-matched failed event, got %q", got)
	}
}

func TestRequireScenarioEventForPhaseFallsBackToSingleMatch(t *testing.T) {
	t.Parallel()
	profile := providerNotificationProfileForTestCase(providerTestCase{name: "codex"}, nil)
	scenario, ok := providerTerminalNotificationScenarioByName(providerNotificationScenarioCompleted)
	if !ok {
		t.Fatalf("expected completed scenario")
	}

	target := NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-2",
		Provider:  "codex",
		TurnID:    "turn-2",
	}
	eventInProbe := types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-2",
		Provider:  "codex",
		TurnID:    "turn-2",
		Status:    "running",
	}
	probe := stubNotificationEventProbe{
		waitEvent: eventInProbe,
		waitOK:    true,
		matching:  []types.NotificationEvent{eventInProbe},
	}

	event := requireScenarioEventForPhase(
		t,
		probe,
		profile,
		scenario,
		notificationAssertionPhasePublished,
		target,
		newProviderNotificationMatchPolicy(),
		"",
		time.Second,
		"",
	)
	if event.Status != "running" {
		t.Fatalf("expected fallback single-match event, got %#v", event)
	}
}

func TestAssertScenarioStatusContractSkipsWhenPhaseDisabled(t *testing.T) {
	t.Parallel()
	profile := providerNotificationProfileForTestCase(providerTestCase{name: "codex"}, nil)
	scenario, ok := providerTerminalNotificationScenarioByName(providerNotificationScenarioCompleted)
	if !ok {
		t.Fatalf("expected completed scenario")
	}

	// Published status assertions are disabled for codex completed, so this should not fail.
	assertScenarioStatusContract(
		t,
		profile,
		scenario,
		"completed",
		types.NotificationEvent{Status: "running"},
		notificationAssertionPhasePublished,
	)
}

func TestAssertScenarioStatusContractValidatesWhenPhaseEnabled(t *testing.T) {
	t.Parallel()
	profile := providerNotificationProfileForTestCase(providerTestCase{name: "opencode"}, nil)
	scenario, ok := providerTerminalNotificationScenarioByName(providerNotificationScenarioFailed)
	if !ok {
		t.Fatalf("expected failed scenario")
	}

	assertScenarioStatusContract(
		t,
		profile,
		scenario,
		"failed",
		types.NotificationEvent{Status: "failed"},
		notificationAssertionPhaseDispatched,
	)
}
