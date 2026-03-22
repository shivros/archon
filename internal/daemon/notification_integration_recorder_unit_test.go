package daemon

import (
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

func TestProviderNotificationMatchPolicyMatchesNormalizedFields(t *testing.T) {
	t.Parallel()
	policy := newProviderNotificationMatchPolicy()
	event := types.NotificationEvent{
		Trigger:   types.NotificationTrigger("turn_completed"),
		SessionID: "sess-1",
		Provider:  "CoDeX",
		TurnID:    "turn-1",
	}
	target := NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		Provider:  "codex",
		TurnID:    "turn-1",
	}
	if !policy.Matches(event, target) {
		t.Fatalf("expected normalized event to match target")
	}
}

func TestProviderNotificationMatchPolicyRejectsMismatchedTurnID(t *testing.T) {
	t.Parallel()
	policy := newProviderNotificationMatchPolicy()
	event := types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		Provider:  "codex",
		TurnID:    "turn-a",
	}
	target := NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		Provider:  "codex",
		TurnID:    "turn-b",
	}
	if policy.Matches(event, target) {
		t.Fatalf("expected mismatch on turn id")
	}
}

func TestCapturingNotificationRecorderWaitForMatchImmediate(t *testing.T) {
	t.Parallel()
	recorder := newCapturingNotificationRecorder()
	event := types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-immediate",
		Provider:  "codex",
		TurnID:    "turn-1",
	}
	recorder.Publish(event)
	got, ok := recorder.WaitForMatch(NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-immediate",
		Provider:  "codex",
		TurnID:    "turn-1",
	}, nil, 100*time.Millisecond)
	if !ok {
		t.Fatalf("expected immediate match")
	}
	if got.TurnID != "turn-1" {
		t.Fatalf("unexpected turn id: %q", got.TurnID)
	}
}

func TestCapturingNotificationRecorderWaitForMatchAsync(t *testing.T) {
	t.Parallel()
	recorder := newCapturingNotificationRecorder()
	target := NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-async",
		Provider:  "opencode",
		TurnID:    "turn-async",
	}
	go func() {
		time.Sleep(10 * time.Millisecond)
		recorder.Publish(types.NotificationEvent{
			Trigger:   types.NotificationTriggerTurnCompleted,
			SessionID: "sess-async",
			Provider:  "opencode",
			TurnID:    "turn-async",
		})
	}()
	_, ok := recorder.WaitForMatch(target, newProviderNotificationMatchPolicy(), 500*time.Millisecond)
	if !ok {
		t.Fatalf("expected async match")
	}
}

func TestCapturingNotificationRecorderWaitForMatchTimeout(t *testing.T) {
	t.Parallel()
	recorder := newCapturingNotificationRecorder()
	_, ok := recorder.WaitForMatch(NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-timeout",
		Provider:  "claude",
		TurnID:    "turn-timeout",
	}, nil, 40*time.Millisecond)
	if ok {
		t.Fatalf("expected timeout without matching event")
	}
}

func TestCapturingNotificationRecorderSnapshotDeepCopy(t *testing.T) {
	t.Parallel()
	recorder := newCapturingNotificationRecorder()
	recorder.Publish(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-copy",
		Provider:  "codex",
		TurnID:    "turn-copy",
		Payload: map[string]any{
			"turn_status": "completed",
			"nested": map[string]any{
				"key": "value",
			},
		},
	})

	first := recorder.Snapshot()
	if len(first) != 1 {
		t.Fatalf("expected one snapshot event, got %d", len(first))
	}
	first[0].Payload["turn_status"] = "mutated"
	if nested, ok := first[0].Payload["nested"].(map[string]any); ok {
		nested["key"] = "changed"
	}

	second := recorder.Snapshot()
	if got := notificationTurnStatus(second[0]); got != "completed" {
		t.Fatalf("expected payload clone to remain unchanged, got %q", got)
	}
	nested, ok := second[0].Payload["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested payload map")
	}
	if nested["key"] != "value" {
		t.Fatalf("expected nested payload clone unchanged, got %#v", nested["key"])
	}
}

func TestNotificationEventsDebugStringIncludesFields(t *testing.T) {
	t.Parallel()
	events := []types.NotificationEvent{
		{
			Trigger:   types.NotificationTriggerTurnCompleted,
			SessionID: "sess-debug",
			Provider:  "codex",
			TurnID:    "turn-debug",
			Payload:   map[string]any{"turn_status": "completed"},
			Source:    "source-test",
		},
	}
	got := notificationEventsDebugString(events)
	if got == "" || got == "<none>" {
		t.Fatalf("expected non-empty debug string, got %q", got)
	}
	if want := "sess-debug"; !strings.Contains(got, want) {
		t.Fatalf("expected debug string to contain %q, got %q", want, got)
	}
}
