package app

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestReasoningPreviewTextTruncates(t *testing.T) {
	text := "one\ntwo\nthree\nfour\nfive"
	preview, truncated := reasoningPreviewText(text, 3, 100)
	if !truncated {
		t.Fatalf("expected truncated preview")
	}
	if strings.Contains(preview, "four") {
		t.Fatalf("expected preview to truncate lines, got %q", preview)
	}
}

func TestRenderChatBlocksCollapsedReasoningShowsHint(t *testing.T) {
	blocks := []ChatBlock{
		{
			ID:        "reasoning-1",
			Role:      ChatRoleReasoning,
			Text:      "line1\nline2\nline3\nline4\nline5",
			Collapsed: true,
		},
	}
	rendered, _ := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "collapsed, press e or use [Expand]") {
		t.Fatalf("expected collapsed hint in rendered output: %q", plain)
	}
}

func TestRenderChatBlocksExpandedReasoningOmitsHint(t *testing.T) {
	blocks := []ChatBlock{
		{
			ID:        "reasoning-1",
			Role:      ChatRoleReasoning,
			Text:      "line1\nline2\nline3\nline4\nline5",
			Collapsed: false,
		},
	}
	rendered, _ := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if strings.Contains(plain, "collapsed, press e or use [Expand]") {
		t.Fatalf("did not expect collapsed hint in expanded output: %q", plain)
	}
}

func TestRenderChatBlocksShowsCopyControlPerMessage(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleAgent, Text: "hello"},
	}
	rendered, spans := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "[Copy]") {
		t.Fatalf("expected copy control in rendered output: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	if spans[0].CopyLine < 0 || spans[0].CopyStart < 0 || spans[0].CopyEnd < spans[0].CopyStart {
		t.Fatalf("expected copy hitbox metadata, got %#v", spans[0])
	}
}

func TestRenderChatBlocksAssistantTimestampAppearsOnRight(t *testing.T) {
	now := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	blocks := []ChatBlock{
		{
			Role:      ChatRoleAgent,
			Text:      "hello",
			CreatedAt: now.Add(-2 * time.Minute),
		},
	}
	rendered, _ := renderChatBlocksWithRendererAndContext(
		blocks,
		80,
		2000,
		-1,
		defaultChatBlockRenderer{},
		chatRenderContext{TimestampMode: ChatTimestampModeRelative, Now: now},
	)
	plain := xansi.Strip(rendered)
	lines := strings.Split(plain, "\n")
	if len(lines) == 0 {
		t.Fatalf("expected rendered lines")
	}
	meta := lines[0]
	if !strings.Contains(meta, "Assistant [Copy] [Pin]") || !strings.Contains(meta, "2 minutes ago") {
		t.Fatalf("expected controls and timestamp in assistant meta line: %q", meta)
	}
	if strings.Index(meta, "Assistant [Copy] [Pin]") > strings.Index(meta, "2 minutes ago") {
		t.Fatalf("expected assistant controls before timestamp: %q", meta)
	}
}

func TestRenderChatBlocksUserTimestampAppearsOnLeft(t *testing.T) {
	now := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	blocks := []ChatBlock{
		{
			Role:      ChatRoleUser,
			Text:      "hello",
			CreatedAt: now.Add(-3 * time.Minute),
		},
	}
	rendered, _ := renderChatBlocksWithRendererAndContext(
		blocks,
		80,
		2000,
		-1,
		defaultChatBlockRenderer{},
		chatRenderContext{TimestampMode: ChatTimestampModeRelative, Now: now},
	)
	plain := xansi.Strip(rendered)
	lines := strings.Split(plain, "\n")
	if len(lines) == 0 {
		t.Fatalf("expected rendered lines")
	}
	meta := lines[0]
	if !strings.Contains(meta, "3 minutes ago") || !strings.Contains(meta, "You [Copy] [Pin]") {
		t.Fatalf("expected timestamp and controls in user meta line: %q", meta)
	}
	if strings.Index(meta, "3 minutes ago") > strings.Index(meta, "You [Copy] [Pin]") {
		t.Fatalf("expected user timestamp before controls: %q", meta)
	}
}

