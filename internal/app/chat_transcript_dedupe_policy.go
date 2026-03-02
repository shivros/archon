package app

import (
	"strings"
	"time"
)

type TranscriptDedupePolicy interface {
	IsDuplicate(blocks []ChatBlock, role ChatRole, text, turnID, providerMessageID string, createdAt time.Time) bool
}

type defaultTranscriptDedupePolicy struct{}

func (defaultTranscriptDedupePolicy) IsDuplicate(blocks []ChatBlock, role ChatRole, text, turnID, providerMessageID string, createdAt time.Time) bool {
	text = normalizeTranscriptMessageText(text)
	if text == "" {
		return false
	}
	turnID = strings.TrimSpace(turnID)
	providerMessageID = strings.TrimSpace(providerMessageID)
	if providerMessageID != "" {
		for i := range blocks {
			block := blocks[i]
			if block.Role != role {
				continue
			}
			if strings.TrimSpace(block.ProviderMessageID) != providerMessageID {
				continue
			}
			if normalizeTranscriptMessageText(block.Text) == text {
				return true
			}
			if role == ChatRoleUser {
				return true
			}
		}
	}
	for i := range blocks {
		block := blocks[i]
		if block.Role != role {
			continue
		}
		if normalizeTranscriptMessageText(block.Text) != text {
			continue
		}
		blockTurnID := strings.TrimSpace(block.TurnID)
		if turnID != "" && blockTurnID == turnID {
			return true
		}
		if !createdAt.IsZero() && !block.CreatedAt.IsZero() && block.CreatedAt.Equal(createdAt) {
			return true
		}
		// Semantic replay fallback when providers don't include stable message IDs.
		if turnID == "" && providerMessageID == "" && (createdAt.IsZero() || block.CreatedAt.IsZero()) && isRecentTranscriptMessageIndex(i, len(blocks)) {
			return true
		}
	}
	return false
}

func normalizeTranscriptMessageText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.TrimSpace(text)
}

func isRecentTranscriptMessageIndex(index, total int) bool {
	if index < 0 || total <= 0 || index >= total {
		return false
	}
	return total-index <= 3
}
