package app

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
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

func TestReasoningPreviewTextBoundaries(t *testing.T) {
	tests := []struct {
		name             string
		text             string
		maxLines         int
		maxChars         int
		wantTruncated    bool
		wantPreview      string
		wantPreviewBytes int
	}{
		{
			name:          "empty text",
			text:          "",
			maxLines:      3,
			maxChars:      100,
			wantTruncated: false,
			wantPreview:   "",
		},
		{
			name:             "char truncation",
			text:             strings.Repeat("x", 300),
			maxLines:         10,
			maxChars:         280,
			wantTruncated:    true,
			wantPreviewBytes: 280,
		},
		{
			name:          "exactly three lines",
			text:          "one\ntwo\nthree",
			maxLines:      3,
			maxChars:      280,
			wantTruncated: false,
			wantPreview:   "one\ntwo\nthree",
		},
		{
			name:          "four lines",
			text:          "one\ntwo\nthree\nfour",
			maxLines:      3,
			maxChars:      280,
			wantTruncated: true,
			wantPreview:   "one\ntwo\nthree",
		},
		{
			name:             "exactly 280 chars",
			text:             strings.Repeat("x", 280),
			maxLines:         3,
			maxChars:         280,
			wantTruncated:    false,
			wantPreviewBytes: 280,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			preview, truncated := reasoningPreviewText(tc.text, tc.maxLines, tc.maxChars)
			if truncated != tc.wantTruncated {
				t.Fatalf("truncated = %v, want %v (preview=%q)", truncated, tc.wantTruncated, preview)
			}
			if tc.wantPreview != "" && preview != tc.wantPreview {
				t.Fatalf("preview = %q, want %q", preview, tc.wantPreview)
			}
			if tc.wantPreview == "" && tc.text == "" && preview != "" {
				t.Fatalf("empty input should return empty preview, got %q", preview)
			}
			if tc.wantPreviewBytes > 0 && len(preview) != tc.wantPreviewBytes {
				t.Fatalf("preview byte length = %d, want %d", len(preview), tc.wantPreviewBytes)
			}
		})
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

func TestRenderChatBlocksCustomMetaPrimaryLabelRendersTwoLines(t *testing.T) {
	now := time.Now().UTC()
	blocks := []ChatBlock{
		{
			ID:        "recents:ready:s1",
			Role:      ChatRoleAgent,
			Text:      "hello",
			CreatedAt: now.Add(-3 * time.Minute),
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
			Now:           now,
			MetaByBlockID: map[string]ChatBlockMetaPresentation{
				"recents:ready:s1": {
					PrimaryLabel: "Session Alpha • Workspace / feature/refactor",
					Label:        "Ready",
					Controls: []ChatMetaControl{
						{ID: recentsControlReply, Label: "[Reply]", Tone: ChatMetaControlToneCopy},
						{ID: recentsControlOpen, Label: "[Open]", Tone: ChatMetaControlTonePin},
					},
				},
			},
		},
	)
	plain := xansi.Strip(rendered)
	lines := strings.Split(plain, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least two rendered lines, got %q", plain)
	}
	primaryLine := ""
	secondaryLine := ""
	for _, line := range lines {
		if strings.Contains(line, "Session Alpha • Workspace / feature/refactor") {
			primaryLine = line
		}
		if strings.Contains(line, "[Reply]") && strings.Contains(line, "Ready") {
			secondaryLine = line
		}
	}
	if primaryLine == "" {
		t.Fatalf("expected primary metadata line in output, got %q", plain)
	}
	if strings.Contains(primaryLine, "[Reply]") || strings.Contains(primaryLine, "[Open]") {
		t.Fatalf("expected controls on secondary metadata line, got primary %q", primaryLine)
	}
	if secondaryLine == "" {
		t.Fatalf("expected secondary metadata line with controls, got %q", plain)
	}
	if !strings.Contains(secondaryLine, "3 minutes ago") {
		t.Fatalf("expected timestamp on secondary metadata line, got %q", secondaryLine)
	}
	if len(spans) != 1 || len(spans[0].MetaControls) != 2 {
		t.Fatalf("expected custom control hitboxes on two-line meta, got %#v", spans)
	}
	if spans[0].MetaControls[0].Line == spans[0].StartLine {
		t.Fatalf("expected control hitboxes on secondary metadata line, got span %#v", spans[0])
	}
}

func TestRenderChatBlocksReasoningShowsToggleControlAndHitbox(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleReasoning, Text: "line1\nline2\nline3\nline4\nline5", Collapsed: true},
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

func TestRenderChatBlocksShortReasoningCollapsedOmitsCollapsedHint(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleReasoning, Text: "short message", Collapsed: true},
	}
	rendered, spans := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if strings.Contains(plain, "... (collapsed, press e or use [Expand])") {
		t.Fatalf("short collapsed reasoning should not include collapsed hint text: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
}

func TestRenderChatBlocksExpandedShortReasoningOmitsCollapsedHint(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleReasoning, Text: "short", Collapsed: false},
	}
	rendered, spans := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if strings.Contains(plain, "... (collapsed, press e or use [Expand])") {
		t.Fatalf("expanded short reasoning should not include collapsed hint text: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
}

func TestRenderChatBlocksRendererContractCoordinatesStayWithinBounds(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	blocks := []ChatBlock{
		{ID: "u1", Role: ChatRoleUser, Text: "hello"},
		{ID: "a1", Role: ChatRoleAgent, Text: "world"},
		{ID: "r1", Role: ChatRoleReasoning, Text: "line1\nline2\nline3\nline4", Collapsed: true},
		{ID: "ap1", Role: ChatRoleApproval, Text: "approval required"},
		{ID: "n1", Role: ChatRoleSessionNote, Text: "note text"},
		{
			ID:   "notes-scope",
			Role: ChatRoleSystem,
			Text: "Notes\n\nScope: session s1\n\nFilters:\n[x] Workspace\n[ ] Worktree\n[x] Session",
		},
	}
	ctx := chatRenderContext{TimestampMode: ChatTimestampModeRelative, Now: now}

	t.Run("default renderer", func(t *testing.T) {
		rendered, spans := renderChatBlocksWithRendererAndContext(
			blocks,
			80,
			2000,
			-1,
			defaultChatBlockRenderer{},
			ctx,
		)
		assertRenderedSpansWithinBounds(t, rendered, spans)
	})

	t.Run("cached default renderer", func(t *testing.T) {
		rendered, spans := renderChatBlocksWithRendererAndContext(
			blocks,
			80,
			2000,
			-1,
			newCachedChatBlockRenderer(defaultChatBlockRenderer{}, newBlockRenderCache(64)),
			ctx,
		)
		assertRenderedSpansWithinBounds(t, rendered, spans)
	})
}

func TestRenderChatBlocksCachedDefaultRendererMatchesDefaultOutput(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	blocks := []ChatBlock{
		{ID: "u1", Role: ChatRoleUser, Text: "hello"},
		{ID: "r1", Role: ChatRoleReasoning, Text: "line1\nline2\nline3\nline4", Collapsed: true},
		{ID: "a1", Role: ChatRoleAgent, Text: "world", CreatedAt: now.Add(-2 * time.Minute)},
	}
	ctx := chatRenderContext{TimestampMode: ChatTimestampModeRelative, Now: now}

	defaultRendered, defaultSpans := renderChatBlocksWithRendererAndContext(
		blocks,
		80,
		2000,
		-1,
		defaultChatBlockRenderer{},
		ctx,
	)
	cachedRendered, cachedSpans := renderChatBlocksWithRendererAndContext(
		blocks,
		80,
		2000,
		-1,
		newCachedChatBlockRenderer(defaultChatBlockRenderer{}, newBlockRenderCache(64)),
		ctx,
	)
	if defaultRendered != cachedRendered {
		t.Fatalf("cached renderer output should match default renderer output")
	}
	if len(defaultSpans) != len(cachedSpans) {
		t.Fatalf("span count mismatch: default=%d cached=%d", len(defaultSpans), len(cachedSpans))
	}
	for i := range defaultSpans {
		if !reflect.DeepEqual(defaultSpans[i], cachedSpans[i]) {
			t.Fatalf("span mismatch at index %d: default=%#v cached=%#v", i, defaultSpans[i], cachedSpans[i])
		}
	}
}

func assertRenderedSpansWithinBounds(t *testing.T, rendered string, spans []renderedBlockSpan) {
	t.Helper()
	lines := strings.Split(rendered, "\n")
	maxLine := len(lines) - 1
	for i, span := range spans {
		if span.StartLine < 0 || span.EndLine < span.StartLine || span.EndLine > maxLine {
			t.Fatalf("span %d has invalid range [%d,%d] with maxLine %d: %#v", i, span.StartLine, span.EndLine, maxLine, span)
		}
		assertHitboxWithinSpan(t, i, "copy", span.CopyLine, span.CopyStart, span.CopyEnd, span)
		assertHitboxWithinSpan(t, i, "pin", span.PinLine, span.PinStart, span.PinEnd, span)
		assertHitboxWithinSpan(t, i, "move", span.MoveLine, span.MoveStart, span.MoveEnd, span)
		assertHitboxWithinSpan(t, i, "delete", span.DeleteLine, span.DeleteStart, span.DeleteEnd, span)
		assertHitboxWithinSpan(t, i, "toggle", span.ToggleLine, span.ToggleStart, span.ToggleEnd, span)
		assertHitboxWithinSpan(t, i, "approve", span.ApproveLine, span.ApproveStart, span.ApproveEnd, span)
		assertHitboxWithinSpan(t, i, "decline", span.DeclineLine, span.DeclineStart, span.DeclineEnd, span)
		assertHitboxWithinSpan(t, i, "workspace-filter", span.WorkspaceFilterLine, span.WorkspaceFilterStart, span.WorkspaceFilterEnd, span)
		assertHitboxWithinSpan(t, i, "worktree-filter", span.WorktreeFilterLine, span.WorktreeFilterStart, span.WorktreeFilterEnd, span)
		assertHitboxWithinSpan(t, i, "session-filter", span.SessionFilterLine, span.SessionFilterStart, span.SessionFilterEnd, span)
		for _, control := range span.MetaControls {
			if control.Line < span.StartLine || control.Line > span.EndLine {
				t.Fatalf("span %d meta control %q line %d out of range [%d,%d]: %#v", i, control.Label, control.Line, span.StartLine, span.EndLine, span)
			}
			if control.Start < 0 || control.End < control.Start {
				t.Fatalf("span %d meta control %q invalid range [%d,%d]: %#v", i, control.Label, control.Start, control.End, span)
			}
		}
	}
}

func assertHitboxWithinSpan(t *testing.T, spanIndex int, name string, line, start, end int, span renderedBlockSpan) {
	t.Helper()
	if line < 0 {
		return
	}
	if line < span.StartLine || line > span.EndLine {
		t.Fatalf("span %d %s line %d out of range [%d,%d]: %#v", spanIndex, name, line, span.StartLine, span.EndLine, span)
	}
	if start < 0 || end < start {
		t.Fatalf("span %d %s has invalid range [%d,%d]: %#v", spanIndex, name, start, end, span)
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

type mapChatBlockRenderer struct {
	byID       map[string]renderedChatBlock
	defaultOut renderedChatBlock
}

func (r mapChatBlockRenderer) RenderChatBlock(block ChatBlock, _ int, _ bool, _ chatRenderContext) renderedChatBlock {
	if out, ok := r.byID[block.ID]; ok {
		return out
	}
	return r.defaultOut
}

func TestRenderChatBlocksWithRendererAndContextHandlesEmptyBlocks(t *testing.T) {
	rendered, spans := renderChatBlocksWithRendererAndContext(nil, 80, 2000, -1, defaultChatBlockRenderer{}, chatRenderContext{})
	if rendered != "" {
		t.Fatalf("expected empty rendering, got %q", rendered)
	}
	if spans != nil {
		t.Fatalf("expected nil spans for empty blocks, got %#v", spans)
	}
}

func TestRenderChatBlocksWithRendererAndContextDefaultsWidthAndRenderer(t *testing.T) {
	blocks := []ChatBlock{{ID: "m1", Role: ChatRoleAgent, Text: "hello"}}
	rendered, spans := renderChatBlocksWithRendererAndContext(blocks, 0, 2000, -1, nil, chatRenderContext{TimestampMode: ChatTimestampModeRelative, Now: time.Now()})
	if strings.TrimSpace(xansi.Strip(rendered)) == "" {
		t.Fatalf("expected non-empty output with width/renderer defaults")
	}
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
}

func TestRenderChatBlocksWithRendererAndContextSkipsEmptyRenderedBlocks(t *testing.T) {
	renderer := mapChatBlockRenderer{
		byID: map[string]renderedChatBlock{
			"empty": {},
			"full":  {Lines: []string{"line"}},
		},
	}
	rendered, spans := renderChatBlocksWithRendererAndContext(
		[]ChatBlock{{ID: "empty"}, {ID: "full"}},
		80,
		2000,
		-1,
		renderer,
		chatRenderContext{},
	)
	if !strings.Contains(rendered, "line") {
		t.Fatalf("expected non-empty block to render, got %q", rendered)
	}
	if len(spans) != 1 || spans[0].ID != "full" {
		t.Fatalf("expected only non-empty rendered span, got %#v", spans)
	}
}

func TestRenderChatBlocksWithRendererAndContextDropsAndResetsOutOfWindowHitboxes(t *testing.T) {
	renderer := mapChatBlockRenderer{
		byID: map[string]renderedChatBlock{
			"drop": {
				Lines:                []string{"line-1", "line-2", "line-3"},
				CopyLine:             0,
				CopyStart:            1,
				CopyEnd:              2,
				PinLine:              0,
				PinStart:             1,
				PinEnd:               2,
				MoveLine:             0,
				MoveStart:            1,
				MoveEnd:              2,
				DeleteLine:           0,
				DeleteStart:          1,
				DeleteEnd:            2,
				ToggleLine:           0,
				ToggleStart:          1,
				ToggleEnd:            2,
				ApproveLine:          0,
				ApproveStart:         1,
				ApproveEnd:           2,
				DeclineLine:          0,
				DeclineStart:         1,
				DeclineEnd:           2,
				WorkspaceFilterLine:  0,
				WorkspaceFilterStart: 1,
				WorkspaceFilterEnd:   2,
				WorktreeFilterLine:   0,
				WorktreeFilterStart:  1,
				WorktreeFilterEnd:    2,
				SessionFilterLine:    0,
				SessionFilterStart:   1,
				SessionFilterEnd:     2,
				MetaControls: []renderedMetaControlHit{
					{ID: "a", Label: "[A]", Tone: ChatMetaControlToneCopy, Line: 0, Start: 1, End: 3},
				},
			},
		},
	}
	rendered, spans := renderChatBlocksWithRendererAndContext(
		[]ChatBlock{{ID: "drop", Role: ChatRoleAgent}},
		80,
		2,
		-1,
		renderer,
		chatRenderContext{},
	)
	if strings.TrimSpace(rendered) == "" {
		t.Fatalf("expected truncated lines to remain visible")
	}
	if len(spans) != 1 {
		t.Fatalf("expected one span after truncation, got %#v", spans)
	}
	span := spans[0]
	if span.StartLine != 0 || span.EndLine != 0 {
		t.Fatalf("expected span to be rebased into remaining window, got %#v", span)
	}
	if span.CopyLine >= 0 || span.PinLine >= 0 || span.MoveLine >= 0 || span.DeleteLine >= 0 ||
		span.ToggleLine >= 0 || span.ApproveLine >= 0 || span.DeclineLine >= 0 ||
		span.WorkspaceFilterLine >= 0 || span.WorktreeFilterLine >= 0 || span.SessionFilterLine >= 0 {
		t.Fatalf("expected dropped hitbox lines to reset after truncation, got %#v", span)
	}
	if len(span.MetaControls) != 0 {
		t.Fatalf("expected dropped meta controls to be pruned, got %#v", span.MetaControls)
	}
}

func TestRenderChatBlocksWithRendererAndContextPrunesHitboxesBeyondMaxRenderedLine(t *testing.T) {
	renderer := mapChatBlockRenderer{
		byID: map[string]renderedChatBlock{
			"prune": {
				Lines:                []string{"single"},
				CopyLine:             5,
				CopyStart:            1,
				CopyEnd:              2,
				PinLine:              5,
				PinStart:             1,
				PinEnd:               2,
				MoveLine:             5,
				MoveStart:            1,
				MoveEnd:              2,
				DeleteLine:           5,
				DeleteStart:          1,
				DeleteEnd:            2,
				ToggleLine:           5,
				ToggleStart:          1,
				ToggleEnd:            2,
				ApproveLine:          5,
				ApproveStart:         1,
				ApproveEnd:           2,
				DeclineLine:          5,
				DeclineStart:         1,
				DeclineEnd:           2,
				WorkspaceFilterLine:  5,
				WorkspaceFilterStart: 1,
				WorkspaceFilterEnd:   2,
				WorktreeFilterLine:   5,
				WorktreeFilterStart:  1,
				WorktreeFilterEnd:    2,
				SessionFilterLine:    5,
				SessionFilterStart:   1,
				SessionFilterEnd:     2,
				MetaControls: []renderedMetaControlHit{
					{ID: "a", Label: "[A]", Tone: ChatMetaControlToneCopy, Line: 5, Start: 1, End: 3},
				},
			},
		},
	}
	_, spans := renderChatBlocksWithRendererAndContext(
		[]ChatBlock{{ID: "prune", Role: ChatRoleAgent}},
		80,
		2000,
		-1,
		renderer,
		chatRenderContext{},
	)
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %#v", spans)
	}
	span := spans[0]
	if span.CopyLine >= 0 || span.PinLine >= 0 || span.MoveLine >= 0 || span.DeleteLine >= 0 ||
		span.ToggleLine >= 0 || span.ApproveLine >= 0 || span.DeclineLine >= 0 ||
		span.WorkspaceFilterLine >= 0 || span.WorktreeFilterLine >= 0 || span.SessionFilterLine >= 0 {
		t.Fatalf("expected out-of-bounds hitboxes to be pruned, got %#v", span)
	}
	if len(span.MetaControls) != 0 {
		t.Fatalf("expected out-of-bounds meta controls to be pruned, got %#v", span.MetaControls)
	}
}

func TestRenderCustomMetaControlCoversAllTones(t *testing.T) {
	label := "[X]"
	tests := []struct {
		name string
		tone ChatMetaControlTone
		want string
	}{
		{name: "copy", tone: ChatMetaControlToneCopy, want: copyButtonStyle.Render(label)},
		{name: "pin", tone: ChatMetaControlTonePin, want: pinButtonStyle.Render(label)},
		{name: "move", tone: ChatMetaControlToneMove, want: moveButtonStyle.Render(label)},
		{name: "delete", tone: ChatMetaControlToneDelete, want: deleteButtonStyle.Render(label)},
		{name: "approve", tone: ChatMetaControlToneApprove, want: approveButtonStyle.Render(label)},
		{name: "decline", tone: ChatMetaControlToneDecline, want: declineButtonStyle.Render(label)},
		{name: "notes filter off", tone: ChatMetaControlToneNotesFilterOff, want: notesFilterButtonOffStyle.Render(label)},
		{name: "default", tone: ChatMetaControlToneDefault, want: chatMetaStyle.Render(label)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderCustomMetaControl(label, tc.tone)
			if got != tc.want {
				t.Fatalf("renderCustomMetaControl(%q, %q) mismatch", label, tc.tone)
			}
		})
	}
}

func TestBuildCustomMetaControlHitsGuardsAndFallbackSearch(t *testing.T) {
	if hits := buildCustomMetaControlHits("A [Copy]", -1, []ChatMetaControl{{Label: "[Copy]"}}); hits != nil {
		t.Fatalf("expected nil hits for negative line, got %#v", hits)
	}
	if hits := buildCustomMetaControlHits("A [Copy]", 0, nil); hits != nil {
		t.Fatalf("expected nil hits for empty controls, got %#v", hits)
	}
	if hits := buildCustomMetaControlHits("   ", 0, []ChatMetaControl{{Label: "[Copy]"}}); hits != nil {
		t.Fatalf("expected nil hits for blank meta line, got %#v", hits)
	}

	controls := []ChatMetaControl{
		{ID: "b", Label: "[B]", Tone: ChatMetaControlTonePin},
		{ID: "empty", Label: "   ", Tone: ChatMetaControlToneCopy},
		{ID: "a", Label: "[A]", Tone: ChatMetaControlToneCopy},
		{ID: "missing", Label: "[Z]", Tone: ChatMetaControlToneCopy},
	}
	hits := buildCustomMetaControlHits("[A] [B]", 3, controls)
	if len(hits) != 2 {
		t.Fatalf("expected two hits from fallback + direct search, got %#v", hits)
	}
	if hits[0].Label != "[B]" || hits[0].Line != 3 {
		t.Fatalf("expected first hit for [B], got %#v", hits[0])
	}
	if hits[1].Label != "[A]" || hits[1].Line != 3 {
		t.Fatalf("expected second hit for [A] via fallback lookup, got %#v", hits[1])
	}
}

func TestMetaForBlockCases(t *testing.T) {
	tests := []struct {
		name     string
		ctx      chatRenderContext
		block    ChatBlock
		wantOK   bool
		wantMeta ChatBlockMetaPresentation
	}{
		{
			name:   "no map",
			ctx:    chatRenderContext{},
			block:  ChatBlock{ID: "x"},
			wantOK: false,
		},
		{
			name: "blank id",
			ctx: chatRenderContext{
				MetaByBlockID: map[string]ChatBlockMetaPresentation{"x": {Label: "ok"}},
			},
			block:  ChatBlock{ID: "   "},
			wantOK: false,
		},
		{
			name: "missing id",
			ctx: chatRenderContext{
				MetaByBlockID: map[string]ChatBlockMetaPresentation{"x": {Label: "ok"}},
			},
			block:  ChatBlock{ID: "y"},
			wantOK: false,
		},
		{
			name: "found id",
			ctx: chatRenderContext{
				MetaByBlockID: map[string]ChatBlockMetaPresentation{"x": {Label: "ok"}},
			},
			block:    ChatBlock{ID: "x"},
			wantOK:   true,
			wantMeta: ChatBlockMetaPresentation{Label: "ok"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta, ok := tc.ctx.metaForBlock(tc.block)
			if ok != tc.wantOK {
				t.Fatalf("metaForBlock ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && !reflect.DeepEqual(meta, tc.wantMeta) {
				t.Fatalf("metaForBlock meta = %#v, want %#v", meta, tc.wantMeta)
			}
		})
	}
}

func TestComposeChatMetaLineBranches(t *testing.T) {
	tests := []struct {
		name             string
		width            int
		controlsPlain    string
		controlsDisplay  string
		timestampPlain   string
		timestampDisplay string
		controlsOnRight  bool
		assert           func(t *testing.T, plain string)
	}{
		{
			name:            "timestamp empty uses controls only",
			width:           20,
			controlsPlain:   "Controls",
			controlsDisplay: "Controls",
			timestampPlain:  "",
			assert: func(t *testing.T, plain string) {
				if !strings.Contains(plain, "Controls") {
					t.Fatalf("expected controls in line, got %q", plain)
				}
			},
		},
		{
			name:             "controls empty uses timestamp alignment",
			width:            20,
			controlsPlain:    "",
			controlsDisplay:  "",
			timestampPlain:   "2m ago",
			timestampDisplay: "2m ago",
			controlsOnRight:  true,
			assert: func(t *testing.T, plain string) {
				if !strings.Contains(plain, "2m ago") {
					t.Fatalf("expected timestamp in line, got %q", plain)
				}
			},
		},
		{
			name:             "overflow falls back to controls only",
			width:            10,
			controlsPlain:    "ABCDEFGHIJ",
			controlsDisplay:  "ABCDEFGHIJ",
			timestampPlain:   "TS",
			timestampDisplay: "TS",
			assert: func(t *testing.T, plain string) {
				if strings.Contains(plain, "TS") {
					t.Fatalf("expected overflow fallback without timestamp, got %q", plain)
				}
			},
		},
		{
			name:             "controls on right",
			width:            20,
			controlsPlain:    "CTRL",
			controlsDisplay:  "CTRL",
			timestampPlain:   "TIME",
			timestampDisplay: "TIME",
			controlsOnRight:  true,
			assert: func(t *testing.T, plain string) {
				if strings.Index(plain, "TIME") > strings.Index(plain, "CTRL") {
					t.Fatalf("expected timestamp before controls, got %q", plain)
				}
			},
		},
		{
			name:            "default width when non-positive",
			width:           0,
			controlsPlain:   "CTRL",
			controlsDisplay: "CTRL",
			timestampPlain:  "",
			assert: func(t *testing.T, plain string) {
				if len(plain) != 80 {
					t.Fatalf("expected default width 80, got %d", len(plain))
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line, plain := composeChatMetaLine(
				tc.width,
				lipgloss.Left,
				tc.controlsPlain,
				tc.controlsDisplay,
				tc.timestampPlain,
				tc.timestampDisplay,
				tc.controlsOnRight,
			)
			if xansi.Strip(line) != plain {
				t.Fatalf("plain output should match stripped line")
			}
			tc.assert(t, plain)
		})
	}
}

func TestShouldShowTimestampForBlockCases(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		block ChatBlock
		want  bool
	}{
		{name: "zero time", block: ChatBlock{Role: ChatRoleAgent}, want: false},
		{name: "note role", block: ChatBlock{Role: ChatRoleSessionNote, CreatedAt: now}, want: false},
		{name: "notes scope header", block: ChatBlock{ID: "notes-scope", Role: ChatRoleSystem, CreatedAt: now}, want: false},
		{name: "allowed role", block: ChatBlock{Role: ChatRoleApprovalResolved, CreatedAt: now}, want: true},
		{name: "unknown role", block: ChatBlock{Role: ChatRole("custom"), CreatedAt: now}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldShowTimestampForBlock(tc.block)
			if got != tc.want {
				t.Fatalf("shouldShowTimestampForBlock() = %v, want %v", got, tc.want)
			}
		})
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

func TestTrimLeadingRenderedBlankLinesHandlesEmptyAndAllBlank(t *testing.T) {
	if got := trimLeadingRenderedBlankLines(""); got != "" {
		t.Fatalf("expected empty input to remain empty, got %q", got)
	}
	allBlank := " \n\t\n"
	if got := trimLeadingRenderedBlankLines(allBlank); got != "" {
		t.Fatalf("expected all-blank rendered text to trim to empty, got %q", got)
	}
}
