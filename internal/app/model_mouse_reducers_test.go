package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMouseReducerLeftPressOutsideContextMenuCloses(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	if m.contextMenu == nil {
		t.Fatalf("expected context menu controller")
	}
	m.contextMenu.OpenSession("s1", "Session", 2, 2)

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
	m.contextMenu.OpenSession("s1", "Session", 2, 2)
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
	if m.status != "nothing to copy" {
		t.Fatalf("unexpected status %q", m.status)
	}
}
