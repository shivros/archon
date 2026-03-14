package transcriptadapters

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
)

type claudeTranscriptAdapter struct {
	providerName string
}

func NewClaudeTranscriptAdapter(providerName string) *claudeTranscriptAdapter {
	providerName = providers.Normalize(providerName)
	if providerName == "" {
		providerName = "claude"
	}
	return &claudeTranscriptAdapter{providerName: providerName}
}

func (a claudeTranscriptAdapter) Provider() string {
	return a.providerName
}

func (a claudeTranscriptAdapter) MapItem(ctx MappingContext, item map[string]any) []transcriptdomain.TranscriptEvent {
	if item == nil {
		return nil
	}
	itemType := strings.ToLower(strings.TrimSpace(asString(item["type"])))
	switch itemType {
	case "usermessage":
		return a.mapMessageItem(ctx, item, "user_message", "user", "")
	case "agentmessagedelta", "agentmessage", "assistant", "reasoning":
		return a.mapMessageItem(ctx, item, itemType, "assistant", itemVariantForClaudeItem(itemType, item))
	case "agentmessageend":
		turnID := strings.TrimSpace(firstNonEmpty(asString(item["turn_id"]), asString(item["turnId"]), ctx.ActiveTurnID))
		if turnID == "" {
			return nil
		}
		event := transcriptdomain.TranscriptEvent{
			Kind:      transcriptdomain.TranscriptEventTurnCompleted,
			SessionID: strings.TrimSpace(ctx.SessionID),
			Provider:  a.providerName,
			Revision:  ctx.Revision,
			Turn: &transcriptdomain.TurnState{
				State:  transcriptdomain.TurnStateCompleted,
				TurnID: turnID,
			},
		}
		if err := transcriptdomain.ValidateEvent(event); err != nil {
			return nil
		}
		return []transcriptdomain.TranscriptEvent{event}
	case "turncompletion":
		return mapTurnCompletionItem(a.providerName, ctx, item)
	default:
		return nil
	}
}

func (a claudeTranscriptAdapter) mapMessageItem(
	ctx MappingContext,
	item map[string]any,
	kind string,
	role string,
	variant string,
) []transcriptdomain.TranscriptEvent {
	text := strings.TrimSpace(extractItemText(item))
	if text == "" {
		return nil
	}
	id := strings.TrimSpace(firstNonEmpty(
		asString(item["id"]),
		asString(item["item_id"]),
		asString(item["message_id"]),
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
	event := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: strings.TrimSpace(ctx.SessionID),
		Provider:  a.providerName,
		Revision:  ctx.Revision,
		Delta:     []transcriptdomain.Block{block},
	}
	if err := transcriptdomain.ValidateEvent(event); err != nil {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{event}
}

func itemVariantForClaudeItem(itemType string, item map[string]any) string {
	variant := strings.TrimSpace(asString(item["variant"]))
	if itemType == "reasoning" && variant == "" {
		return "reasoning"
	}
	return variant
}

func extractItemText(item map[string]any) string {
	if item == nil {
		return ""
	}
	if direct := firstNonEmpty(
		asString(item["text"]),
		asString(item["delta"]),
		asString(item["content"]),
	); strings.TrimSpace(direct) != "" {
		return strings.TrimSpace(direct)
	}
	if message, ok := item["message"].(map[string]any); ok && message != nil {
		if nested := extractItemText(message); nested != "" {
			return nested
		}
	}
	content, ok := item["content"].([]map[string]any)
	if !ok {
		anyContent, ok := item["content"].([]any)
		if !ok {
			return ""
		}
		content = make([]map[string]any, 0, len(anyContent))
		for _, entry := range anyContent {
			if m, ok := entry.(map[string]any); ok {
				content = append(content, m)
			}
		}
	}
	parts := make([]string, 0, len(content))
	for _, block := range content {
		if block == nil {
			continue
		}
		text := strings.TrimSpace(firstNonEmpty(
			asString(block["text"]),
			asString(block["thinking"]),
		))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
