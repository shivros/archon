package app

import (
	"strings"
	"time"
	"unicode"
)

type assistantAppendContext struct {
	createdAt         time.Time
	turnID            string
	providerMessageID string
	forceSplit        bool
}

type assistantMergePolicy interface {
	ShouldMerge(last ChatBlock, nextText string, ctx assistantAppendContext) bool
}

type defaultAssistantMergePolicy struct{}

func (defaultAssistantMergePolicy) ShouldMerge(last ChatBlock, nextText string, ctx assistantAppendContext) bool {
	if last.Role != ChatRoleAgent {
		return false
	}
	nextText = strings.TrimSpace(nextText)
	if nextText == "" {
		return false
	}
	lastTurnID := strings.TrimSpace(last.TurnID)
	nextTurnID := strings.TrimSpace(ctx.turnID)
	if lastTurnID != "" && nextTurnID != "" && lastTurnID != nextTurnID {
		return false
	}
	lastMessageID := strings.TrimSpace(last.ProviderMessageID)
	nextMessageID := strings.TrimSpace(ctx.providerMessageID)
	if ctx.forceSplit {
		return strictAssistantContinuation(last.Text, nextText, last.CreatedAt, ctx.createdAt)
	}
	if lastMessageID != "" && nextMessageID != "" {
		return lastMessageID == nextMessageID
	}
	if lastMessageID != "" || nextMessageID != "" {
		return false
	}
	return strictAssistantContinuation(last.Text, nextText, last.CreatedAt, ctx.createdAt)
}

const (
	maxAssistantContinuationGap      = 1200 * time.Millisecond
	maxAssistantContinuationTextSize = 96
	maxAssistantCombinedTextSize     = 1200
)

func strictAssistantContinuation(currentText, nextText string, currentAt, nextAt time.Time) bool {
	currentTrimmed := strings.TrimSpace(currentText)
	nextTrimmed := strings.TrimSpace(nextText)
	if currentTrimmed == "" || nextTrimmed == "" {
		return false
	}
	if len([]rune(nextTrimmed)) > maxAssistantContinuationTextSize {
		return false
	}
	if len([]rune(currentTrimmed))+len([]rune(nextTrimmed)) > maxAssistantCombinedTextSize {
		return false
	}
	if strings.Contains(currentTrimmed, "\n\n") || strings.Contains(nextTrimmed, "\n\n") {
		return false
	}
	if currentAt.IsZero() || nextAt.IsZero() {
		return false
	}
	gap := nextAt.Sub(currentAt)
	if gap < 0 {
		gap = -gap
	}
	if gap > maxAssistantContinuationGap {
		return false
	}
	if hasExplicitAssistantBoundary(currentTrimmed, nextTrimmed) {
		return false
	}
	if !looksLikeAssistantFragment(currentTrimmed, nextTrimmed) {
		return false
	}
	return true
}

func hasExplicitAssistantBoundary(currentText, nextText string) bool {
	currentText = strings.TrimSpace(currentText)
	nextText = strings.TrimSpace(nextText)
	if currentText == "" || nextText == "" {
		return false
	}
	last := currentText[len(currentText)-1]
	if last != '.' && last != '!' && last != '?' {
		return false
	}
	first, ok := firstNonSpaceRune(nextText)
	if !ok {
		return false
	}
	return unicode.IsUpper(first)
}

func looksLikeAssistantFragment(currentText, nextText string) bool {
	currentText = strings.TrimSpace(currentText)
	nextText = strings.TrimSpace(nextText)
	if currentText == "" || nextText == "" {
		return false
	}
	first, ok := firstNonSpaceRune(nextText)
	if !ok {
		return false
	}
	if unicode.IsLower(first) || strings.ContainsRune(",.;:)]}", first) {
		return true
	}
	if strings.HasPrefix(nextText, "'") || strings.HasPrefix(nextText, "\"") {
		return true
	}
	lower := strings.ToLower(nextText)
	for _, prefix := range []string{"and ", "but ", "so ", "because ", "then ", "or "} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	last := currentText[len(currentText)-1]
	return strings.ContainsRune(",;:([{", rune(last))
}

func firstNonSpaceRune(text string) (rune, bool) {
	for _, r := range text {
		if unicode.IsSpace(r) {
			continue
		}
		return r, true
	}
	return 0, false
}
