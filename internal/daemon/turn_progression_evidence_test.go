package daemon

import (
	"testing"

	"control/internal/types"
)

func TestTurnProgressionEvidenceFromNotificationCompletedFallback(t *testing.T) {
	evidence := turnProgressionEvidenceFromNotification(types.NotificationEvent{
		Trigger: types.NotificationTriggerTurnCompleted,
	})
	if !evidence.Terminal {
		t.Fatalf("expected terminal fallback for turn-completed trigger")
	}
	if evidence.Status != "completed" {
		t.Fatalf("expected fallback completed status, got %q", evidence.Status)
	}
}

func TestTurnProgressionEvidenceFromNotificationReadsFreshnessAndKey(t *testing.T) {
	evidence := turnProgressionEvidenceFromNotification(types.NotificationEvent{
		Trigger: types.NotificationTriggerTurnCompleted,
		Payload: map[string]any{
			"turn_status":            "completed",
			"turn_output":            "done",
			"turn_output_fresh":      true,
			"assistant_evidence_key": "id:msg-1",
		},
	})
	if !evidence.FreshOutput {
		t.Fatalf("expected fresh output signal")
	}
	if evidence.EvidenceKey != "id:msg-1" {
		t.Fatalf("unexpected evidence key: %q", evidence.EvidenceKey)
	}
	if evidence.Output != "done" {
		t.Fatalf("unexpected output: %q", evidence.Output)
	}
}
