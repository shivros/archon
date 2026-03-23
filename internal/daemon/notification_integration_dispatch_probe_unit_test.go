package daemon

import (
	"context"
	"testing"
	"time"

	"control/internal/types"
)

func TestNotificationDispatchProbeRecordsAndMatches(t *testing.T) {
	t.Parallel()
	probe := newCapturingNotificationDispatchProbe()
	target := NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		Provider:  "codex",
		TurnID:    "turn-1",
	}

	probe.Record(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		Provider:  "codex",
		TurnID:    "turn-1",
		Payload:   map[string]any{"turn_status": "completed"},
	})

	event, ok := probe.WaitForMatch(target, newProviderNotificationMatchPolicy(), time.Second)
	if !ok {
		t.Fatalf("expected dispatch probe match")
	}
	if event.Trigger != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected trigger %q", event.Trigger)
	}
	matched := probe.MatchingEvents(target, newProviderNotificationMatchPolicy())
	if len(matched) != 1 {
		t.Fatalf("expected one matching dispatch, got %d", len(matched))
	}
}

func TestNotificationDispatchProbeSinkWritesToProbe(t *testing.T) {
	t.Parallel()
	probe := newCapturingNotificationDispatchProbe()
	sink := newNotificationDispatchProbeSink(probe)
	if sink.Method() != types.NotificationMethodBell {
		t.Fatalf("unexpected sink method %q", sink.Method())
	}

	err := sink.Notify(context.Background(), types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-2",
		Provider:  "claude",
		TurnID:    "turn-2",
	}, types.NotificationSettings{Enabled: true})
	if err != nil {
		t.Fatalf("sink notify failed: %v", err)
	}

	target := NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-2",
		Provider:  "claude",
		TurnID:    "turn-2",
	}
	if _, ok := probe.WaitForMatch(target, newProviderNotificationMatchPolicy(), time.Second); !ok {
		t.Fatalf("expected sink notification to reach probe")
	}
}
