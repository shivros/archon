package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type ChatRole string

const (
	ChatRoleUser      ChatRole = "user"
	ChatRoleAgent     ChatRole = "agent"
	ChatRoleSystem    ChatRole = "system"
	ChatRoleReasoning ChatRole = "reasoning"
)

type ChatStatus string

const (
	ChatStatusNone    ChatStatus = ""
	ChatStatusSending ChatStatus = "sending"
	ChatStatusFailed  ChatStatus = "failed"
)

type ChatBlock struct {
	ID        string
	Role      ChatRole
	Text      string
	Status    ChatStatus
	Collapsed bool
}

type renderedBlockSpan struct {
	BlockIndex int
	ID         string
	Role       ChatRole
	StartLine  int
	EndLine    int
	Collapsed  bool
}

const (
	reasoningPreviewLines = 3
	reasoningPreviewChars = 280
)

func renderChatBlocks(blocks []ChatBlock, width int, maxLines int) (string, []renderedBlockSpan) {
	if len(blocks) == 0 {
		return "", nil
	}
	if width <= 0 {
		width = 80
	}
	lines := make([]string, 0, len(blocks)*4)
	spans := make([]renderedBlockSpan, 0, len(blocks))
	for i, block := range blocks {
		blockLines := renderChatBlock(block, width)
		if len(blockLines) == 0 {
			continue
		}
		start := len(lines)
		lines = append(lines, blockLines...)
		end := len(lines) - 1
		spans = append(spans, renderedBlockSpan{
			BlockIndex: i,
			ID:         block.ID,
			Role:       block.Role,
			StartLine:  start,
			EndLine:    end,
			Collapsed:  block.Collapsed,
		})
		lines = append(lines, "")
		if maxLines > 0 && len(lines) > maxLines {
			drop := len(lines) - maxLines
			lines = lines[drop:]
			next := make([]renderedBlockSpan, 0, len(spans))
			for _, span := range spans {
				span.StartLine -= drop
				span.EndLine -= drop
				if span.EndLine < 0 {
					continue
				}
				if span.StartLine < 0 {
					span.StartLine = 0
				}
				next = append(next, span)
			}
			spans = next
		}
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	maxLine := len(lines) - 1
	next := make([]renderedBlockSpan, 0, len(spans))
	for _, span := range spans {
		if span.StartLine > maxLine {
			continue
		}
		if span.EndLine > maxLine {
			span.EndLine = maxLine
		}
		if span.EndLine < span.StartLine {
			continue
		}
		next = append(next, span)
	}
	return strings.Join(lines, "\n"), next
}

func renderChatBlock(block ChatBlock, width int) []string {
	if width <= 0 {
		width = 80
	}
	text := strings.TrimSpace(block.Text)
	if text == "" && block.Status == ChatStatusNone {
		return nil
	}
	maxBubbleWidth := width - 4
	if maxBubbleWidth < 10 {
		maxBubbleWidth = width
	}
	innerWidth := maxBubbleWidth - 2 - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	if block.Role == ChatRoleReasoning && block.Collapsed {
		preview, truncated := reasoningPreviewText(text, reasoningPreviewLines, reasoningPreviewChars)
		if truncated {
			preview = preview + "\n\n... (collapsed, press e or click to expand)"
		}
		text = preview
	}
	renderedText := renderMarkdown(text, innerWidth)
	var bubbleStyle lipgloss.Style
	align := lipgloss.Left
	switch block.Role {
	case ChatRoleUser:
		bubbleStyle = userBubbleStyle
		align = lipgloss.Right
	case ChatRoleReasoning:
		bubbleStyle = reasoningBubbleStyle
	default:
		if block.Role == ChatRoleAgent {
			bubbleStyle = agentBubbleStyle
		} else {
			bubbleStyle = systemBubbleStyle
		}
		align = lipgloss.Left
	}
	bubble := bubbleStyle.Render(renderedText)
	placed := lipgloss.PlaceHorizontal(width, align, bubble)
	lines := strings.Split(placed, "\n")
	if block.Role == ChatRoleUser && block.Status != ChatStatusNone {
		status := "(sending…)"
		if block.Status == ChatStatusFailed {
			status = "(failed)"
		}
		statusLine := userStatusStyle.Render(status)
		lines = append(lines, lipgloss.PlaceHorizontal(width, lipgloss.Right, statusLine))
	}
	return lines
}

func reasoningPreviewText(text string, maxLines int, maxChars int) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	lines := strings.Split(text, "\n")
	truncated := false
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	preview := strings.Join(lines, "\n")
	if maxChars > 0 && len(preview) > maxChars {
		preview = preview[:maxChars]
		truncated = true
	}
	return strings.TrimSpace(preview), truncated
}
