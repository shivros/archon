package app

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"control/internal/daemon/transcriptdomain"
)

func transcriptBlocksToChatBlocks(blocks []transcriptdomain.Block) []ChatBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]ChatBlock, 0, len(blocks))
	for _, block := range blocks {
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		role := transcriptBlockRole(block)
		chatBlock := ChatBlock{
			ID:                strings.TrimSpace(block.ID),
			Role:              role,
			Text:              text,
			TurnID:            transcriptMetaString(block.Meta, "turn_id", "turnId"),
			ProviderMessageID: transcriptMetaString(block.Meta, "provider_message_id", "providerMessageID", "message_id"),
		}
		if requestID, ok := transcriptMetaInt(block.Meta, "request_id", "requestId", "id"); ok {
			chatBlock.RequestID = requestID
		}
		if createdAt := transcriptMetaTime(block.Meta, "provider_created_at", "created_at", "createdAt", "timestamp", "ts"); !createdAt.IsZero() {
			chatBlock.CreatedAt = createdAt
		}
		out = append(out, chatBlock)
	}
	return out
}

func transcriptBlockRole(block transcriptdomain.Block) ChatRole {
	role := strings.ToLower(strings.TrimSpace(block.Role))
	kind := strings.ToLower(strings.TrimSpace(block.Kind))
	switch {
	case role == "assistant" || role == "agent" || role == "model":
		return ChatRoleAgent
	case role == "user":
		return ChatRoleUser
	case role == "reasoning":
		return ChatRoleReasoning
	case role == "approval":
		return ChatRoleApproval
	case role == "approval_resolved":
		return ChatRoleApprovalResolved
	case role == "system":
		return ChatRoleSystem
	case strings.Contains(kind, "assistant") || strings.Contains(kind, "agent"):
		return ChatRoleAgent
	case strings.Contains(kind, "user"):
		return ChatRoleUser
	case strings.Contains(kind, "reason"):
		return ChatRoleReasoning
	case strings.Contains(kind, "approval_resolved"):
		return ChatRoleApprovalResolved
	case strings.Contains(kind, "approval"):
		return ChatRoleApproval
	default:
		return ChatRoleSystem
	}
}

func transcriptMetaString(meta map[string]any, keys ...string) string {
	if len(meta) == 0 {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(asString(meta[key])); value != "" {
			return value
		}
	}
	return ""
}

func transcriptMetaInt(meta map[string]any, keys ...string) (int, bool) {
	if len(meta) == 0 {
		return 0, false
	}
	for _, key := range keys {
		if value, ok := transcriptAnyInt(meta[key]); ok {
			return value, true
		}
	}
	return 0, false
}

func transcriptMetaTime(meta map[string]any, keys ...string) (at time.Time) {
	if len(meta) == 0 {
		return time.Time{}
	}
	for _, key := range keys {
		if parsed := parseChatTimestamp(meta[key]); !parsed.IsZero() {
			return parsed
		}
	}
	return time.Time{}
}

func transcriptAnyInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed, true
		}
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed), true
		}
	}
	return 0, false
}
