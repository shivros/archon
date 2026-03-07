package transcriptadapters

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
	"control/internal/types"
)

type openCodeTranscriptAdapter struct {
	providerName string
	classifier   ProviderEventClassifier
}

func NewOpenCodeTranscriptAdapter(providerName string) *openCodeTranscriptAdapter {
	providerName = providers.Normalize(providerName)
	if providerName == "" {
		providerName = "opencode"
	}
	return &openCodeTranscriptAdapter{
		providerName: providerName,
		classifier:   NewOpenCodeEventClassifier(providerName),
	}
}

func (a openCodeTranscriptAdapter) Provider() string {
	return a.providerName
}

func (a openCodeTranscriptAdapter) MapEvent(ctx MappingContext, event types.CodexEvent) []transcriptdomain.TranscriptEvent {
	classifier := a.classifier
	if classifier == nil {
		classifier = NewOpenCodeEventClassifier(a.providerName)
	}
	mapped, ok := mapOpenCodeEventWithClassifier(a.providerName, classifier, ctx, event)
	if !ok {
		return nil
	}
	if err := transcriptdomain.ValidateEvent(mapped); err != nil {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{mapped}
}

func mapOpenCodeEventWithClassifier(
	providerName string,
	classifier ProviderEventClassifier,
	ctx MappingContext,
	event types.CodexEvent,
) (transcriptdomain.TranscriptEvent, bool) {
	if classifier == nil {
		classifier = NewOpenCodeEventClassifier(providerName)
	}
	classified := classifier.ClassifyEvent(event)
	canonical := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: strings.TrimSpace(ctx.SessionID),
		Provider:  strings.TrimSpace(providerName),
		Revision:  ctx.Revision,
	}
	if ts := parseEventTime(event.TS); !ts.IsZero() {
		canonical.OccurredAt = &ts
	}

	switch classified.Intent {
	case EventIntentTurnStarted:
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateRunning)
		if strings.TrimSpace(turn.TurnID) == "" {
			turn.TurnID = strings.TrimSpace(ctx.ActiveTurnID)
		}
		canonical.Kind = transcriptdomain.TranscriptEventTurnStarted
		canonical.Turn = &turn
	case EventIntentTurnCompleted:
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateCompleted)
		if strings.TrimSpace(turn.TurnID) == "" {
			turn.TurnID = strings.TrimSpace(ctx.ActiveTurnID)
		}
		canonical.Kind = transcriptdomain.TranscriptEventTurnCompleted
		canonical.Turn = &turn
	case EventIntentStreamReady:
		canonical.Kind = transcriptdomain.TranscriptEventStreamStatus
		canonical.StreamStatus = transcriptdomain.StreamStatusReady
	case EventIntentTurnFailed:
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateFailed)
		if strings.TrimSpace(turn.TurnID) == "" {
			turn.TurnID = strings.TrimSpace(ctx.ActiveTurnID)
		}
		canonical.Kind = transcriptdomain.TranscriptEventTurnFailed
		canonical.Turn = &turn
	case EventIntentApprovalPending:
		canonical.Kind = transcriptdomain.TranscriptEventApprovalPending
		canonical.Approval = &transcriptdomain.ApprovalState{
			RequestID: approvalRequestID(event),
			State:     "pending",
			Method:    strings.TrimSpace(event.Method),
		}
	case EventIntentApprovalResolved:
		canonical.Kind = transcriptdomain.TranscriptEventApprovalResolved
		canonical.Approval = &transcriptdomain.ApprovalState{
			RequestID: approvalRequestID(event),
			State:     "resolved",
			Method:    strings.TrimSpace(event.Method),
		}
	default:
		return transcriptdomain.TranscriptEvent{}, false
	}
	return canonical, true
}

func (a openCodeTranscriptAdapter) MapItem(ctx MappingContext, item map[string]any) []transcriptdomain.TranscriptEvent {
	if item == nil {
		return nil
	}
	itemType := strings.ToLower(strings.TrimSpace(asString(item["type"])))
	switch itemType {
	case "turncompletion":
		return mapTurnCompletionItem(a.providerName, ctx, item)
	default:
		event, ok := DeltaEventFromItem(ctx.SessionID, a.providerName, ctx.Revision, item)
		if !ok {
			return nil
		}
		if err := transcriptdomain.ValidateEvent(event); err != nil {
			return nil
		}
		return []transcriptdomain.TranscriptEvent{event}
	}
}

func mapTurnCompletionItem(
	providerName string,
	ctx MappingContext,
	item map[string]any,
) []transcriptdomain.TranscriptEvent {
	if item == nil {
		return nil
	}
	turnID := strings.TrimSpace(firstNonEmpty(
		asString(item["turn_id"]),
		asString(item["turnId"]),
		ctx.ActiveTurnID,
	))
	if turnID == "" {
		return nil
	}
	status := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		asString(item["turn_status"]),
		asString(item["status"]),
	)))
	errText := strings.TrimSpace(firstNonEmpty(
		asString(item["turn_error"]),
		asString(item["error"]),
	))

	event := transcriptdomain.TranscriptEvent{
		SessionID: strings.TrimSpace(ctx.SessionID),
		Provider:  strings.TrimSpace(providerName),
		Revision:  ctx.Revision,
	}
	switch status {
	case "failed", "error", "abandoned":
		event.Kind = transcriptdomain.TranscriptEventTurnFailed
		if errText == "" {
			errText = "provider error"
		}
		event.Turn = &transcriptdomain.TurnState{State: transcriptdomain.TurnStateFailed, TurnID: turnID, Error: errText}
	default:
		event.Kind = transcriptdomain.TranscriptEventTurnCompleted
		event.Turn = &transcriptdomain.TurnState{State: transcriptdomain.TurnStateCompleted, TurnID: turnID}
	}
	if err := transcriptdomain.ValidateEvent(event); err != nil {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{event}
}