func TestRenderChatBlocksCustomMetaControlsOverrideDefaults(t *testing.T) {
	blocks := []ChatBlock{
		{
			ID:   "recents:ready:s1",
			Role: ChatRoleAgent,
			Text: "hello",
		},
	}
	rendered, spans := renderChatBlocksWithRendererAndContext(
		blocks,
		80,
		2000,
		-1,
		defaultChatBlockRenderer{},
		chatRenderContext{
			TimestampMode: ChatTimestampModeRelative,
			Now:           time.Now(),
			MetaByBlockID: map[string]ChatBlockMetaPresentation{
				"recents:ready:s1": {
					Label: "Ready • Session Alpha",
					Controls: []ChatMetaControl{
						{ID: recentsControlReply, Label: "[Reply]", Tone: ChatMetaControlToneCopy},
						{ID: recentsControlOpen, Label: "[Open]", Tone: ChatMetaControlTonePin},
					},
				},
			},
		},
	)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "Ready • Session Alpha [Reply] [Open]") {
		t.Fatalf("expected custom controls in meta line, got %q", plain)
	}
	if strings.Contains(plain, "[Copy]") || strings.Contains(plain, "[Pin]") {
		t.Fatalf("expected default controls to be overridden, got %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	if spans[0].CopyStart >= 0 || spans[0].PinStart >= 0 {
		t.Fatalf("expected no default copy/pin hitboxes for custom controls, got %#v", spans[0])
	}
	if len(spans[0].MetaControls) != 2 {
		t.Fatalf("expected two custom control hitboxes, got %#v", spans[0].MetaControls)
	}
	if spans[0].MetaControls[0].Label != "[Reply]" || spans[0].MetaControls[0].Line < 0 {
		t.Fatalf("expected reply hitbox metadata, got %#v", spans[0].MetaControls[0])
	}
	if spans[0].MetaControls[0].ID != recentsControlReply {
		t.Fatalf("expected reply control id, got %#v", spans[0].MetaControls[0])
	}
	if spans[0].MetaControls[1].Label != "[Open]" || spans[0].MetaControls[1].Line < 0 {
		t.Fatalf("expected open hitbox metadata, got %#v", spans[0].MetaControls[1])
	}
	if spans[0].MetaControls[1].ID != recentsControlOpen {
		t.Fatalf("expected open control id, got %#v", spans[0].MetaControls[1])
	}
	lines := strings.Split(plain, "\n")
	metaLine := ""
	for _, line := range lines {
		if strings.Contains(line, "Ready • Session Alpha") {
			metaLine = line
			break
		}
	}
	if strings.TrimSpace(metaLine) == "" {
		t.Fatalf("expected meta line in rendered output, got %q", plain)
	}
	replyByteIndex := strings.Index(metaLine, "[Reply]")
	if replyByteIndex < 0 {
		t.Fatalf("expected [Reply] token in meta line %q", metaLine)
	}
	replyVisualStart := xansi.StringWidth(metaLine[:replyByteIndex])
	if spans[0].MetaControls[0].Start != replyVisualStart {
		t.Fatalf("expected visual-width hitbox start %d, got %d", replyVisualStart, spans[0].MetaControls[0].Start)
	}
}

func TestRenderChatBlocksReasoningShowsToggleControlAndHitbox(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleReasoning, Text: "hello", Collapsed: true},
	}
	rendered, spans := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "[Expand]") {
		t.Fatalf("expected expand control in rendered output: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	if spans[0].ToggleLine < 0 || spans[0].ToggleStart < 0 || spans[0].ToggleEnd < spans[0].ToggleStart {
		t.Fatalf("expected toggle hitbox metadata, got %#v", spans[0])
	}
}

func TestRenderChatBlocksApprovalShowsActionControlsAndHitboxes(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleApproval, Text: "approval required", RequestID: 0},
	}
	rendered, spans := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "[Approve]") {
		t.Fatalf("expected approve control in rendered output: %q", plain)
	}
	if !strings.Contains(plain, "[Decline]") {
		t.Fatalf("expected decline control in rendered output: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	if spans[0].ApproveLine < 0 || spans[0].ApproveStart < 0 || spans[0].ApproveEnd < spans[0].ApproveStart {
		t.Fatalf("expected approve hitbox metadata, got %#v", spans[0])
	}
	if spans[0].DeclineLine < 0 || spans[0].DeclineStart < 0 || spans[0].DeclineEnd < spans[0].DeclineStart {
		t.Fatalf("expected decline hitbox metadata, got %#v", spans[0])
	}
	if strings.Contains(plain, "Request ID:") {
		t.Fatalf("request id should not be rendered in approval message: %q", plain)
	}
}

func TestRenderChatBlocksResolvedApprovalHidesActionControls(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleApprovalResolved, Text: "approval approved", RequestID: 7},
	}
	rendered, spans := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if strings.Contains(plain, "[Approve]") || strings.Contains(plain, "[Decline]") {
		t.Fatalf("resolved approval should not render action controls: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	if spans[0].ApproveStart >= 0 || spans[0].DeclineStart >= 0 {
		t.Fatalf("resolved approval should not have action hitboxes, got %#v", spans[0])
	}
	if strings.Contains(plain, "Request ID:") {
		t.Fatalf("request id should not be rendered in resolved approval message: %q", plain)
	}
}

