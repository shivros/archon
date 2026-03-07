package transcriptadapters

import (
	"encoding/json"
	"strings"
	"time"

	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
	"control/internal/types"
)

type codexTranscriptAdapter struct {
	providerName string
	classifier   ProviderEventClassifier
}

func NewCodexTranscriptAdapter(providerName string) *codexTranscriptAdapter {
	providerName = providers.Normalize(providerName)
	if providerName == "" {
		providerName = "codex"
	}
	return &codexTranscriptAdapter{
		providerName: providerName,
		classifier:   NewCodexEventClassifier(providerName),
	}
}

func (a codexTranscriptAdapter) Provider() string {
	return a.providerName
}

func (a codexTranscriptAdapter) MapEvent(ctx MappingContext, event types.CodexEvent) []transcriptdomain.TranscriptEvent {
	classifier := a.classifier
	if classifier == nil {
		classifier = NewCodexEventClassifier(a.providerName)
	}
	mapped, ok := mapCodexEventWithClassifier(a.providerName, classifier, ctx, event)
	if !ok {
		return nil
	}
	if err := transcriptdomain.ValidateEvent(mapped); err != nil {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{mapped}
}

func (a codexTranscriptAdapter) MapItem(ctx MappingContext, item map[string]any) []transcriptdomain.TranscriptEvent {
	event, ok := DeltaEventFromItem(ctx.SessionID, a.providerName, ctx.Revision, item)
	if !ok {
		return nil
	}
	if err := transcriptdomain.ValidateEvent(event); err != nil {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{event}
}

func mapCodexEventWithClassifier(
	providerName string,
	classifier ProviderEventClassifier,
	ctx MappingContext,
	event types.CodexEvent,
) (transcriptdomain.TranscriptEvent, bool) {
	if classifier == nil {
		classifier = NewCodexEventClassifier(providerName)
	}
	classified := classifier.ClassifyEvent(event)
	method := classified.Method
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
	case EventIntentAssistantDelta:
		block, ok := deltaBlockFromCodexEventMethod(method, event.Params)
		if !ok {
			return transcriptdomain.TranscriptEvent{}, false
		}
		canonical.Delta = []transcriptdomain.Block{block}
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
	default:
		switch {
		case strings.HasPrefix(method, "item/"):
			block, ok := blockFromCodexEventItem(method, event.Params)
			if !ok {
				return transcriptdomain.TranscriptEvent{}, false
			}
			canonical.Delta = []transcriptdomain.Block{block}
		default:
			return transcriptdomain.TranscriptEvent{}, false
		}
	}
	return canonical, true
}

func CapabilityEnvelopeFromProvider(provider string) transcriptdomain.CapabilityEnvelope {
	caps := providers.CapabilitiesFor(provider)
	return transcriptdomain.CapabilityEnvelope{
		SupportsGuidedWorkflowDispatch: caps.SupportsGuidedWorkflowDispatch,
		UsesItems:                      caps.UsesItems,
		SupportsEvents:                 caps.SupportsEvents,
		SupportsApprovals:              caps.SupportsApprovals,
		SupportsInterrupt:              caps.SupportsInterrupt,
		NoProcess:                      caps.NoProcess,
	}
}

func TranscriptEventFromCodexEvent(
	sessionID, provider string,
	revision transcriptdomain.RevisionToken,
	event types.CodexEvent,
) transcriptdomain.TranscriptEvent {
	adapter := NewCodexTranscriptAdapter(provider)
	events := adapter.MapEvent(MappingContext{SessionID: sessionID, Revision: revision}, event)
	if len(events) == 0 {
		return transcriptdomain.TranscriptEvent{}
	}
	return events[0]
}

func deltaBlockFromCodexEventMethod(method string, raw json.RawMessage) (transcriptdomain.Block, bool) {
	params := decodeMap(raw)
	text := strings.TrimSpace(firstNonEmpty(
		asString(params["delta"]),
		asString(params["text"]),
		asString(params["content"]),
	))
	if text == "" {
		return transcriptdomain.Block{}, false
	}
	lowerMethod := strings.ToLower(strings.TrimSpace(method))
	role := "assistant"
	variant := ""
	if strings.Contains(lowerMethod, "reasoning") {
		role = "reasoning"
		variant = "reasoning"
	}
	return transcriptdomain.Block{
		ID: strings.TrimSpace(firstNonEmpty(
			asString(params["item_id"]),
			asString(params["itemId"]),
			asString(params["itemid"]),
			asString(params["id"]),
		)),
		Kind:    itemKindFromMethod(lowerMethod),
		Role:    role,
		Text:    text,
		Variant: variant,
	}, true
}

func blockFromCodexEventItem(method string, raw json.RawMessage) (transcriptdomain.Block, bool) {
	params := decodeMap(raw)
	item, _ := params["item"].(map[string]any)
	if item == nil {
		return transcriptdomain.Block{}, false
	}
	block, ok := BlockFromItem(item)
	if !ok {
		return transcriptdomain.Block{}, false
	}
	if block.ID == "" {
		block.ID = strings.TrimSpace(firstNonEmpty(
			asString(params["item_id"]),
			asString(params["itemId"]),
			asString(params["itemid"]),
			asString(params["id"]),
		))
	}
	if block.Variant == "" {
		if strings.Contains(strings.ToLower(method), "reasoning") {
			block.Variant = "reasoning"
		}
	}
	return block, true
}

func itemKindFromMethod(method string) string {
	trimmed := strings.TrimSpace(strings.ToLower(method))
	if !strings.HasPrefix(trimmed, "item/") {
		return "message"
	}
	parts := strings.Split(strings.TrimPrefix(trimmed, "item/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "message"
	}
	return strings.TrimSpace(parts[0])
}

func threadStatusFromEventParams(raw json.RawMessage) string {
	params := decodeMap(raw)
	status, _ := params["status"].(map[string]any)
	if status != nil {
		return strings.ToLower(strings.TrimSpace(asString(status["type"])))
	}
	return strings.ToLower(strings.TrimSpace(asString(params["status"])))
}

func BlockFromItem(item map[string]any) (transcriptdomain.Block, bool) {
	if item == nil {
		return transcriptdomain.Block{}, false
	}
	kind := strings.TrimSpace(asString(item["type"]))
	role := strings.TrimSpace(asString(item["role"]))
	text := strings.TrimSpace(firstNonEmpty(
		asString(item["text"]),
		asString(item["delta"]),
		asString(item["content"]),
	))
	if text == "" {
		return transcriptdomain.Block{}, false
	}
	if kind == "" {
		kind = "message"
	}
	if role == "" {
		lower := strings.ToLower(kind)
		switch {
		case strings.Contains(lower, "assistant"), strings.Contains(lower, "agent"):
			role = "assistant"
		case strings.Contains(lower, "user"):
			role = "user"
		}
	}
	id := strings.TrimSpace(firstNonEmpty(
		asString(item["id"]),
		asString(item["item_id"]),
		asString(item["message_id"]),
	))
	variant := strings.TrimSpace(firstNonEmpty(
		asString(item["variant"]),
		asString(item["subtype"]),
	))
	return transcriptdomain.Block{ID: id, Kind: kind, Role: role, Text: text, Variant: variant}, true
}

func DeltaEventFromItem(
	sessionID, provider string,
	revision transcriptdomain.RevisionToken,
	item map[string]any,
) (transcriptdomain.TranscriptEvent, bool) {
	block, ok := BlockFromItem(item)
	if !ok {
		return transcriptdomain.TranscriptEvent{}, false
	}
	return transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: strings.TrimSpace(sessionID),
		Provider:  strings.TrimSpace(provider),
		Revision:  revision,
		Delta:     []transcriptdomain.Block{block},
	}, true
}

func turnStateFromEventParams(raw json.RawMessage, fallback transcriptdomain.TurnLifecycleState) transcriptdomain.TurnState {
	params := decodeMap(raw)
	turnID := strings.TrimSpace(firstNonEmpty(
		asString(params["turn_id"]),
		asString(params["turnId"]),
		asString(params["id"]),
	))
	errMsg := strings.TrimSpace(firstNonEmpty(
		asString(params["error"]),
		asString(params["message"]),
	))
	state := fallback
	if status := strings.TrimSpace(asString(params["status"])); status != "" {
		switch strings.ToLower(status) {
		case "running", "in_progress", "started":
			state = transcriptdomain.TurnStateRunning
		case "completed", "done", "success":
			state = transcriptdomain.TurnStateCompleted
		case "failed", "error", "abandoned":
			state = transcriptdomain.TurnStateFailed
		}
	}
	if state == transcriptdomain.TurnStateFailed && errMsg == "" {
		errMsg = "provider error"
	}
	return transcriptdomain.TurnState{
		State:  state,
		TurnID: turnID,
		Error:  errMsg,
	}
}

func approvalRequestID(event types.CodexEvent) int {
	if event.ID != nil {
		return *event.ID
	}
	params := decodeMap(event.Params)
	if id, ok := asInt(params["request_id"]); ok {
		return id
	}
	if id, ok := asInt(params["requestId"]); ok {
		return id
	}
	if id, ok := asInt(params["id"]); ok {
		return id
	}
	return 0
}

func parseEventTime(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t
	}
	return time.Time{}
}

func decodeMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return map[string]any{}
	}
	return payload
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func asInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
