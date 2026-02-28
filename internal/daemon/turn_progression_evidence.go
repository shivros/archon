package daemon

import (
	"strings"

	"control/internal/types"
)

// TurnProgressionEvidence is the typed, provider-agnostic contract used by
// readiness policies to decide whether a turn can advance guided workflows.
type TurnProgressionEvidence struct {
	Status      string
	Error       string
	Output      string
	Terminal    bool
	Failed      bool
	FreshOutput bool
	EvidenceKey string
}

func turnProgressionEvidenceFromNotification(event types.NotificationEvent) TurnProgressionEvidence {
	status := strings.TrimSpace(notificationPayloadString(event.Payload, "turn_status"))
	errMsg := strings.TrimSpace(notificationPayloadString(event.Payload, "turn_error"))
	if status == "" {
		status = strings.TrimSpace(notificationPayloadString(event.Payload, "status"))
	}
	if errMsg == "" {
		errMsg = strings.TrimSpace(notificationPayloadString(event.Payload, "error"))
	}
	outcome := classifyTurnOutcome(status, errMsg)
	if !outcome.Terminal &&
		outcome.Status == "" &&
		outcome.Error == "" &&
		event.Trigger == types.NotificationTriggerTurnCompleted {
		outcome.Status = "completed"
		outcome.Terminal = true
	}
	output := firstNonEmpty(
		strings.TrimSpace(notificationPayloadString(event.Payload, "turn_output")),
		firstNonEmpty(
			strings.TrimSpace(notificationPayloadString(event.Payload, "output")),
			firstNonEmpty(
				strings.TrimSpace(notificationPayloadString(event.Payload, "assistant_output")),
				strings.TrimSpace(notificationPayloadString(event.Payload, "result")),
			),
		),
	)
	return TurnProgressionEvidence{
		Status:      strings.TrimSpace(outcome.Status),
		Error:       strings.TrimSpace(outcome.Error),
		Output:      strings.TrimSpace(output),
		Terminal:    outcome.Terminal,
		Failed:      outcome.Failed,
		FreshOutput: notificationPayloadBool(event.Payload, "turn_output_fresh"),
		EvidenceKey: strings.TrimSpace(notificationPayloadString(event.Payload, "assistant_evidence_key")),
	}
}
