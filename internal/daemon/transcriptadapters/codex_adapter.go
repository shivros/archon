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
	providerName  string
	classifier    ProviderEventClassifier
	textExtractor TranscriptTextExtractor
}

func NewCodexTranscriptAdapter(providerName string) *codexTranscriptAdapter {
	providerName = providers.Normalize(providerName)
	if providerName == "" {
		providerName = "codex"
	}
	return &codexTranscriptAdapter{
		providerName:  providerName,
		classifier:    NewCodexEventClassifier(providerName),
		textExtractor: newCodexTranscriptTextExtractor(),
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
	textExtractor := a.textExtractor
	if textExtractor == nil {
		textExtractor = newCodexTranscriptTextExtractor()
	}
	mapped, ok := mapCodexEventWithClassifier(a.providerName, classifier, textExtractor, ctx, event)
	if !ok {
		return nil
	}
	if err := transcriptdomain.ValidateEvent(mapped); err != nil {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{mapped}
}

func (a codexTranscriptAdapter) MapItem(ctx MappingContext, item map[string]any) []transcriptdomain.TranscriptEvent {
	textExtractor := a.textExtractor
	if textExtractor == nil {
		textExtractor = newCodexTranscriptTextExtractor()
	}
	event, ok := deltaEventFromItemWithExtractor(ctx.SessionID, a.providerName, ctx.Revision, item, textExtractor)
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
	textExtractor TranscriptTextExtractor,
	ctx MappingContext,
	event types.CodexEvent,
) (transcriptdomain.TranscriptEvent, bool) {
	if classifier == nil {
		classifier = NewCodexEventClassifier(providerName)
	}
	if textExtractor == nil {
		textExtractor = newCodexTranscriptTextExtractor()
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
		block, ok := deltaBlockFromCodexEventMethod(method, event.Params, textExtractor)
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
			block, ok := blockFromCodexEventItem(method, event.Params, textExtractor)
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

func deltaBlockFromCodexEventMethod(method string, raw json.RawMessage, extractor TranscriptTextExtractor) (transcriptdomain.Block, bool) {
	params := decodeMap(raw)
	if extractor == nil {
		extractor = newCodexTranscriptTextExtractor()
	}
	text, ok := firstPresentExtracted(extractor,
		params["delta"],
		params["text"],
		params["content"],
	)
	if !ok {
		return transcriptdomain.Block{}, false
	}
	lowerMethod := strings.ToLower(strings.TrimSpace(method))
	role := "assistant"
	variant := ""
	if strings.Contains(lowerMethod, "reasoning") {
		role = "reasoning"
		variant = "reasoning"
	}
	block := transcriptdomain.Block{
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
		Meta:    transcriptMetaFromCodexEventParams(params),
	}
	ensureTranscriptBlockIdentityMeta(&block)
	return block, true
}

func blockFromCodexEventItem(method string, raw json.RawMessage, extractor TranscriptTextExtractor) (transcriptdomain.Block, bool) {
	params := decodeMap(raw)
	item, _ := params["item"].(map[string]any)
	if item == nil {
		return transcriptdomain.Block{}, false
	}
	if extractor == nil {
		extractor = newCodexTranscriptTextExtractor()
	}
	block, ok := blockFromItemWithExtractor(item, extractor)
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
	mergeTranscriptBlockMeta(&block, transcriptMetaFromCodexEventParams(params))
	if strings.EqualFold(strings.TrimSpace(method), "item/completed") {
		if block.Meta == nil {
			block.Meta = map[string]any{}
		}
		block.Meta["final"] = true
	}
	ensureTranscriptBlockIdentityMeta(&block)
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
	return blockFromItemWithExtractor(item, newCodexTranscriptTextExtractor())
}

func blockFromItemWithExtractor(item map[string]any, extractor TranscriptTextExtractor) (transcriptdomain.Block, bool) {
	if item == nil {
		return transcriptdomain.Block{}, false
	}
	kind := strings.TrimSpace(asString(item["type"]))
	role := strings.TrimSpace(asString(item["role"]))
	if extractor == nil {
		extractor = newCodexTranscriptTextExtractor()
	}
	text, ok := firstPresentExtracted(extractor,
		item["text"],
		item["delta"],
		item["content"],
	)
	if !ok {
		text = firstNonEmptyExtracted(extractor,
			item["content"],
			item["message"],
			item["result"],
		)
	}
	if transcriptdomain.IsSemanticallyEmpty(text) {
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
	block := transcriptdomain.Block{
		ID:      id,
		Kind:    kind,
		Role:    role,
		Text:    text,
		Variant: variant,
		Meta:    transcriptMetaFromCodexItem(item),
	}
	ensureTranscriptBlockIdentityMeta(&block)
	return block, true
}

func transcriptMetaFromCodexEventParams(params map[string]any) map[string]any {
	if len(params) == 0 {
		return nil
	}
	meta := map[string]any{}
	if turnID := strings.TrimSpace(firstNonEmpty(
		asString(params["turn_id"]),
		asString(params["turnId"]),
	)); turnID != "" {
		meta["turn_id"] = turnID
	}
	if providerMessageID := strings.TrimSpace(firstNonEmpty(
		asString(params["provider_message_id"]),
		asString(params["providerMessageID"]),
		asString(params["message_id"]),
		asString(params["messageId"]),
		asString(params["item_id"]),
		asString(params["itemId"]),
		asString(params["itemid"]),
		asString(params["id"]),
	)); providerMessageID != "" {
		meta["provider_message_id"] = providerMessageID
	}
	for _, key := range []string{"provider_created_at", "created_at", "createdAt", "timestamp", "ts"} {
		if value := params[key]; value != nil && strings.TrimSpace(asString(value)) != "" {
			meta[key] = value
			break
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func transcriptMetaFromCodexItem(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	meta := map[string]any{}
	if turnID := strings.TrimSpace(firstNonEmpty(
		asString(item["turn_id"]),
		asString(item["turnID"]),
	)); turnID != "" {
		meta["turn_id"] = turnID
	} else if turnRaw, ok := item["turn"].(map[string]any); ok && turnRaw != nil {
		if turnID := strings.TrimSpace(asString(turnRaw["id"])); turnID != "" {
			meta["turn_id"] = turnID
		}
	}
	if providerMessageID := strings.TrimSpace(firstNonEmpty(
		asString(item["provider_message_id"]),
		asString(item["providerMessageID"]),
		asString(item["message_id"]),
		asString(item["messageId"]),
		asString(item["id"]),
		asString(item["item_id"]),
	)); providerMessageID != "" {
		meta["provider_message_id"] = providerMessageID
	} else if message, ok := item["message"].(map[string]any); ok && message != nil {
		if providerMessageID := strings.TrimSpace(firstNonEmpty(
			asString(message["id"]),
			asString(message["message_id"]),
		)); providerMessageID != "" {
			meta["provider_message_id"] = providerMessageID
		}
	}
	for _, key := range []string{"provider_created_at", "created_at", "createdAt", "timestamp", "ts"} {
		if value := item[key]; value != nil && strings.TrimSpace(asString(value)) != "" {
			meta[key] = value
			break
		}
	}
	if _, ok := meta["created_at"]; !ok {
		if message, ok := item["message"].(map[string]any); ok && message != nil {
			for _, key := range []string{"created_at", "createdAt", "timestamp", "ts"} {
				if value := message[key]; value != nil && strings.TrimSpace(asString(value)) != "" {
					meta["created_at"] = value
					break
				}
			}
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func mergeTranscriptBlockMeta(block *transcriptdomain.Block, incoming map[string]any) {
	if block == nil || len(incoming) == 0 {
		return
	}
	if block.Meta == nil {
		block.Meta = map[string]any{}
	}
	for key, value := range incoming {
		if _, exists := block.Meta[key]; exists {
			continue
		}
		block.Meta[key] = value
	}
}

func ensureTranscriptBlockIdentityMeta(block *transcriptdomain.Block) {
	if block == nil {
		return
	}
	if block.Meta == nil {
		block.Meta = map[string]any{}
	}
	if strings.TrimSpace(asString(block.Meta["provider_message_id"])) == "" && strings.TrimSpace(block.ID) != "" {
		block.Meta["provider_message_id"] = strings.TrimSpace(block.ID)
	}
	if len(block.Meta) == 0 {
		block.Meta = nil
	}
}

func itemTextFromContent(raw any) string {
	return firstNonEmptyExtracted(newCodexTranscriptTextExtractor(), raw)
}

func itemTextFromMap(raw map[string]any) string {
	return firstNonEmptyExtracted(newCodexTranscriptTextExtractor(), raw)
}

func DeltaEventFromItem(
	sessionID, provider string,
	revision transcriptdomain.RevisionToken,
	item map[string]any,
) (transcriptdomain.TranscriptEvent, bool) {
	return deltaEventFromItemWithExtractor(sessionID, provider, revision, item, newCodexTranscriptTextExtractor())
}

func deltaEventFromItemWithExtractor(
	sessionID, provider string,
	revision transcriptdomain.RevisionToken,
	item map[string]any,
	extractor TranscriptTextExtractor,
) (transcriptdomain.TranscriptEvent, bool) {
	block, ok := blockFromItemWithExtractor(item, extractor)
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
