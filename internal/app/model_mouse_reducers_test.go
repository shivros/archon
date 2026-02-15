package app

import (
	"strings"
	"testing"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestMouseReducerLeftPressOutsideContextMenuCloses(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	if m.contextMenu == nil {
		t.Fatalf("expected context menu controller")
	}
	m.contextMenu.OpenSession("s1", "", "", "Session", 2, 2)

	handled := m.reduceContextMenuLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 119, Y: 39})
	if handled {
		t.Fatalf("expected outside click to remain unhandled")
	}
	if m.contextMenu.IsOpen() {
		t.Fatalf("expected context menu to close on outside click")
	}
}

func TestMouseReducerRightPressClosesContextMenu(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	if m.contextMenu == nil {
		t.Fatalf("expected context menu controller")
	}
	m.contextMenu.OpenSession("s1", "", "", "Session", 2, 2)
	layout := m.resolveMouseLayout()

	handled := m.reduceContextMenuRightPressMouse(tea.MouseClickMsg{Button: tea.MouseRight, X: layout.rightStart, Y: 0}, layout)
	if !handled {
		t.Fatalf("expected right click to be handled")
	}
	if m.contextMenu.IsOpen() {
		t.Fatalf("expected context menu to close")
	}
}

func TestMouseReducerLeftPressInputFocusesComposeInput(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.enterCompose("s1")
	if m.chatInput == nil || m.input == nil {
		t.Fatalf("expected input controllers")
	}
	m.chatInput.Blur()
	m.input.FocusSidebar()
	layout := m.resolveMouseLayout()
	y := m.viewport.Height() + 2

	handled := m.reduceInputFocusLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y}, layout)
	if !handled {
		t.Fatalf("expected compose input click to be handled")
	}
	if !m.chatInput.Focused() {
		t.Fatalf("expected chat input to be focused")
	}
	if !m.input.IsChatFocused() {
		t.Fatalf("expected input controller focus to switch to chat")
	}
}

func TestMouseReducerWheelPausesFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.follow = true
	layout := m.resolveMouseLayout()

	handled := m.reduceMouseWheel(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: layout.rightStart, Y: 2}, layout, 1)
	if !handled {
		t.Fatalf("expected wheel event to be handled")
	}
	if m.follow {
		t.Fatalf("expected follow to pause after wheel scroll")
	}
	if m.status != "follow: paused" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerWheelDownToBottomResumesFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	seedFollowContent(&m, 220)
	layout := m.resolveMouseLayout()

	if !m.reduceMouseWheel(tea.MouseClickMsg{Button: tea.MouseWheelUp, X: layout.rightStart, Y: 2}, layout, -1) {
		t.Fatalf("expected wheel up to be handled")
	}
	if m.follow {
		t.Fatalf("expected follow paused after wheel up")
	}

	if !m.reduceMouseWheel(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: layout.rightStart, Y: 2}, layout, 1) {
		t.Fatalf("expected wheel down to be handled")
	}
	if !m.follow {
		t.Fatalf("expected follow to resume once wheel scroll reaches bottom")
	}
}

func TestMouseReducerPickProviderLeftClickSelects(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()
	layout := m.resolveMouseLayout()

	handled := m.reduceModePickersLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: 2}, layout)
	if !handled {
		t.Fatalf("expected provider click to be handled")
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode after provider pick, got %v", m.mode)
	}
	if m.newSession == nil || m.newSession.provider == "" {
		t.Fatalf("expected provider to be selected, got %#v", m.newSession)
	}
}

