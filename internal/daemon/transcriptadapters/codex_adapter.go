package transcriptadapters

import (
	"encoding/json"
	"strings"
	"time"

	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
	"control/internal/types"
)

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
	method := strings.ToLower(strings.TrimSpace(event.Method))
	canonical := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: strings.TrimSpace(sessionID),
		Provider:  strings.TrimSpace(provider),
		Revision:  revision,
	}
	if ts := parseEventTime(event.TS); !ts.IsZero() {
		canonical.OccurredAt = &ts
	}

	switch {
	case method == "turn/started":
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateRunning)
		canonical.Kind = transcriptdomain.TranscriptEventTurnStarted
		canonical.Turn = &turn
	case method == "turn/completed":
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateCompleted)
		canonical.Kind = transcriptdomain.TranscriptEventTurnCompleted
		canonical.Turn = &turn
	case method == "session.idle":
		canonical.Kind = transcriptdomain.TranscriptEventStreamStatus
		canonical.StreamStatus = transcriptdomain.StreamStatusReady
	case method == "error":
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateFailed)
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
	}
	return canonical
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
		case strings.Contains(lower, "assistant"):
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
		case "failed", "error":
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
