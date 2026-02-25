package app

import "testing"

func TestResolveMouseLayoutMatchesLayoutFrame(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(180, 40)

	frame := m.layoutFrame()
	layout := m.resolveMouseLayout()

	if layout.listWidth != frame.sidebarWidth {
		t.Fatalf("expected list width %d, got %d", frame.sidebarWidth, layout.listWidth)
	}
	if layout.rightStart != frame.rightStart {
		t.Fatalf("expected right start %d, got %d", frame.rightStart, layout.rightStart)
	}
	if layout.panelVisible != frame.panelVisible {
		t.Fatalf("expected panel visible %v, got %v", frame.panelVisible, layout.panelVisible)
	}
	if layout.panelWidth != frame.panelWidth {
		t.Fatalf("expected panel width %d, got %d", frame.panelWidth, layout.panelWidth)
	}
	if layout.panelStart != frame.panelStart {
		t.Fatalf("expected panel start %d, got %d", frame.panelStart, layout.panelStart)
	}
}

func TestLayoutFrameCollapsedSidebarStartsRightPaneAtZero(t *testing.T) {
	m := NewModel(nil)
	m.appState.SidebarCollapsed = true
	m.notesPanelOpen = true
	m.resize(180, 40)

	frame := m.layoutFrame()
	if frame.sidebarWidth != 0 {
		t.Fatalf("expected no sidebar width, got %d", frame.sidebarWidth)
	}
	if frame.rightStart != 0 {
		t.Fatalf("expected right pane to start at column 0, got %d", frame.rightStart)
	}
	if frame.panelVisible && frame.panelStart != m.notesPanelMainWidth+1 {
		t.Fatalf("unexpected panel start %d for main width %d", frame.panelStart, m.notesPanelMainWidth)
	}
}

func TestComputeSidebarWidthUsesSharedRule(t *testing.T) {
	if got := computeSidebarWidth(120, true); got != 0 {
		t.Fatalf("expected collapsed sidebar width 0, got %d", got)
	}
	if got := computeSidebarWidth(120, false); got <= 0 {
		t.Fatalf("expected expanded sidebar width > 0, got %d", got)
	}
}

func TestLayoutFrameUsesDebugPanelDimensionsWhenEnabled(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.appState.DebugStreamsEnabled = true
	m.resize(180, 40)

	if m.notesPanelVisible {
		t.Fatalf("expected notes panel to be hidden while debug panel is enabled")
	}
	if !m.debugPanelVisible {
		t.Fatalf("expected debug panel to be visible")
	}
	frame := m.layoutFrame()
	if !frame.panelVisible {
		t.Fatalf("expected panel visible in layout frame")
	}
	if frame.panelWidth != m.debugPanelWidth {
		t.Fatalf("expected debug panel width %d, got %d", m.debugPanelWidth, frame.panelWidth)
	}
	if frame.panelMain != m.debugPanelMainWidth {
		t.Fatalf("expected debug panel main width %d, got %d", m.debugPanelMainWidth, frame.panelMain)
	}
}