func TestMouseReducerTranscriptCopyClickHandlesPerMessage(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "   ", Status: ChatStatusSending},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.CopyLine < 0 || span.CopyStart < 0 {
		t.Fatalf("expected copy metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.CopyLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.CopyStart

	handled := m.reduceTranscriptCopyLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected copy click to be handled")
	}
	if m.messageSelectActive {
		t.Fatalf("expected copy click to avoid message selection")
	}
	if m.status != "nothing to copy" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerTranscriptCopyClickHandlesInNotesMode(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleSystem, Text: "   ", Status: ChatStatusSending},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.CopyLine < 0 || span.CopyStart < 0 {
		t.Fatalf("expected copy metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.CopyLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.CopyStart

	handled := m.reduceTranscriptCopyLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected copy click to be handled in notes mode")
	}
	if m.status != "nothing to copy" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerTranscriptPinClickQueuesPinCommand(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection")
	}
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{ID: "m1", Role: ChatRoleAgent, Text: "hello"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.PinLine < 0 || span.PinStart < 0 {
		t.Fatalf("expected pin metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.PinLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.PinStart

	handled := m.reduceTranscriptPinLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected pin click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected pin click to queue command")
	}
	if m.status != "pinning message" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerTranscriptMoveClickOpensMovePickerInNotesMode(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.worktrees["ws1"] = []*types.Worktree{{ID: "wt1", WorkspaceID: "ws1", Name: "feature"}}
	m.sessions = []*types.Session{
		{ID: "s2", Status: types.SessionStatusRunning},
	}
	m.sessionMeta["s2"] = &types.SessionMeta{SessionID: "s2", WorkspaceID: "ws1"}
	m.notes = []*types.Note{
		{
			ID:          "n1",
			Scope:       types.NoteScopeWorktree,
			WorkspaceID: "ws1",
			WorktreeID:  "wt1",
			Title:       "Move me",
		},
	}
	m.applyBlocks([]ChatBlock{
		{ID: "n1", Role: ChatRoleWorktreeNote, Text: "remember this"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.MoveLine < 0 || span.MoveStart < 0 {
		t.Fatalf("expected move metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.MoveLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.MoveStart

	handled := m.reduceTranscriptMoveLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected move click to be handled in notes mode")
	}
	if m.mode != uiModePickNoteMoveTarget {
		t.Fatalf("expected note move target picker mode, got %v", m.mode)
	}
	if m.noteMoveNoteID != "n1" {
		t.Fatalf("expected move state to track note n1, got %q", m.noteMoveNoteID)
	}
}

func TestMouseReducerTranscriptDeleteClickOpensConfirmInNotesMode(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.notes = []*types.Note{{ID: "n1", Title: "Important follow-up"}}
	m.applyBlocks([]ChatBlock{
		{ID: "n1", Role: ChatRoleSessionNote, Text: "remember this"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.DeleteLine < 0 || span.DeleteStart < 0 {
		t.Fatalf("expected delete metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.DeleteLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.DeleteStart

	handled := m.reduceTranscriptDeleteLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected delete click to be handled in notes mode")
	}
	if m.pendingConfirm.kind != confirmDeleteNote || m.pendingConfirm.noteID != "n1" {
		t.Fatalf("unexpected pending confirm: %#v", m.pendingConfirm)
	}
	if m.confirm == nil || !m.confirm.IsOpen() {
		t.Fatalf("expected confirm dialog to open")
	}
}

func TestMouseReducerTranscriptNotesFilterClickTogglesWorkspace(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.setNotesRootScope(noteScopeTarget{
		Scope:       types.NoteScopeSession,
		WorkspaceID: "ws1",
		WorktreeID:  "wt1",
		SessionID:   "s1",
	})
	m.renderNotesViewsFromState()
	lines := m.currentLines()
	targetLine := -1
	targetCol := -1
	for i, line := range lines {
		idx := strings.Index(line, "Workspace")
		if idx >= 0 {
			targetLine = i
			targetCol = idx
			break
		}
	}
	if targetLine < 0 {
		t.Fatalf("expected filter token in notes header")
	}
	layout := m.resolveMouseLayout()
	y := targetLine - m.viewport.YOffset() + 1
	x := layout.rightStart + targetCol
	handled := m.reduceTranscriptNotesFilterLeftPressMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      x,
		Y:      y,
	}, layout)
	if !handled {
		t.Fatalf("expected notes filter click to be handled")
	}
	if m.notesFilters.ShowWorkspace {
		t.Fatalf("expected workspace filter to toggle off")
	}
}

func TestMouseReducerApprovalButtonClickSendsApprovalForRequestZero(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection")
	}
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleApproval, Text: "Approval required", RequestID: 0, SessionID: "s1"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.ApproveLine < 0 || span.ApproveStart < 0 {
		t.Fatalf("expected approve metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.ApproveLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.ApproveStart

	handled := m.reduceTranscriptApprovalButtonLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected approval click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected approval click to queue approval command")
	}
	if m.status != "sending approval" {
		t.Fatalf("unexpected status %q", m.status)
	}
	if m.messageSelectActive {
		t.Fatalf("expected approval click to avoid message selection")
	}
}

func TestMouseReducerApprovalButtonUsesBlockSessionWhenSidebarIsWorkspace(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection")
	}
	items := m.sidebar.Items()
	workspaceIndex := -1
	for i, item := range items {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.kind != sidebarWorkspace {
			continue
		}
		workspaceIndex = i
		break
	}
	if workspaceIndex < 0 {
		t.Fatalf("expected workspace item in sidebar")
	}
	m.sidebar.Select(workspaceIndex)
	if m.selectedSessionID() != "" {
		t.Fatalf("expected no selected session after workspace selection")
	}
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleApproval, Text: "Approval required", RequestID: 3, SessionID: "s1"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	y := span.ApproveLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.ApproveStart

	handled := m.reduceTranscriptApprovalButtonLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected approval click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected approval click to queue approval command")
	}
	if m.status != "sending approval" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerTranscriptClickSelectsMessage(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "first"},
		{Role: ChatRoleAgent, Text: "second"},
	})
	if len(m.contentBlockSpans) < 2 {
		t.Fatalf("expected span metadata for messages")
	}
	first := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	y := first.CopyLine - m.viewport.YOffset() + 2

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y}, layout)
	if !handled {
		t.Fatalf("expected transcript click to be handled")
	}
	if !m.messageSelectActive {
		t.Fatalf("expected message selection to be active")
	}
	if m.messageSelectIndex != first.BlockIndex {
		t.Fatalf("expected selected index %d, got %d", first.BlockIndex, m.messageSelectIndex)
	}

	second := m.contentBlockSpans[1]
	y = second.CopyLine - m.viewport.YOffset() + 2
	handled = m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y}, layout)
	if !handled {
		t.Fatalf("expected second transcript click to be handled")
	}
	if m.messageSelectIndex != second.BlockIndex {
		t.Fatalf("expected selected index %d, got %d", second.BlockIndex, m.messageSelectIndex)
	}
}

func TestMouseReducerReasoningBodyClickSelectsWithoutToggle(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleReasoning, Text: "line one\nline two", Collapsed: true},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	bodyY := span.CopyLine - m.viewport.YOffset() + 2

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: bodyY}, layout)
	if !handled {
		t.Fatalf("expected body click to be handled")
	}
	if !m.messageSelectActive || m.messageSelectIndex != span.BlockIndex {
		t.Fatalf("expected reasoning message to be selected")
	}
	if !m.contentBlocks[span.BlockIndex].Collapsed {
		t.Fatalf("expected reasoning block to remain collapsed")
	}
}