func TestRenderChatBlocksWithSelectionShowsSelectedMarker(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleAgent, Text: "hello"},
	}
	rendered, spans := renderChatBlocksWithSelection(blocks, 80, 2000, 0)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "Selected") {
		t.Fatalf("expected selected marker in rendered output: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	if spans[0].CopyLine <= spans[0].StartLine {
		t.Fatalf("expected copy line to account for selected marker, got span %#v", spans[0])
	}
}

func TestRenderChatBlocksNotesShowDeleteControlAndHitbox(t *testing.T) {
	blocks := []ChatBlock{
		{ID: "n1", Role: ChatRoleSessionNote, Text: "hello"},
	}
	rendered, spans := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "Session [Copy] [Move] [Delete]") {
		t.Fatalf("expected move/delete controls in rendered output: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	if spans[0].MoveLine < 0 || spans[0].MoveStart < 0 || spans[0].MoveEnd < spans[0].MoveStart {
		t.Fatalf("expected move hitbox metadata, got %#v", spans[0])
	}
	if spans[0].DeleteLine < 0 || spans[0].DeleteStart < 0 || spans[0].DeleteEnd < spans[0].DeleteStart {
		t.Fatalf("expected delete hitbox metadata, got %#v", spans[0])
	}
	if strings.Contains(plain, "[Pin]") {
		t.Fatalf("notes should not render pin control: %q", plain)
	}
}

func TestRenderChatBlocksTranscriptShowsPinControlAndHitbox(t *testing.T) {
	blocks := []ChatBlock{
		{ID: "m1", Role: ChatRoleAgent, Text: "hello"},
	}
	rendered, spans := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "Assistant [Copy] [Pin]") {
		t.Fatalf("expected pin control in rendered output: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	if spans[0].PinLine < 0 || spans[0].PinStart < 0 || spans[0].PinEnd < spans[0].PinStart {
		t.Fatalf("expected pin hitbox metadata, got %#v", spans[0])
	}
}

func TestRenderChatBlocksNotesScopeHeaderShowsFilterButtonsAndHitboxes(t *testing.T) {
	blocks := []ChatBlock{
		{
			ID:   "notes-scope",
			Role: ChatRoleSystem,
			Text: "Notes\n\nScope: session s1\n\nFilters:\n[x] Workspace\n[ ] Worktree\n[x] Session",
		},
	}
	rendered, spans := renderChatBlocks(blocks, 100, 2000)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "[Workspace]") || !strings.Contains(plain, "[Worktree]") || !strings.Contains(plain, "[Session]") {
		t.Fatalf("expected filter buttons in notes scope header: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	span := spans[0]
	if span.WorkspaceFilterLine < 0 || span.WorkspaceFilterStart < 0 || span.WorkspaceFilterEnd < span.WorkspaceFilterStart {
		t.Fatalf("expected workspace filter hitbox metadata, got %#v", span)
	}
	if span.WorktreeFilterLine < 0 || span.WorktreeFilterStart < 0 || span.WorktreeFilterEnd < span.WorktreeFilterStart {
		t.Fatalf("expected worktree filter hitbox metadata, got %#v", span)
	}
	if span.SessionFilterLine < 0 || span.SessionFilterStart < 0 || span.SessionFilterEnd < span.SessionFilterStart {
		t.Fatalf("expected session filter hitbox metadata, got %#v", span)
	}
}

func TestRenderChatTextUserStripsANSIStyling(t *testing.T) {
	got := renderChatText(ChatRoleUser, "**bold** and `code`", 80)
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected user chat text to omit ANSI sequences, got %q", got)
	}
}

func TestRenderChatTextReasoningTrimsLeadingNewlines(t *testing.T) {
	got := renderChatText(ChatRoleReasoning, "- first\n- second", 80)
	if strings.HasPrefix(got, "\n") || strings.HasPrefix(got, "\r\n") {
		t.Fatalf("expected reasoning chat text to trim leading newlines, got %q", got)
	}
}

func TestTrimLeadingRenderedBlankLinesDropsANSIOnlyLines(t *testing.T) {
	raw := "\x1b[38;5;245m\x1b[0m\n\x1b[38;5;245mhello\x1b[0m"
	got := trimLeadingRenderedBlankLines(raw)
	if strings.HasPrefix(got, "\n") {
		t.Fatalf("expected no leading newline, got %q", got)
	}
	if plain := xansi.Strip(got); plain != "hello" {
		t.Fatalf("expected trimmed content to keep visible text, got %q", plain)
	}
}
