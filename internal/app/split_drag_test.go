package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSidebarSplitDragUpdatesPreferenceAndQueuesSaveOnRelease(t *testing.T) {
	m := NewModel(nil)
	m.stateAPI = &appStateSyncStub{}
	m.resize(180, 40)
	layout := m.resolveMouseLayout()

	if !m.reduceSplitDragMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.listWidth, Y: 2}, layout) {
		t.Fatalf("expected sidebar divider click to start drag")
	}
	if m.splitDraggingTarget != splitDragTargetSidebar {
		t.Fatalf("expected sidebar drag target, got %v", m.splitDraggingTarget)
	}

	if !m.reduceSplitDragMouse(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 30, Y: 2}, layout) {
		t.Fatalf("expected sidebar drag motion to be handled")
	}
	if m.appState.SidebarSplit == nil || m.appState.SidebarSplit.Columns <= 0 {
		t.Fatalf("expected sidebar split preference to be updated")
	}

	if !m.reduceSplitDragMouse(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: 30, Y: 2}, layout) {
		t.Fatalf("expected sidebar drag release to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected app state save command after sidebar drag release")
	}
}

func TestPanelSplitDragUpdatesNotesWidthAndQueuesSaveOnRelease(t *testing.T) {
	m := NewModel(nil)
	m.stateAPI = &appStateSyncStub{}
	m.notesPanelOpen = true
	m.resize(180, 40)
	layout := m.resolveMouseLayout()
	if !layout.panelVisible {
		t.Fatalf("expected notes panel to be visible")
	}

	if !m.reduceSplitDragMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.panelStart - 1, Y: 2}, layout) {
		t.Fatalf("expected panel divider click to start drag")
	}

	targetX := layout.panelStart - 8
	if !m.reduceSplitDragMouse(tea.MouseMotionMsg{Button: tea.MouseLeft, X: targetX, Y: 2}, layout) {
		t.Fatalf("expected panel drag motion to be handled")
	}
	if m.appState.NotesPanelWidth <= 0 {
		t.Fatalf("expected notes panel width preference to be updated")
	}
	if m.appState.MainSideSplit == nil {
		t.Fatalf("expected main/side split preference to be updated")
	}

	if !m.reduceSplitDragMouse(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: targetX, Y: 2}, layout) {
		t.Fatalf("expected panel drag release to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected app state save command after panel drag release")
	}
}

func TestSplitDragIgnoresUnsupportedMouseEvents(t *testing.T) {
	m := NewModel(nil)
	m.resize(180, 40)
	layout := m.resolveMouseLayout()

	if handled := m.reduceSplitDragMouse(tea.MouseClickMsg{Button: tea.MouseRight, X: layout.listWidth, Y: 2}, layout); handled {
		t.Fatalf("expected non-left click to be ignored")
	}
	if handled := m.reduceSplitDragMouse(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 20, Y: 2}, layout); handled {
		t.Fatalf("expected motion without active drag to be ignored")
	}
	if handled := m.reduceSplitDragMouse(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: 20, Y: 2}, layout); handled {
		t.Fatalf("expected release without active drag to be ignored")
	}
}

func TestBeginSplitDragMissesDivider(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(180, 40)
	layout := m.resolveMouseLayout()

	if started := m.beginSplitDrag(tea.Mouse{X: layout.rightStart + 2, Y: 2, Button: tea.MouseLeft}, layout); started {
		t.Fatalf("expected click away from dividers not to start drag")
	}
}

func TestSplitDragReleaseWithoutWidthChangeDoesNotQueueSave(t *testing.T) {
	m := NewModel(nil)
	m.stateAPI = &appStateSyncStub{}
	m.resize(180, 40)
	layout := m.resolveMouseLayout()

	if !m.reduceSplitDragMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.listWidth, Y: 2}, layout) {
		t.Fatalf("expected divider click to start drag")
	}
	if !m.reduceSplitDragMouse(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: layout.listWidth, Y: 2}, layout) {
		t.Fatalf("expected release to end drag")
	}
	if m.pendingMouseCmd != nil {
		t.Fatalf("expected no save command when width did not change")
	}
}

func TestApplySidebarSplitWidthNoopGuards(t *testing.T) {
	m := NewModel(nil)
	m.width = 180
	m.appState.SidebarCollapsed = true
	if changed := m.applySidebarSplitWidth(30); changed {
		t.Fatalf("expected no change while sidebar collapsed")
	}

	m.appState.SidebarCollapsed = false
	m.appState.SidebarSplit = toAppStateSplit(captureSplitPreference(180, 30, nil))
	if changed := m.applySidebarSplitWidth(30); changed {
		t.Fatalf("expected no change when sidebar width is unchanged")
	}
}

func TestApplyPanelSplitWidthNoopGuards(t *testing.T) {
	m := NewModel(nil)
	if changed := m.applyPanelSplitWidth(sidePanelModeNone, 120, 30); changed {
		t.Fatalf("expected no change for none panel mode")
	}
	if changed := m.applyPanelSplitWidth(sidePanelModeNotes, 0, 30); changed {
		t.Fatalf("expected no change for zero viewport width")
	}
}
