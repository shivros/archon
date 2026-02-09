package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

type ChatRole string

const (
	ChatRoleUser             ChatRole = "user"
	ChatRoleAgent            ChatRole = "agent"
	ChatRoleSystem           ChatRole = "system"
	ChatRoleReasoning        ChatRole = "reasoning"
	ChatRoleApproval         ChatRole = "approval"
	ChatRoleApprovalResolved ChatRole = "approval_resolved"
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
	RequestID int
	SessionID string
}

type renderedBlockSpan struct {
	BlockIndex   int
	ID           string
	Role         ChatRole
	StartLine    int
	EndLine      int
	Collapsed    bool
	CopyLine     int
	CopyStart    int
	CopyEnd      int
	ToggleLine   int
	ToggleStart  int
	ToggleEnd    int
	ApproveLine  int
	ApproveStart int
	ApproveEnd   int
	DeclineLine  int
	DeclineStart int
	DeclineEnd   int
}

type renderedChatBlock struct {
	Lines        []string
	CopyLine     int
	CopyStart    int
	CopyEnd      int
	ToggleLine   int
	ToggleStart  int
	ToggleEnd    int
	ApproveLine  int
	ApproveStart int
	ApproveEnd   int
	DeclineLine  int
	DeclineStart int
	DeclineEnd   int
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
		toggleLine := -1
		toggleStart := -1
		toggleEnd := -1
		approveLine := -1
		approveStart := -1
		approveEnd := -1
		declineLine := -1
		declineStart := -1
		declineEnd := -1
		if rendered.CopyLine >= 0 {
			copyLine = start + rendered.CopyLine
			copyStart = rendered.CopyStart
			copyEnd = rendered.CopyEnd
		}
		if rendered.ToggleLine >= 0 {
			toggleLine = start + rendered.ToggleLine
			toggleStart = rendered.ToggleStart
			toggleEnd = rendered.ToggleEnd
		}
		if rendered.ApproveLine >= 0 {
			approveLine = start + rendered.ApproveLine
			approveStart = rendered.ApproveStart
			approveEnd = rendered.ApproveEnd
		}
		if rendered.DeclineLine >= 0 {
			declineLine = start + rendered.DeclineLine
			declineStart = rendered.DeclineStart
			declineEnd = rendered.DeclineEnd
		}
		spans = append(spans, renderedBlockSpan{
			BlockIndex:   i,
			ID:           block.ID,
			Role:         block.Role,
			StartLine:    start,
			EndLine:      end,
			Collapsed:    block.Collapsed,
			CopyLine:     copyLine,
			CopyStart:    copyStart,
			CopyEnd:      copyEnd,
			ToggleLine:   toggleLine,
			ToggleStart:  toggleStart,
			ToggleEnd:    toggleEnd,
			ApproveLine:  approveLine,
			ApproveStart: approveStart,
			ApproveEnd:   approveEnd,
			DeclineLine:  declineLine,
			DeclineStart: declineStart,
			DeclineEnd:   declineEnd,
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
				if span.ToggleLine >= 0 {
					span.ToggleLine -= drop
					if span.ToggleLine < 0 {
						span.ToggleLine = -1
						span.ToggleStart = -1
						span.ToggleEnd = -1
					}
				}
				if span.ApproveLine >= 0 {
					span.ApproveLine -= drop
					if span.ApproveLine < 0 {
						span.ApproveLine = -1
						span.ApproveStart = -1
						span.ApproveEnd = -1
					}
				}
				if span.DeclineLine >= 0 {
					span.DeclineLine -= drop
					if span.DeclineLine < 0 {
						span.DeclineLine = -1
						span.DeclineStart = -1
						span.DeclineEnd = -1
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
		if span.ToggleLine > maxLine {
			span.ToggleLine = -1
			span.ToggleStart = -1
			span.ToggleEnd = -1
		}
		if span.ApproveLine > maxLine {
			span.ApproveLine = -1
			span.ApproveStart = -1
			span.ApproveEnd = -1
		}
		if span.DeclineLine > maxLine {
			span.DeclineLine = -1
			span.DeclineStart = -1
			span.DeclineEnd = -1
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
			preview = preview + "\n\n... (collapsed, press e or use [Expand])"
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
	case ChatRoleApproval:
		bubbleStyle = approvalBubbleStyle
	case ChatRoleApprovalResolved:
		bubbleStyle = approvalResolvedBubbleStyle
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
	toggleLabel := ""
	approveLabel := ""
	declineLabel := ""
	if block.Role == ChatRoleReasoning {
		if block.Collapsed {
			toggleLabel = "[Expand]"
		} else {
			toggleLabel = "[Collapse]"
		}
	}
	if block.Role == ChatRoleApproval {
		approveLabel = "[Approve]"
		declineLabel = "[Decline]"
	}
	meta := roleLabel + " " + copyLabel
	if toggleLabel != "" {
		meta += " " + toggleLabel
	}
	if approveLabel != "" {
		meta += " " + approveLabel
	}
	if declineLabel != "" {
		meta += " " + declineLabel
	}
	if width > 0 {
		meta = truncateToWidth(meta, width)
	}
	metaStyle := chatMetaStyle
	if selected {
		metaStyle = chatMetaSelectedStyle
	}
	metaDisplay := metaStyle.Render(meta)
	if strings.HasPrefix(meta, roleLabel+" ") {
		parts := []string{metaStyle.Render(roleLabel + " ")}
		remaining := strings.TrimPrefix(meta, roleLabel+" ")
		if strings.HasPrefix(remaining, copyLabel) {
			parts = append(parts, copyButtonStyle.Render(copyLabel))
			remaining = strings.TrimPrefix(remaining, copyLabel)
		}
		if strings.HasPrefix(remaining, " ") {
			parts = append(parts, metaStyle.Render(" "))
			remaining = strings.TrimPrefix(remaining, " ")
		}
		if toggleLabel != "" && strings.HasPrefix(remaining, toggleLabel) {
			parts = append(parts, copyButtonStyle.Render(toggleLabel))
			remaining = strings.TrimPrefix(remaining, toggleLabel)
		}
		if strings.HasPrefix(remaining, " ") {
			parts = append(parts, metaStyle.Render(" "))
			remaining = strings.TrimPrefix(remaining, " ")
		}
		if approveLabel != "" && strings.HasPrefix(remaining, approveLabel) {
			parts = append(parts, approveButtonStyle.Render(approveLabel))
			remaining = strings.TrimPrefix(remaining, approveLabel)
		}
		if strings.HasPrefix(remaining, " ") {
			parts = append(parts, metaStyle.Render(" "))
			remaining = strings.TrimPrefix(remaining, " ")
		}
		if declineLabel != "" && strings.HasPrefix(remaining, declineLabel) {
			parts = append(parts, declineButtonStyle.Render(declineLabel))
			remaining = strings.TrimPrefix(remaining, declineLabel)
		}
		if remaining != "" {
			parts = append(parts, metaStyle.Render(remaining))
		}
		metaDisplay = strings.Join(parts, "")
	}
	metaLine := lipgloss.PlaceHorizontal(width, align, metaDisplay)
	metaPlain := xansi.Strip(metaLine)
	copyStart := -1
	copyEnd := -1
	if idx := strings.Index(metaPlain, copyLabel); idx >= 0 {
		copyStart = idx
		copyEnd = idx + len(copyLabel) - 1
	}
	toggleStart := -1
	toggleEnd := -1
	if toggleLabel != "" {
		if idx := strings.Index(metaPlain, toggleLabel); idx >= 0 {
			toggleStart = idx
			toggleEnd = idx + len(toggleLabel) - 1
		}
	}
	approveStart := -1
	approveEnd := -1
	if approveLabel != "" {
		if idx := strings.Index(metaPlain, approveLabel); idx >= 0 {
			approveStart = idx
			approveEnd = idx + len(approveLabel) - 1
		}
	}
	declineStart := -1
	declineEnd := -1
	if declineLabel != "" {
		if idx := strings.Index(metaPlain, declineLabel); idx >= 0 {
			declineStart = idx
			declineEnd = idx + len(declineLabel) - 1
		}
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
		Lines:        lines,
		CopyLine:     copyLine,
		CopyStart:    copyStart,
		CopyEnd:      copyEnd,
		ToggleLine:   copyLine,
		ToggleStart:  toggleStart,
		ToggleEnd:    toggleEnd,
		ApproveLine:  copyLine,
		ApproveStart: approveStart,
		ApproveEnd:   approveEnd,
		DeclineLine:  copyLine,
		DeclineStart: declineStart,
		DeclineEnd:   declineEnd,
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
	case ChatRoleApproval:
		return "Approval"
	case ChatRoleApprovalResolved:
		return "Approval"
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
