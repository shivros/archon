package transcriptadapters

import (
	"encoding/json"
	"strings"

	"control/internal/daemon/acp"
	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
	"control/internal/types"
)

type hermesTranscriptAdapter struct {
	providerName string
}

func NewHermesTranscriptAdapter(providerName string) *hermesTranscriptAdapter {
	providerName = providers.Normalize(providerName)
	if providerName == "" {
		providerName = "hermes"
	}
	return &hermesTranscriptAdapter{providerName: providerName}
}

func (a hermesTranscriptAdapter) Provider() string {
	return a.providerName
}

func (a hermesTranscriptAdapter) MapEvent(ctx MappingContext, event types.CodexEvent) []transcriptdomain.TranscriptEvent {
	mapped, ok := mapHermesEvent(a.providerName, ctx, event)
	if !ok {
		return nil
	}
	if err := transcriptdomain.ValidateEvent(mapped); err != nil {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{mapped}
}

func mapHermesEvent(providerName string, ctx MappingContext, event types.CodexEvent) (transcriptdomain.TranscriptEvent, bool) {
	canonical := transcriptdomain.TranscriptEvent{
		SessionID: strings.TrimSpace(ctx.SessionID),
		Provider:  strings.TrimSpace(providerName),
		Revision:  ctx.Revision,
	}
	if ts := parseEventTime(event.TS); !ts.IsZero() {
		canonical.OccurredAt = &ts
	}

	switch strings.TrimSpace(event.Method) {
	case "turn/started":
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateRunning)
		if turn.TurnID == "" {
			turn.TurnID = strings.TrimSpace(ctx.ActiveTurnID)
		}
		canonical.Kind = transcriptdomain.TranscriptEventTurnStarted
		canonical.Turn = &turn
		return canonical, true
	case "turn/completed":
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateCompleted)
		if turn.TurnID == "" {
			turn.TurnID = strings.TrimSpace(ctx.ActiveTurnID)
		}
		canonical.Kind = transcriptdomain.TranscriptEventTurnCompleted
		canonical.Turn = &turn
		return canonical, true
	case "turn/failed":
		turn := turnStateFromEventParams(event.Params, transcriptdomain.TurnStateFailed)
		if turn.TurnID == "" {
			turn.TurnID = strings.TrimSpace(ctx.ActiveTurnID)
		}
		if turn.Error == "" {
			turn.Error = "hermes turn failed"
		}
		canonical.Kind = transcriptdomain.TranscriptEventTurnFailed
		canonical.Turn = &turn
		return canonical, true
	case acp.MethodRequestPermission:
		canonical.Kind = transcriptdomain.TranscriptEventApprovalPending
		canonical.Approval = &transcriptdomain.ApprovalState{
			RequestID: approvalRequestID(event),
			State:     "pending",
			Method:    strings.TrimSpace(event.Method),
		}
		return canonical, true
	case "permission/replied":
		canonical.Kind = transcriptdomain.TranscriptEventApprovalResolved
		canonical.Approval = &transcriptdomain.ApprovalState{
			RequestID: approvalRequestID(event),
			State:     "resolved",
			Method:    strings.TrimSpace(event.Method),
		}
		return canonical, true
	case acp.MethodSessionUpdate:
		update, err := acp.DecodeSessionUpdate(event.Params)
		if err != nil {
			return transcriptdomain.TranscriptEvent{}, false
		}
		block, ok := hermesBlockFromUpdate(update)
		if !ok {
			return transcriptdomain.TranscriptEvent{}, false
		}
		canonical.Kind = transcriptdomain.TranscriptEventDelta
		canonical.Delta = []transcriptdomain.Block{block}
		return canonical, true
	default:
		return transcriptdomain.TranscriptEvent{}, false
	}
}

func hermesBlockFromUpdate(update acp.SessionUpdateNotification) (transcriptdomain.Block, bool) {
	switch update.SessionUpdate {
	case acp.SessionUpdateAgentMessageChunk:
		return hermesTextBlock("message", "assistant", "", []acp.ContentBlock{update.AgentMessageChunk.Content}, nil), true
	case acp.SessionUpdateUserMessageChunk:
		return hermesTextBlock("message", "user", "", []acp.ContentBlock{update.UserMessageChunk.Content}, nil), true
	case acp.SessionUpdateAgentThoughtChunk:
		return hermesTextBlock("thinking", "assistant", "thinking", []acp.ContentBlock{update.AgentThoughtChunk.Content}, nil), true
	case acp.SessionUpdateToolCall:
		meta := map[string]any{
			"tool_call_id": strings.TrimSpace(update.ToolCall.ToolCallID),
			"tool_kind":    strings.TrimSpace(update.ToolCall.Kind),
			"status":       strings.TrimSpace(update.ToolCall.Status),
			"title":        strings.TrimSpace(update.ToolCall.Title),
		}
		return hermesTextBlock("tool_call", "assistant", "started", hermesToolContentBlocks(update.ToolCall.Content), meta), true
	case acp.SessionUpdateToolCallUpdate:
		meta := map[string]any{
			"tool_call_id": strings.TrimSpace(update.ToolCallUpdate.ToolCallID),
			"tool_kind":    strings.TrimSpace(update.ToolCallUpdate.Kind),
			"status":       strings.TrimSpace(update.ToolCallUpdate.Status),
			"title":        strings.TrimSpace(update.ToolCallUpdate.Title),
		}
		variant := strings.TrimSpace(update.ToolCallUpdate.Status)
		if variant == "" {
			variant = "update"
		}
		return hermesTextBlock("tool_call", "assistant", variant, hermesToolContentBlocks(update.ToolCallUpdate.Content), meta), true
	case acp.SessionUpdatePlan:
		meta := map[string]any{"entries": len(update.Plan.Entries)}
		return transcriptdomain.Block{
			Kind:    "plan",
			Role:    "assistant",
			Text:    hermesPlanText(update.Plan),
			Variant: "plan",
			Meta:    meta,
		}, true
	default:
		if len(update.Raw) == 0 {
			return transcriptdomain.Block{}, false
		}
		return transcriptdomain.Block{
			Kind:    "event",
			Role:    "assistant",
			Text:    string(update.Raw),
			Variant: "raw",
		}, true
	}
}

func hermesTextBlock(kind, role, variant string, blocks []acp.ContentBlock, meta map[string]any) transcriptdomain.Block {
	text := hermesContentText(blocks)
	if text == "" {
		text = "{}"
	}
	return transcriptdomain.Block{
		Kind:    kind,
		Role:    role,
		Text:    text,
		Variant: variant,
		Meta:    meta,
	}
}

func hermesToolContentBlocks(content []acp.ToolCallContent) []acp.ContentBlock {
	if len(content) == 0 {
		return nil
	}
	out := make([]acp.ContentBlock, 0, len(content))
	for _, item := range content {
		if item.Content == nil {
			continue
		}
		out = append(out, *item.Content)
	}
	return out
}

func hermesContentText(blocks []acp.ContentBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if text := strings.TrimSpace(block.Text); text != "" {
			parts = append(parts, text)
			continue
		}
		if raw, err := json.Marshal(block); err == nil {
			parts = append(parts, string(raw))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func hermesPlanText(plan *acp.Plan) string {
	if plan == nil || len(plan.Entries) == 0 {
		return "[]"
	}
	data, err := json.Marshal(plan.Entries)
	if err != nil {
		return "[]"
	}
	return string(data)
}