func TestMouseReducerReasoningButtonClickToggles(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleReasoning, Text: "line one\nline two", Collapsed: true},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.ToggleLine < 0 || span.ToggleStart < 0 {
		t.Fatalf("expected reasoning toggle metadata, got %#v", span)
	}
	before := xansi.Strip(m.renderedText)
	if !strings.Contains(before, "[Expand]") {
		t.Fatalf("expected expand button before click, got %q", before)
	}
	layout := m.resolveMouseLayout()
	y := span.ToggleLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.ToggleStart

	handled := m.reduceTranscriptReasoningButtonLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected reasoning button click to be handled")
	}
	if m.contentBlocks[span.BlockIndex].Collapsed {
		t.Fatalf("expected reasoning block to expand")
	}
	if m.messageSelectActive {
		t.Fatalf("expected reasoning button click to avoid message selection")
	}
	after := xansi.Strip(m.renderedText)
	if !strings.Contains(after, "[Collapse]") {
		t.Fatalf("expected collapse button after click, got %q", after)
	}
}

func TestMouseReducerMetaLineClickDoesNotSelectMessage(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "first"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected span metadata for message")
	}
	span := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	y := span.CopyLine - m.viewport.YOffset() + 1
	x := layout.rightStart

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if handled {
		t.Fatalf("expected meta line click to avoid selection")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to remain inactive")
	}
}

func TestMouseReducerUserStatusLineClickDoesNotSelectMessage(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "sending", Status: ChatStatusSending},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected span metadata for message")
	}
	span := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	y := span.EndLine - m.viewport.YOffset() + 1
	x := layout.rightStart

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if handled {
		t.Fatalf("expected status line click to avoid selection")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to remain inactive")
	}
}

func TestMouseReducerGlobalStatusBarClickCopiesStatus(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	defer func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	}()

	copied := ""
	clipboardWriteAll = func(text string) error {
		copied = text
		return nil
	}
	clipboardWriteOSC52 = func(string) error {
		t.Fatalf("expected system clipboard copy to succeed without OSC52 fallback")
		return nil
	}

	m := NewModel(nil)
	m.resize(120, 40)
	m.hotkeys = nil
	m.setStatusMessage("follow: paused")
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "first"},
	})
	start, _, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected global status hitbox")
	}

	handled := m.handleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      start,
		Y:      m.height,
	})
	if !handled {
		t.Fatalf("expected global status click to be handled")
	}
	if copied != "follow: paused" {
		t.Fatalf("expected status copy payload %q, got %q", "follow: paused", copied)
	}
	if m.status != "status copied" {
		t.Fatalf("expected copy success status, got %q", m.status)
	}
	if m.messageSelectActive {
		t.Fatalf("expected status line copy click to avoid message selection")
	}
}

