package app

import (
	"strings"
	"testing"

	"control/internal/types"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestMouseReducerLeftPressOutsideContextMenuCloses(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	if m.contextMenu == nil {
		t.Fatalf("expected context menu controller")
	}
	m.contextMenu.OpenSession("s1", "", "", "Session", 2, 2)

	handled := m.reduceContextMenuLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 119, Y: 39})
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

	handled := m.reduceContextMenuRightPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonRight, X: layout.rightStart, Y: 0}, layout)
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
	y := m.viewport.Height + 2

	handled := m.reduceInputFocusLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: layout.rightStart, Y: y}, layout)
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

	handled := m.reduceMouseWheel(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown, X: layout.rightStart, Y: 2}, layout, 1)
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

	if !m.reduceMouseWheel(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp, X: layout.rightStart, Y: 2}, layout, -1) {
		t.Fatalf("expected wheel up to be handled")
	}
	if m.follow {
		t.Fatalf("expected follow paused after wheel up")
	}

	if !m.reduceMouseWheel(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown, X: layout.rightStart, Y: 2}, layout, 1) {
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

	handled := m.reduceModePickersLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: layout.rightStart, Y: 1}, layout)
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
	y := span.CopyLine - m.viewport.YOffset + 1
	x := layout.rightStart + span.CopyStart

	handled := m.reduceTranscriptCopyLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}, layout)
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
	y := span.CopyLine - m.viewport.YOffset + 1
	x := layout.rightStart + span.CopyStart

	handled := m.reduceTranscriptCopyLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}, layout)
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
	y := span.PinLine - m.viewport.YOffset + 1
	x := layout.rightStart + span.PinStart

	handled := m.reduceTranscriptPinLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}, layout)
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
	y := span.DeleteLine - m.viewport.YOffset + 1
	x := layout.rightStart + span.DeleteStart

	handled := m.reduceTranscriptDeleteLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}, layout)
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
	y := span.ApproveLine - m.viewport.YOffset + 1
	x := layout.rightStart + span.ApproveStart

	handled := m.reduceTranscriptApprovalButtonLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}, layout)
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
	y := span.ApproveLine - m.viewport.YOffset + 1
	x := layout.rightStart + span.ApproveStart

	handled := m.reduceTranscriptApprovalButtonLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}, layout)
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
	y := first.CopyLine - m.viewport.YOffset + 2

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: layout.rightStart, Y: y}, layout)
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
	y = second.CopyLine - m.viewport.YOffset + 2
	handled = m.reduceTranscriptSelectLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: layout.rightStart, Y: y}, layout)
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
	bodyY := span.CopyLine - m.viewport.YOffset + 2

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: layout.rightStart, Y: bodyY}, layout)
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
	y := span.ToggleLine - m.viewport.YOffset + 1
	x := layout.rightStart + span.ToggleStart

	handled := m.reduceTranscriptReasoningButtonLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}, layout)
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
	y := span.CopyLine - m.viewport.YOffset + 1
	x := layout.rightStart

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}, layout)
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
	y := span.EndLine - m.viewport.YOffset + 1
	x := layout.rightStart

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y}, layout)
	if handled {
		t.Fatalf("expected status line click to avoid selection")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to remain inactive")
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
		tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: layout.rightStart, Y: row + 1},
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
		tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: layout.rightStart, Y: y},
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
