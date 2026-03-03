package transcriptadapters

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
	"control/internal/types"
)

type openCodeTranscriptAdapter struct {
	providerName string
}

func NewOpenCodeTranscriptAdapter(providerName string) *openCodeTranscriptAdapter {
	providerName = providers.Normalize(providerName)
	if providerName == "" {
		providerName = "opencode"
	}
	return &openCodeTranscriptAdapter{providerName: providerName}
}

func (a openCodeTranscriptAdapter) Provider() string {
	return a.providerName
}

func (a openCodeTranscriptAdapter) MapEvent(ctx MappingContext, event types.CodexEvent) []transcriptdomain.TranscriptEvent {
	mapped := mapOpenCodeEvent(a.providerName, ctx, event)
	if err := transcriptdomain.ValidateEvent(mapped); err != nil {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{mapped}
}

func mapOpenCodeEvent(providerName string, ctx MappingContext, event types.CodexEvent) transcriptdomain.TranscriptEvent {
	method := strings.ToLower(strings.TrimSpace(event.Method))
	canonical := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: strings.TrimSpace(ctx.SessionID),
		Provider:  strings.TrimSpace(providerName),
		Revision:  ctx.Revision,
	}
	if ts := parseEventTime(event.TS); !ts.IsZero() {
		canonical.OccurredAt = &ts
	}

	switch {
	case method == "turn/started":
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateRunning)
		if strings.TrimSpace(turn.TurnID) == "" {
			turn.TurnID = strings.TrimSpace(ctx.ActiveTurnID)
		}
		canonical.Kind = transcriptdomain.TranscriptEventTurnStarted
		canonical.Turn = &turn
	case method == "turn/completed":
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateCompleted)
		if strings.TrimSpace(turn.TurnID) == "" {
			turn.TurnID = strings.TrimSpace(ctx.ActiveTurnID)
		}
		canonical.Kind = transcriptdomain.TranscriptEventTurnCompleted
		canonical.Turn = &turn
	case method == "session.idle":
		canonical.Kind = transcriptdomain.TranscriptEventStreamStatus
		canonical.StreamStatus = transcriptdomain.StreamStatusReady
	case method == "error":
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateFailed)
		if strings.TrimSpace(turn.TurnID) == "" {
			turn.TurnID = strings.TrimSpace(ctx.ActiveTurnID)
		}
		canonical.Kind = transcriptdomain.TranscriptEventTurnFailed
		canonical.Turn = &turn
	case strings.Contains(method, "requestapproval"):
		canonical.Kind = transcriptdomain.TranscriptEventApprovalPending
		canonical.Approval = &transcriptdomain.ApprovalState{
			RequestID: approvalRequestID(event),
			State:     "pending",
			Method:    strings.TrimSpace(event.Method),
		}
	case strings.Contains(method, "approvalresolved") || strings.Contains(method, "replypermission"):
		canonical.Kind = transcriptdomain.TranscriptEventApprovalResolved
		canonical.Approval = &transcriptdomain.ApprovalState{
			RequestID: approvalRequestID(event),
			State:     "resolved",
			Method:    strings.TrimSpace(event.Method),
		}
	default:
		canonical.Delta = []transcriptdomain.Block{{
			Kind: "provider_event",
			Role: "system",
			Text: strings.TrimSpace(event.Method),
		}}
	}
	return canonical
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