func TestMouseReducerGlobalStatusBarHelpClickDoesNotCopy(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	defer func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	}()

	copied := false
	clipboardWriteAll = func(string) error {
		copied = true
		return nil
	}
	clipboardWriteOSC52 = func(string) error {
		t.Fatalf("expected no clipboard fallback for non-copy click")
		return nil
	}

	m := NewModel(nil)
	m.resize(120, 40)
	m.hotkeys = nil
	m.setStatusMessage("ready")
	start, _, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected global status hitbox")
	}
	if start <= 0 {
		t.Fatalf("expected non-empty help segment before status, got status start %d", start)
	}

	handled := m.handleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      start - 1,
		Y:      m.height,
	})
	if handled {
		t.Fatalf("expected help-segment click to remain unhandled")
	}
	if copied {
		t.Fatalf("expected help-segment click not to invoke clipboard copy")
	}
	if m.status != "ready" {
		t.Fatalf("expected status to remain unchanged, got %q", m.status)
	}
}

func TestMouseReducerGlobalStatusHitboxVisibleWithDefaultHotkeys(t *testing.T) {
	m := NewModel(nil)
	m.resize(40, 20)
	m.setStatusMessage("ready")

	start, end, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected status hitbox with default hotkeys")
	}
	if end != m.width-1 {
		t.Fatalf("expected status to render at right edge %d, got %d", m.width-1, end)
	}
	if start < 0 || start > end {
		t.Fatalf("invalid status bounds [%d,%d]", start, end)
	}
}

func TestMouseReducerGlobalStatusBarClickCopiesStatusOnZeroBasedBottomRow(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	defer func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	}()

	copied := ""
	clipboardWriteAll = func(text string) error {
		copied = text
		return nil
	}
	clipboardWriteOSC52 = func(string) error {
		t.Fatalf("expected system clipboard copy to succeed without OSC52 fallback")
		return nil
	}

	m := NewModel(nil)
	m.resize(120, 40)
	m.hotkeys = nil
	m.setStatusMessage("ready")
	start, _, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected global status hitbox")
	}

	handled := m.handleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      start,
		Y:      m.height - 1,
	})
	if !handled {
		t.Fatalf("expected global status click on zero-based bottom row to be handled")
	}
	if copied != "ready" {
		t.Fatalf("expected status copy payload %q, got %q", "ready", copied)
	}
}

func TestMouseReducerGlobalStatusBarClickCopiesStatusWithOneBasedX(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	defer func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	}()

	copied := ""
	clipboardWriteAll = func(text string) error {
		copied = text
		return nil
	}
	clipboardWriteOSC52 = func(string) error {
		t.Fatalf("expected system clipboard copy to succeed without OSC52 fallback")
		return nil
	}

	m := NewModel(nil)
	m.resize(120, 40)
	m.hotkeys = nil
	m.setStatusMessage("ready")
	start, _, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected global status hitbox")
	}

	handled := m.handleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      start + 1,
		Y:      m.height - 1,
	})
	if !handled {
		t.Fatalf("expected global status click with one-based X to be handled")
	}
	if copied != "ready" {
		t.Fatalf("expected status copy payload %q, got %q", "ready", copied)
	}
}

func TestMouseReducerComposeOptionPickerClickSelectsOption(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	layout := m.resolveMouseLayout()
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected model option picker to open")
	}
	popup, row := m.composeOptionPopupView()
	if popup == "" {
		t.Fatalf("expected popup content")
	}

	handled := m.reduceComposeOptionPickerLeftPressMouse(
		tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: row + 2},
		layout,
	)
	if !handled {
		t.Fatalf("expected picker click to be handled")
	}
	if m.composeOptionPickerOpen() {
		t.Fatalf("expected picker to close after selection")
	}
	if m.newSession == nil || m.newSession.runtimeOptions == nil {
		t.Fatalf("expected runtime options to be updated")
	}
	if got := m.newSession.runtimeOptions.Model; got != "gpt-5.2-codex" {
		t.Fatalf("expected model gpt-5.2-codex, got %q", got)
	}
}

func TestMouseReducerComposeOptionPickerClickBelowPopupSelectsLastOption(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	layout := m.resolveMouseLayout()
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected model option picker to open")
	}
	popup, row := m.composeOptionPopupView()
	if popup == "" {
		t.Fatalf("expected popup content")
	}
	height := len(strings.Split(popup, "\n"))
	y := row + height

	handled := m.reduceComposeOptionPickerLeftPressMouse(
		tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y},
		layout,
	)
	if !handled {
		t.Fatalf("expected bottom-edge picker click to be handled")
	}
	if m.composeOptionPickerOpen() {
		t.Fatalf("expected picker to close after selection")
	}
	if m.newSession == nil || m.newSession.runtimeOptions == nil {
		t.Fatalf("expected runtime options to be updated")
	}
	if got := m.newSession.runtimeOptions.Model; got != "gpt-5.1-codex-max" {
		t.Fatalf("expected bottom-edge click to select last model, got %q", got)
	}
}
