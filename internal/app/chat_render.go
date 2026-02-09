package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
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
	CopyLine   int
	CopyStart  int
	CopyEnd    int
}

type renderedChatBlock struct {
	Lines     []string
	CopyLine  int
	CopyStart int
	CopyEnd   int
}

const (
	reasoningPreviewLines = 3
	reasoningPreviewChars = 280
)

func renderChatBlocks(blocks []ChatBlock, width int, maxLines int) (string, []renderedBlockSpan) {
	return renderChatBlocksWithSelection(blocks, width, maxLines, -1)
}

func renderChatBlocksWithSelection(blocks []ChatBlock, width int, maxLines int, selectedBlockIndex int) (string, []renderedBlockSpan) {
	if len(blocks) == 0 {
		return "", nil
	}
	if width <= 0 {
		width = 80
	}
	lines := make([]string, 0, len(blocks)*4)
	spans := make([]renderedBlockSpan, 0, len(blocks))
	for i, block := range blocks {
		rendered := renderChatBlock(block, width, i == selectedBlockIndex)
		if len(rendered.Lines) == 0 {
			continue
		}
		start := len(lines)
		lines = append(lines, rendered.Lines...)
		end := len(lines) - 1
		copyLine := -1
		copyStart := -1
		copyEnd := -1
		if rendered.CopyLine >= 0 {
			copyLine = start + rendered.CopyLine
			copyStart = rendered.CopyStart
			copyEnd = rendered.CopyEnd
		}
		spans = append(spans, renderedBlockSpan{
			BlockIndex: i,
			ID:         block.ID,
			Role:       block.Role,
			StartLine:  start,
			EndLine:    end,
			Collapsed:  block.Collapsed,
			CopyLine:   copyLine,
			CopyStart:  copyStart,
			CopyEnd:    copyEnd,
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
				if span.CopyLine >= 0 {
					span.CopyLine -= drop
					if span.CopyLine < 0 {
						span.CopyLine = -1
						span.CopyStart = -1
						span.CopyEnd = -1
					}
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
		if span.CopyLine > maxLine {
			span.CopyLine = -1
			span.CopyStart = -1
			span.CopyEnd = -1
		}
		next = append(next, span)
	}
	return strings.Join(lines, "\n"), next
}

func renderChatBlock(block ChatBlock, width int, selected bool) renderedChatBlock {
	if width <= 0 {
		width = 80
	}
	text := strings.TrimSpace(block.Text)
	if text == "" && block.Status == ChatStatusNone {
		return renderedChatBlock{}
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
	if selected {
		bubbleStyle = bubbleStyle.Copy().BorderForeground(lipgloss.Color("117"))
	}
	roleLabel := chatRoleLabel(block.Role)
	copyLabel := "[Copy]"
	meta := roleLabel + " " + copyLabel
	if width > 0 {
		meta = truncateToWidth(meta, width)
	}
	metaStyle := chatMetaStyle
	if selected {
		metaStyle = chatMetaSelectedStyle
	}
	metaDisplay := metaStyle.Render(meta)
	if strings.Contains(meta, copyLabel) {
		metaDisplay = metaStyle.Render(roleLabel+" ") + copyButtonStyle.Render(copyLabel)
	}
	metaLine := lipgloss.PlaceHorizontal(width, align, metaDisplay)
	copyStart := -1
	copyEnd := -1
	if idx := strings.Index(xansi.Strip(metaLine), copyLabel); idx >= 0 {
		copyStart = idx
		copyEnd = idx + len(copyLabel) - 1
	}
	lines := []string{}
	if selected {
		marker := lipgloss.PlaceHorizontal(width, align, selectedMessageStyle.Render("▶ Selected"))
		lines = append(lines, marker)
	}
	bubble := bubbleStyle.Render(renderedText)
	placed := lipgloss.PlaceHorizontal(width, align, bubble)
	copyLine := len(lines)
	lines = append(lines, metaLine)
	lines = append(lines, strings.Split(placed, "\n")...)
	if block.Role == ChatRoleUser && block.Status != ChatStatusNone {
		status := "(sending…)"
		if block.Status == ChatStatusFailed {
			status = "(failed)"
		}
		statusLine := userStatusStyle.Render(status)
		lines = append(lines, lipgloss.PlaceHorizontal(width, lipgloss.Right, statusLine))
	}
	return renderedChatBlock{
		Lines:     lines,
		CopyLine:  copyLine,
		CopyStart: copyStart,
		CopyEnd:   copyEnd,
	}
}

func chatRoleLabel(role ChatRole) string {
	switch role {
	case ChatRoleUser:
		return "You"
	case ChatRoleAgent:
		return "Assistant"
	case ChatRoleReasoning:
		return "Reasoning"
	default:
		return "System"
	}
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
