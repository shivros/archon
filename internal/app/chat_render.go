package app

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

type ChatRole string

const (
	ChatRoleUser             ChatRole = "user"
	ChatRoleAgent            ChatRole = "agent"
	ChatRoleSystem           ChatRole = "system"
	ChatRoleSessionNote      ChatRole = "session_note"
	ChatRoleWorkspaceNote    ChatRole = "workspace_note"
	ChatRoleWorktreeNote     ChatRole = "worktree_note"
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

type ChatMetaControlTone string

const (
	ChatMetaControlToneDefault        ChatMetaControlTone = ""
	ChatMetaControlToneCopy           ChatMetaControlTone = "copy"
	ChatMetaControlTonePin            ChatMetaControlTone = "pin"
	ChatMetaControlToneMove           ChatMetaControlTone = "move"
	ChatMetaControlToneDelete         ChatMetaControlTone = "delete"
	ChatMetaControlToneApprove        ChatMetaControlTone = "approve"
	ChatMetaControlToneDecline        ChatMetaControlTone = "decline"
	ChatMetaControlToneNotesFilterOff ChatMetaControlTone = "notes_filter_off"
)

type ChatMetaControlID string

type ChatMetaControl struct {
	ID    ChatMetaControlID
	Label string
	Tone  ChatMetaControlTone
}

type ChatBlockMetaPresentation struct {
	PrimaryLabel string
	Label        string
	Controls     []ChatMetaControl
}

type ChatBlock struct {
	ID        string
	Role      ChatRole
	Text      string
	Status    ChatStatus
	CreatedAt time.Time
	Collapsed bool
	RequestID int
	SessionID string
}

type renderedBlockSpan struct {
	BlockIndex           int
	ID                   string
	Role                 ChatRole
	StartLine            int
	EndLine              int
	Collapsed            bool
	CopyLine             int
	CopyStart            int
	CopyEnd              int
	PinLine              int
	PinStart             int
	PinEnd               int
	MoveLine             int
	MoveStart            int
	MoveEnd              int
	DeleteLine           int
	DeleteStart          int
	DeleteEnd            int
	ToggleLine           int
	ToggleStart          int
	ToggleEnd            int
	ApproveLine          int
	ApproveStart         int
	ApproveEnd           int
	DeclineLine          int
	DeclineStart         int
	DeclineEnd           int
	WorkspaceFilterLine  int
	WorkspaceFilterStart int
	WorkspaceFilterEnd   int
	WorktreeFilterLine   int
	WorktreeFilterStart  int
	WorktreeFilterEnd    int
	SessionFilterLine    int
	SessionFilterStart   int
	SessionFilterEnd     int
	MetaControls         []renderedMetaControlHit
}

type renderedMetaControlHit struct {
	ID    ChatMetaControlID
	Label string
	Tone  ChatMetaControlTone
	Line  int
	Start int
	End   int
}

type renderedChatBlock struct {
	Lines                []string
	CopyLine             int
	CopyStart            int
	CopyEnd              int
	PinLine              int
	PinStart             int
	PinEnd               int
	MoveLine             int
	MoveStart            int
	MoveEnd              int
	DeleteLine           int
	DeleteStart          int
	DeleteEnd            int
	ToggleLine           int
	ToggleStart          int
	ToggleEnd            int
	ApproveLine          int
	ApproveStart         int
	ApproveEnd           int
	DeclineLine          int
	DeclineStart         int
	DeclineEnd           int
	WorkspaceFilterLine  int
	WorkspaceFilterStart int
	WorkspaceFilterEnd   int
	WorktreeFilterLine   int
	WorktreeFilterStart  int
	WorktreeFilterEnd    int
	SessionFilterLine    int
	SessionFilterStart   int
	SessionFilterEnd     int
	MetaControls         []renderedMetaControlHit
}

type chatRenderContext struct {
	TimestampMode ChatTimestampMode
	Now           time.Time
	MetaByBlockID map[string]ChatBlockMetaPresentation
}

const (
	reasoningPreviewLines = 3
	reasoningPreviewChars = 280
)

func renderChatBlocks(blocks []ChatBlock, width int, maxLines int) (string, []renderedBlockSpan) {
	return renderChatBlocksWithSelection(blocks, width, maxLines, -1)
}

func renderChatBlocksWithSelection(blocks []ChatBlock, width int, maxLines int, selectedBlockIndex int) (string, []renderedBlockSpan) {
	return renderChatBlocksWithRendererAndContext(
		blocks,
		width,
		maxLines,
		selectedBlockIndex,
		defaultChatBlockRenderer{},
		chatRenderContext{TimestampMode: ChatTimestampModeRelative, Now: time.Now()},
	)
}

func renderChatBlocksWithRenderer(blocks []ChatBlock, width int, maxLines int, selectedBlockIndex int, renderer chatBlockRenderer) (string, []renderedBlockSpan) {
	return renderChatBlocksWithRendererAndContext(
		blocks,
		width,
		maxLines,
		selectedBlockIndex,
		renderer,
		chatRenderContext{TimestampMode: ChatTimestampModeRelative, Now: time.Now()},
	)
}

func renderChatBlocksWithRendererAndContext(blocks []ChatBlock, width int, maxLines int, selectedBlockIndex int, renderer chatBlockRenderer, ctx chatRenderContext) (string, []renderedBlockSpan) {
	if len(blocks) == 0 {
		return "", nil
	}
	if width <= 0 {
		width = 80
	}
	if renderer == nil {
		renderer = defaultChatBlockRenderer{}
	}
	lines := make([]string, 0, len(blocks)*4)
	spans := make([]renderedBlockSpan, 0, len(blocks))
	for i, block := range blocks {
		rendered := renderer.RenderChatBlock(block, width, i == selectedBlockIndex, ctx)
		if len(rendered.Lines) == 0 {
			continue
		}
		start := len(lines)
		lines = append(lines, rendered.Lines...)
		end := len(lines) - 1
		copyLine := -1
		copyStart := -1
		copyEnd := -1
		pinLine := -1
		pinStart := -1
		pinEnd := -1
		moveLine := -1
		moveStart := -1
		moveEnd := -1
		deleteLine := -1
		deleteStart := -1
		deleteEnd := -1
		toggleLine := -1
		toggleStart := -1
		toggleEnd := -1
		approveLine := -1
		approveStart := -1
		approveEnd := -1
		declineLine := -1
		declineStart := -1
		declineEnd := -1
		workspaceFilterLine := -1
		workspaceFilterStart := -1
		workspaceFilterEnd := -1
		worktreeFilterLine := -1
		worktreeFilterStart := -1
		worktreeFilterEnd := -1
		sessionFilterLine := -1
		sessionFilterStart := -1
		sessionFilterEnd := -1
		metaControls := make([]renderedMetaControlHit, 0, len(rendered.MetaControls))
		if rendered.CopyLine >= 0 {
			copyLine = start + rendered.CopyLine
			copyStart = rendered.CopyStart
			copyEnd = rendered.CopyEnd
		}
		if rendered.PinLine >= 0 {
			pinLine = start + rendered.PinLine
			pinStart = rendered.PinStart
			pinEnd = rendered.PinEnd
		}
		if rendered.MoveLine >= 0 {
			moveLine = start + rendered.MoveLine
			moveStart = rendered.MoveStart
			moveEnd = rendered.MoveEnd
		}
		if rendered.DeleteLine >= 0 {
			deleteLine = start + rendered.DeleteLine
			deleteStart = rendered.DeleteStart
			deleteEnd = rendered.DeleteEnd
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
		if rendered.WorkspaceFilterLine >= 0 {
			workspaceFilterLine = start + rendered.WorkspaceFilterLine
			workspaceFilterStart = rendered.WorkspaceFilterStart
			workspaceFilterEnd = rendered.WorkspaceFilterEnd
		}
		if rendered.WorktreeFilterLine >= 0 {
			worktreeFilterLine = start + rendered.WorktreeFilterLine
			worktreeFilterStart = rendered.WorktreeFilterStart
			worktreeFilterEnd = rendered.WorktreeFilterEnd
		}
		if rendered.SessionFilterLine >= 0 {
			sessionFilterLine = start + rendered.SessionFilterLine
			sessionFilterStart = rendered.SessionFilterStart
			sessionFilterEnd = rendered.SessionFilterEnd
		}
		for _, control := range rendered.MetaControls {
			if control.Line < 0 {
				continue
			}
			metaControls = append(metaControls, renderedMetaControlHit{
				ID:    control.ID,
				Label: strings.TrimSpace(control.Label),
				Tone:  control.Tone,
				Line:  start + control.Line,
				Start: control.Start,
				End:   control.End,
			})
		}
		spans = append(spans, renderedBlockSpan{
			BlockIndex:           i,
			ID:                   block.ID,
			Role:                 block.Role,
			StartLine:            start,
			EndLine:              end,
			Collapsed:            block.Collapsed,
			CopyLine:             copyLine,
			CopyStart:            copyStart,
			CopyEnd:              copyEnd,
			PinLine:              pinLine,
			PinStart:             pinStart,
			PinEnd:               pinEnd,
			MoveLine:             moveLine,
			MoveStart:            moveStart,
			MoveEnd:              moveEnd,
			DeleteLine:           deleteLine,
			DeleteStart:          deleteStart,
			DeleteEnd:            deleteEnd,
			ToggleLine:           toggleLine,
			ToggleStart:          toggleStart,
			ToggleEnd:            toggleEnd,
			ApproveLine:          approveLine,
			ApproveStart:         approveStart,
			ApproveEnd:           approveEnd,
			DeclineLine:          declineLine,
			DeclineStart:         declineStart,
			DeclineEnd:           declineEnd,
			WorkspaceFilterLine:  workspaceFilterLine,
			WorkspaceFilterStart: workspaceFilterStart,
			WorkspaceFilterEnd:   workspaceFilterEnd,
			WorktreeFilterLine:   worktreeFilterLine,
			WorktreeFilterStart:  worktreeFilterStart,
			WorktreeFilterEnd:    worktreeFilterEnd,
			SessionFilterLine:    sessionFilterLine,
			SessionFilterStart:   sessionFilterStart,
			SessionFilterEnd:     sessionFilterEnd,
			MetaControls:         metaControls,
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
				if span.PinLine >= 0 {
					span.PinLine -= drop
					if span.PinLine < 0 {
						span.PinLine = -1
						span.PinStart = -1
						span.PinEnd = -1
					}
				}
				if span.MoveLine >= 0 {
					span.MoveLine -= drop
					if span.MoveLine < 0 {
						span.MoveLine = -1
						span.MoveStart = -1
						span.MoveEnd = -1
					}
				}
				if span.DeleteLine >= 0 {
					span.DeleteLine -= drop
					if span.DeleteLine < 0 {
						span.DeleteLine = -1
						span.DeleteStart = -1
						span.DeleteEnd = -1
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
				if span.WorkspaceFilterLine >= 0 {
					span.WorkspaceFilterLine -= drop
					if span.WorkspaceFilterLine < 0 {
						span.WorkspaceFilterLine = -1
						span.WorkspaceFilterStart = -1
						span.WorkspaceFilterEnd = -1
					}
				}
				if span.WorktreeFilterLine >= 0 {
					span.WorktreeFilterLine -= drop
					if span.WorktreeFilterLine < 0 {
						span.WorktreeFilterLine = -1
						span.WorktreeFilterStart = -1
						span.WorktreeFilterEnd = -1
					}
				}
				if span.SessionFilterLine >= 0 {
					span.SessionFilterLine -= drop
					if span.SessionFilterLine < 0 {
						span.SessionFilterLine = -1
						span.SessionFilterStart = -1
						span.SessionFilterEnd = -1
					}
				}
				if len(span.MetaControls) > 0 {
					controls := make([]renderedMetaControlHit, 0, len(span.MetaControls))
					for _, control := range span.MetaControls {
						control.Line -= drop
						if control.Line < 0 {
							continue
						}
						controls = append(controls, control)
					}
					span.MetaControls = controls
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
		if span.PinLine > maxLine {
			span.PinLine = -1
			span.PinStart = -1
			span.PinEnd = -1
		}
		if span.MoveLine > maxLine {
			span.MoveLine = -1
			span.MoveStart = -1
			span.MoveEnd = -1
		}
		if span.DeleteLine > maxLine {
			span.DeleteLine = -1
			span.DeleteStart = -1
			span.DeleteEnd = -1
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
		if span.WorkspaceFilterLine > maxLine {
			span.WorkspaceFilterLine = -1
			span.WorkspaceFilterStart = -1
			span.WorkspaceFilterEnd = -1
		}
		if span.WorktreeFilterLine > maxLine {
			span.WorktreeFilterLine = -1
			span.WorktreeFilterStart = -1
			span.WorktreeFilterEnd = -1
		}
		if span.SessionFilterLine > maxLine {
			span.SessionFilterLine = -1
			span.SessionFilterStart = -1
			span.SessionFilterEnd = -1
		}
		if len(span.MetaControls) > 0 {
			controls := make([]renderedMetaControlHit, 0, len(span.MetaControls))
			for _, control := range span.MetaControls {
				if control.Line > maxLine {
					continue
				}
				controls = append(controls, control)
			}
			span.MetaControls = controls
		}
		next = append(next, span)
	}
	return strings.Join(lines, "\n"), next
}

func renderChatBlock(block ChatBlock, width int, selected bool, ctx chatRenderContext) renderedChatBlock {
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
	renderedText := renderChatText(block.Role, text, innerWidth)
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
	pinLabel := ""
	moveLabel := ""
	deleteLabel := ""
	toggleLabel := ""
	approveLabel := ""
	declineLabel := ""
	workspaceFilterLabel := ""
	worktreeFilterLabel := ""
	sessionFilterLabel := ""
	workspaceFilterOn := false
	worktreeFilterOn := false
	sessionFilterOn := false
	if shouldShowPinControl(block) {
		pinLabel = "[Pin]"
	}
	if isNoteRole(block.Role) {
		moveLabel = "[Move]"
		deleteLabel = "[Delete]"
	}
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
	if isNotesScopeHeaderBlock(block) {
		workspaceFilterOn = notesFilterEnabledFromText(text, "Workspace")
		worktreeFilterOn = notesFilterEnabledFromText(text, "Worktree")
		sessionFilterOn = notesFilterEnabledFromText(text, "Session")
		if notesFilterAvailableFromText(text, "Workspace") {
			workspaceFilterLabel = "[Workspace]"
		}
		if notesFilterAvailableFromText(text, "Worktree") {
			worktreeFilterLabel = "[Worktree]"
		}
		if notesFilterAvailableFromText(text, "Session") {
			sessionFilterLabel = "[Session]"
		}
	}
	metaStyle := chatMetaStyle
	if selected {
		metaStyle = chatMetaSelectedStyle
	}
	meta := ""
	metaDisplay := ""
	primaryMeta := ""
	primaryMetaDisplay := ""
	twoLineCustomMeta := false
	customControlDefs := make([]ChatMetaControl, 0)
	override, customMeta := ctx.metaForBlock(block)
	if customMeta {
		meta = strings.TrimSpace(override.Label)
		if meta == "" {
			meta = roleLabel
		}
		primaryMeta = strings.TrimSpace(override.PrimaryLabel)
		if primaryMeta != "" && width > 0 {
			primaryMeta = truncateToWidth(primaryMeta, width)
		}
		parts := []string{metaStyle.Render(meta)}
		for _, control := range override.Controls {
			label := strings.TrimSpace(control.Label)
			if label == "" {
				continue
			}
			customControlDefs = append(customControlDefs, ChatMetaControl{ID: control.ID, Label: label, Tone: control.Tone})
			meta += " " + label
			parts = append(parts, metaStyle.Render(" "), renderCustomMetaControl(label, control.Tone))
		}
		metaDisplay = strings.Join(parts, "")
		if primaryMeta != "" {
			primaryMetaDisplay = metaStyle.Render(primaryMeta)
			twoLineCustomMeta = true
		}
		copyLabel = ""
		pinLabel = ""
		moveLabel = ""
		deleteLabel = ""
		toggleLabel = ""
		approveLabel = ""
		declineLabel = ""
		workspaceFilterLabel = ""
		worktreeFilterLabel = ""
		sessionFilterLabel = ""
	} else {
		meta = roleLabel + " " + copyLabel
		if pinLabel != "" {
			meta += " " + pinLabel
		}
		if moveLabel != "" {
			meta += " " + moveLabel
		}
		if deleteLabel != "" {
			meta += " " + deleteLabel
		}
		if toggleLabel != "" {
			meta += " " + toggleLabel
		}
		if approveLabel != "" {
			meta += " " + approveLabel
		}
		if declineLabel != "" {
			meta += " " + declineLabel
		}
		if workspaceFilterLabel != "" {
			meta += " " + workspaceFilterLabel
		}
		if worktreeFilterLabel != "" {
			meta += " " + worktreeFilterLabel
		}
		if sessionFilterLabel != "" {
			meta += " " + sessionFilterLabel
		}
		if width > 0 {
			meta = truncateToWidth(meta, width)
		}
		metaDisplay = metaStyle.Render(meta)
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
			if pinLabel != "" && strings.HasPrefix(remaining, pinLabel) {
				parts = append(parts, pinButtonStyle.Render(pinLabel))
				remaining = strings.TrimPrefix(remaining, pinLabel)
			}
			if strings.HasPrefix(remaining, " ") {
				parts = append(parts, metaStyle.Render(" "))
				remaining = strings.TrimPrefix(remaining, " ")
			}
			if moveLabel != "" && strings.HasPrefix(remaining, moveLabel) {
				parts = append(parts, moveButtonStyle.Render(moveLabel))
				remaining = strings.TrimPrefix(remaining, moveLabel)
			}
			if strings.HasPrefix(remaining, " ") {
				parts = append(parts, metaStyle.Render(" "))
				remaining = strings.TrimPrefix(remaining, " ")
			}
			if deleteLabel != "" && strings.HasPrefix(remaining, deleteLabel) {
				parts = append(parts, deleteButtonStyle.Render(deleteLabel))
				remaining = strings.TrimPrefix(remaining, deleteLabel)
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
			if strings.HasPrefix(remaining, " ") {
				parts = append(parts, metaStyle.Render(" "))
				remaining = strings.TrimPrefix(remaining, " ")
			}
			if workspaceFilterLabel != "" && strings.HasPrefix(remaining, workspaceFilterLabel) {
				if workspaceFilterOn {
					parts = append(parts, copyButtonStyle.Render(workspaceFilterLabel))
				} else {
					parts = append(parts, notesFilterButtonOffStyle.Render(workspaceFilterLabel))
				}
				remaining = strings.TrimPrefix(remaining, workspaceFilterLabel)
			}
			if strings.HasPrefix(remaining, " ") {
				parts = append(parts, metaStyle.Render(" "))
				remaining = strings.TrimPrefix(remaining, " ")
			}
			if worktreeFilterLabel != "" && strings.HasPrefix(remaining, worktreeFilterLabel) {
				if worktreeFilterOn {
					parts = append(parts, copyButtonStyle.Render(worktreeFilterLabel))
				} else {
					parts = append(parts, notesFilterButtonOffStyle.Render(worktreeFilterLabel))
				}
				remaining = strings.TrimPrefix(remaining, worktreeFilterLabel)
			}
			if strings.HasPrefix(remaining, " ") {
				parts = append(parts, metaStyle.Render(" "))
				remaining = strings.TrimPrefix(remaining, " ")
			}
			if sessionFilterLabel != "" && strings.HasPrefix(remaining, sessionFilterLabel) {
				if sessionFilterOn {
					parts = append(parts, copyButtonStyle.Render(sessionFilterLabel))
				} else {
					parts = append(parts, notesFilterButtonOffStyle.Render(sessionFilterLabel))
				}
				remaining = strings.TrimPrefix(remaining, sessionFilterLabel)
			}
			if remaining != "" {
				parts = append(parts, metaStyle.Render(remaining))
			}
			metaDisplay = strings.Join(parts, "")
		}
	}
	timestampLabel := ""
	if shouldShowTimestampForBlock(block) {
		formatter := defaultChatTimestampFormatter{}
		timestampLabel = formatter.FormatTimestamp(block.CreatedAt, ctx.Now, normalizeChatTimestampMode(ctx.TimestampMode))
	}
	if width > 0 {
		timestampLabel = truncateToWidth(timestampLabel, width)
	}
	timestampDisplay := metaStyle.Render(timestampLabel)
	controlsOnRight := block.Role == ChatRoleUser
	metaLine, metaPlain := composeChatMetaLine(
		width,
		align,
		meta,
		metaDisplay,
		timestampLabel,
		timestampDisplay,
		controlsOnRight,
	)
	customControlHits := make([]renderedMetaControlHit, 0, len(customControlDefs))
	copyStart := -1
	copyEnd := -1
	if idx := strings.Index(metaPlain, copyLabel); idx >= 0 {
		copyStart = idx
		copyEnd = idx + len(copyLabel) - 1
	}
	pinStart := -1
	pinEnd := -1
	if pinLabel != "" {
		if idx := strings.Index(metaPlain, pinLabel); idx >= 0 {
			pinStart = idx
			pinEnd = idx + len(pinLabel) - 1
		}
	}
	moveStart := -1
	moveEnd := -1
	if moveLabel != "" {
		if idx := strings.Index(metaPlain, moveLabel); idx >= 0 {
			moveStart = idx
			moveEnd = idx + len(moveLabel) - 1
		}
	}
	deleteStart := -1
	deleteEnd := -1
	if deleteLabel != "" {
		if idx := strings.Index(metaPlain, deleteLabel); idx >= 0 {
			deleteStart = idx
			deleteEnd = idx + len(deleteLabel) - 1
		}
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
	workspaceFilterStart := -1
	workspaceFilterEnd := -1
	if workspaceFilterLabel != "" {
		if idx := strings.Index(metaPlain, workspaceFilterLabel); idx >= 0 {
			workspaceFilterStart = idx
			workspaceFilterEnd = idx + len(workspaceFilterLabel) - 1
		}
	}
	worktreeFilterStart := -1
	worktreeFilterEnd := -1
	if worktreeFilterLabel != "" {
		if idx := strings.Index(metaPlain, worktreeFilterLabel); idx >= 0 {
			worktreeFilterStart = idx
			worktreeFilterEnd = idx + len(worktreeFilterLabel) - 1
		}
	}
	sessionFilterStart := -1
	sessionFilterEnd := -1
	if sessionFilterLabel != "" {
		if idx := strings.Index(metaPlain, sessionFilterLabel); idx >= 0 {
			sessionFilterStart = idx
			sessionFilterEnd = idx + len(sessionFilterLabel) - 1
		}
	}
	lines := []string{}
	if selected {
		marker := lipgloss.PlaceHorizontal(width, align, selectedMessageStyle.Render("▶ Selected"))
		lines = append(lines, marker)
	}
	metaLineIndex := len(lines)
	if customMeta && twoLineCustomMeta {
		primaryLine := lipgloss.PlaceHorizontal(width, align, primaryMetaDisplay)
		lines = append(lines, primaryLine)
		metaLineIndex = len(lines)
	}
	bubble := bubbleStyle.Render(renderedText)
	placed := lipgloss.PlaceHorizontal(width, align, bubble)
	if customMeta && len(customControlDefs) > 0 {
		customControlHits = buildCustomMetaControlHits(metaPlain, metaLineIndex, customControlDefs)
	}
	lines = append(lines, metaLine)
	lines = append(lines, strings.Split(placed, "\n")...)
	copyLine := -1
	if copyStart >= 0 && copyEnd >= copyStart {
		copyLine = metaLineIndex
	}
	pinLine := -1
	if pinStart >= 0 && pinEnd >= pinStart {
		pinLine = metaLineIndex
	}
	moveLine := -1
	if moveStart >= 0 && moveEnd >= moveStart {
		moveLine = metaLineIndex
	}
	deleteLine := -1
	if deleteStart >= 0 && deleteEnd >= deleteStart {
		deleteLine = metaLineIndex
	}
	toggleLine := -1
	if toggleStart >= 0 && toggleEnd >= toggleStart {
		toggleLine = metaLineIndex
	}
	approveLine := -1
	if approveStart >= 0 && approveEnd >= approveStart {
		approveLine = metaLineIndex
	}
	declineLine := -1
	if declineStart >= 0 && declineEnd >= declineStart {
		declineLine = metaLineIndex
	}
	workspaceFilterLine := -1
	if workspaceFilterStart >= 0 && workspaceFilterEnd >= workspaceFilterStart {
		workspaceFilterLine = metaLineIndex
	}
	worktreeFilterLine := -1
	if worktreeFilterStart >= 0 && worktreeFilterEnd >= worktreeFilterStart {
		worktreeFilterLine = metaLineIndex
	}
	sessionFilterLine := -1
	if sessionFilterStart >= 0 && sessionFilterEnd >= sessionFilterStart {
		sessionFilterLine = metaLineIndex
	}
	if block.Role == ChatRoleUser && block.Status != ChatStatusNone {
		status := "(sending…)"
		if block.Status == ChatStatusFailed {
			status = "(failed)"
		}
		statusLine := userStatusStyle.Render(status)
		lines = append(lines, lipgloss.PlaceHorizontal(width, lipgloss.Right, statusLine))
	}
	return renderedChatBlock{
		Lines:                lines,
		CopyLine:             copyLine,
		CopyStart:            copyStart,
		CopyEnd:              copyEnd,
		PinLine:              pinLine,
		PinStart:             pinStart,
		PinEnd:               pinEnd,
		MoveLine:             moveLine,
		MoveStart:            moveStart,
		MoveEnd:              moveEnd,
		DeleteLine:           deleteLine,
		DeleteStart:          deleteStart,
		DeleteEnd:            deleteEnd,
		ToggleLine:           toggleLine,
		ToggleStart:          toggleStart,
		ToggleEnd:            toggleEnd,
		ApproveLine:          approveLine,
		ApproveStart:         approveStart,
		ApproveEnd:           approveEnd,
		DeclineLine:          declineLine,
		DeclineStart:         declineStart,
		DeclineEnd:           declineEnd,
		WorkspaceFilterLine:  workspaceFilterLine,
		WorkspaceFilterStart: workspaceFilterStart,
		WorkspaceFilterEnd:   workspaceFilterEnd,
		WorktreeFilterLine:   worktreeFilterLine,
		WorktreeFilterStart:  worktreeFilterStart,
		WorktreeFilterEnd:    worktreeFilterEnd,
		SessionFilterLine:    sessionFilterLine,
		SessionFilterStart:   sessionFilterStart,
		SessionFilterEnd:     sessionFilterEnd,
		MetaControls:         customControlHits,
	}
}

func renderCustomMetaControl(label string, tone ChatMetaControlTone) string {
	switch tone {
	case ChatMetaControlToneCopy:
		return copyButtonStyle.Render(label)
	case ChatMetaControlTonePin:
		return pinButtonStyle.Render(label)
	case ChatMetaControlToneMove:
		return moveButtonStyle.Render(label)
	case ChatMetaControlToneDelete:
		return deleteButtonStyle.Render(label)
	case ChatMetaControlToneApprove:
		return approveButtonStyle.Render(label)
	case ChatMetaControlToneDecline:
		return declineButtonStyle.Render(label)
	case ChatMetaControlToneNotesFilterOff:
		return notesFilterButtonOffStyle.Render(label)
	default:
		return chatMetaStyle.Render(label)
	}
}

func buildCustomMetaControlHits(metaPlain string, line int, controls []ChatMetaControl) []renderedMetaControlHit {
	if line < 0 || len(controls) == 0 || strings.TrimSpace(metaPlain) == "" {
		return nil
	}
	hits := make([]renderedMetaControlHit, 0, len(controls))
	searchFrom := 0
	for _, control := range controls {
		label := strings.TrimSpace(control.Label)
		if label == "" {
			continue
		}
		idx := strings.Index(metaPlain[searchFrom:], label)
		if idx >= 0 {
			idx += searchFrom
			searchFrom = idx + len(label)
		} else {
			idx = strings.Index(metaPlain, label)
		}
		if idx < 0 {
			continue
		}
		start := xansi.StringWidth(metaPlain[:idx])
		end := start + xansi.StringWidth(label) - 1
		if end < start {
			continue
		}
		hits = append(hits, renderedMetaControlHit{
			ID:    control.ID,
			Label: label,
			Tone:  control.Tone,
			Line:  line,
			Start: start,
			End:   end,
		})
	}
	return hits
}

func (ctx chatRenderContext) metaForBlock(block ChatBlock) (ChatBlockMetaPresentation, bool) {
	if len(ctx.MetaByBlockID) == 0 {
		return ChatBlockMetaPresentation{}, false
	}
	id := strings.TrimSpace(block.ID)
	if id == "" {
		return ChatBlockMetaPresentation{}, false
	}
	meta, ok := ctx.MetaByBlockID[id]
	if !ok {
		return ChatBlockMetaPresentation{}, false
	}
	return meta, true
}

func composeChatMetaLine(
	width int,
	align lipgloss.Position,
	controlsPlain string,
	controlsDisplay string,
	timestampPlain string,
	timestampDisplay string,
	controlsOnRight bool,
) (string, string) {
	if width <= 0 {
		width = 80
	}
	if strings.TrimSpace(timestampPlain) == "" {
		line := lipgloss.PlaceHorizontal(width, align, controlsDisplay)
		return line, xansi.Strip(line)
	}
	if strings.TrimSpace(controlsPlain) == "" {
		tsAlign := lipgloss.Right
		if controlsOnRight {
			tsAlign = lipgloss.Left
		}
		line := lipgloss.PlaceHorizontal(width, tsAlign, timestampDisplay)
		return line, xansi.Strip(line)
	}
	controlsWidth := xansi.StringWidth(controlsPlain)
	timestampWidth := xansi.StringWidth(timestampPlain)
	if controlsWidth+1+timestampWidth > width {
		line := lipgloss.PlaceHorizontal(width, align, controlsDisplay)
		return line, xansi.Strip(line)
	}
	spaces := strings.Repeat(" ", width-controlsWidth-timestampWidth)
	if controlsOnRight {
		return timestampDisplay + spaces + controlsDisplay, timestampPlain + spaces + controlsPlain
	}
	return controlsDisplay + spaces + timestampDisplay, controlsPlain + spaces + timestampPlain
}

func shouldShowTimestampForBlock(block ChatBlock) bool {
	if block.CreatedAt.IsZero() {
		return false
	}
	if isNoteRole(block.Role) {
		return false
	}
	if isNotesScopeHeaderBlock(block) {
		return false
	}
	switch block.Role {
	case ChatRoleUser, ChatRoleAgent, ChatRoleReasoning, ChatRoleSystem, ChatRoleApproval, ChatRoleApprovalResolved:
		return true
	default:
		return false
	}
}

func chatRoleLabel(role ChatRole) string {
	switch role {
	case ChatRoleUser:
		return "You"
	case ChatRoleAgent:
		return "Assistant"
	case ChatRoleSessionNote:
		return "Session"
	case ChatRoleWorkspaceNote:
		return "Workspace"
	case ChatRoleWorktreeNote:
		return "Worktree"
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

func isNoteRole(role ChatRole) bool {
	switch role {
	case ChatRoleSessionNote, ChatRoleWorkspaceNote, ChatRoleWorktreeNote:
		return true
	default:
		return false
	}
}

func shouldShowPinControl(block ChatBlock) bool {
	if isNoteRole(block.Role) {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(block.ID), "notes-") {
		return false
	}
	switch block.Role {
	case ChatRoleApproval, ChatRoleApprovalResolved:
		return false
	default:
		return true
	}
}

func isNotesScopeHeaderBlock(block ChatBlock) bool {
	id := strings.TrimSpace(block.ID)
	return id == "notes-scope" || id == "notes-panel-scope"
}

func notesFilterAvailableFromText(text, label string) bool {
	return strings.Contains(text, label)
}

func notesFilterEnabledFromText(text, label string) bool {
	return strings.Contains(text, "[x] "+label) || strings.Contains(text, label+" [x]")
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

func renderChatText(role ChatRole, text string, width int) string {
	rendered := renderMarkdown(text, width)
	if role == ChatRoleReasoning {
		return trimLeadingRenderedBlankLines(rendered)
	}
	if role == ChatRoleUser {
		// Keep markdown layout but remove ANSI styling so the user bubble background
		// remains uniform under the text glyphs.
		return xansi.Strip(rendered)
	}
	return rendered
}

func trimLeadingRenderedBlankLines(rendered string) string {
	if rendered == "" {
		return ""
	}
	lines := strings.Split(rendered, "\n")
	firstNonBlank := 0
	for firstNonBlank < len(lines) {
		if strings.TrimSpace(xansi.Strip(lines[firstNonBlank])) != "" {
			break
		}
		firstNonBlank++
	}
	if firstNonBlank <= 0 {
		return rendered
	}
	if firstNonBlank >= len(lines) {
		return ""
	}
	return strings.Join(lines[firstNonBlank:], "\n")
}
