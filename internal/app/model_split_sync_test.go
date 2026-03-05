package app

import "testing"

func TestSyncLayoutVersionNoopWhenCurrent(t *testing.T) {
	m := NewModel(nil)
	m.appState.LayoutVersion = splitLayoutVersion
	if changed := m.syncLayoutVersion(); changed {
		t.Fatalf("expected no change when layout version already current")
	}
}

func TestSyncLayoutVersionUpdatesWhenOutdated(t *testing.T) {
	m := NewModel(nil)
	m.appState.LayoutVersion = 0
	if changed := m.syncLayoutVersion(); !changed {
		t.Fatalf("expected layout version update")
	}
	if m.appState.LayoutVersion != splitLayoutVersion {
		t.Fatalf("expected layout version %d, got %d", splitLayoutVersion, m.appState.LayoutVersion)
	}
}

func TestSyncSidebarSplitNoopWhenCollapsedOrNoWidth(t *testing.T) {
	m := NewModel(nil)
	m.appState.SidebarCollapsed = true
	m.width = 180
	if changed := m.syncSidebarSplit(); changed {
		t.Fatalf("expected no change when sidebar collapsed")
	}

	m.appState.SidebarCollapsed = false
	m.width = 0
	if changed := m.syncSidebarSplit(); changed {
		t.Fatalf("expected no change when model width is zero")
	}
}

func TestSyncSidebarSplitNoopWhenUnchanged(t *testing.T) {
	m := NewModel(nil)
	m.width = 180
	m.height = 40
	width := m.sidebarWidth()
	m.appState.SidebarSplit = toAppStateSplit(captureSplitPreference(m.width, width, nil))
	if changed := m.syncSidebarSplit(); changed {
		t.Fatalf("expected no change when sidebar split already matches")
	}
}

func TestSyncActivePanelSplitNoopWhenNoVisiblePanel(t *testing.T) {
	m := NewModel(nil)
	if changed := m.syncActivePanelSplit(); changed {
		t.Fatalf("expected no change without active panel mode")
	}

	m.notesPanelOpen = true
	m.resize(40, 20)
	if m.notesPanelVisible {
		t.Fatalf("expected notes panel hidden at narrow width")
	}
	if changed := m.syncActivePanelSplit(); changed {
		t.Fatalf("expected no change when panel is not visible")
	}
}

func TestSyncActivePanelSplitUpdatesNotesSplitAndWidth(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(180, 40)
	if !m.notesPanelVisible {
		t.Fatalf("expected notes panel visible")
	}

	if changed := m.syncActivePanelSplit(); !changed {
		t.Fatalf("expected active panel split sync to update state")
	}
	if m.appState.MainSideSplit == nil || m.appState.MainSideSplit.Columns <= 0 {
		t.Fatalf("expected main/side split preference to be persisted")
	}
	if m.appState.NotesPanelWidth <= 0 {
		t.Fatalf("expected notes panel width to be persisted")
	}
}
