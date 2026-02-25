package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

func TestSidebarSortStripLeftRightCyclesSortKeyForSessionSelection(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNormal
	m.appState.SidebarCollapsed = false
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	now := time.Now().UTC()
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", CreatedAt: now}}
	m.sessions = []*types.Session{{ID: "s1", CreatedAt: now, Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{"s1": {SessionID: "s1", WorkspaceID: "ws1"}}
	m.applySidebarItems()
	if m.sidebar == nil {
		t.Fatalf("expected sidebar")
	}
	if !m.sidebar.SelectByKey("sess:s1") {
		t.Fatalf("expected session selection")
	}
	m.sidebarSort = sidebarSortState{Key: sidebarSortKeyCreated}

	handled, _ := m.reduceSidebarSortStripKeys(tea.KeyPressMsg{Code: tea.KeyLeft})
	if !handled {
		t.Fatalf("expected left key handled for sort cycle")
	}
	if got := m.sidebarSort.Key; got != sidebarSortKeyActivity {
		t.Fatalf("expected activity after left cycle, got %q", got)
	}

	handled, _ = m.reduceSidebarSortStripKeys(tea.KeyPressMsg{Code: tea.KeyRight})
	if !handled {
		t.Fatalf("expected right key handled for sort cycle")
	}
	if got := m.sidebarSort.Key; got != sidebarSortKeyCreated {
		t.Fatalf("expected created after right cycle, got %q", got)
	}
}

func TestSidebarSortStripArrowDefersToContainerExpandCollapse(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNormal
	m.appState.SidebarCollapsed = false
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	now := time.Now().UTC()
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", CreatedAt: now}}
	m.sessions = []*types.Session{{ID: "s1", CreatedAt: now, Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{"s1": {SessionID: "s1", WorkspaceID: "ws1"}}
	m.applySidebarItems()
	if m.sidebar == nil {
		t.Fatalf("expected sidebar")
	}
	if !m.sidebar.SelectByKey("ws:ws1") {
		t.Fatalf("expected workspace selection")
	}
	handled, _ := m.reduceSidebarSortStripKeys(tea.KeyPressMsg{Code: tea.KeyLeft})
	if handled {
		t.Fatalf("expected left key to defer when collapsible container is selected")
	}
}

type hiddenSortStripVisibilityPolicy struct{}

func (hiddenSortStripVisibilityPolicy) ShowStrip(uiMode, bool) bool { return false }

func TestSidebarSortStripVisibilityPolicyCanDisableStripAvailability(t *testing.T) {
	m := NewModel(nil, WithSortStripVisibilityPolicy(hiddenSortStripVisibilityPolicy{}))
	m.mode = uiModeNormal
	m.appState.SidebarCollapsed = false
	if m.sidebarSortStripAvailable() {
		t.Fatalf("expected custom visibility policy to hide sort strip")
	}
}

func TestReduceSidebarFilterInputEditsQueryAndCloses(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNormal
	m.appState.SidebarCollapsed = false
	m.sidebarFilterActive = true
	if m.input != nil {
		m.input.FocusSidebar()
	}

	handled, _ := m.reduceSidebarFilterInput(tea.KeyPressMsg{Code: 'a'})
	if !handled || m.sidebarFilterQuery != "a" {
		t.Fatalf("expected single rune to append query, got handled=%v query=%q", handled, m.sidebarFilterQuery)
	}
	handled, _ = m.reduceSidebarFilterInput(tea.KeyPressMsg{Code: tea.KeySpace})
	if !handled || m.sidebarFilterQuery != "a " {
		t.Fatalf("expected space append, got handled=%v query=%q", handled, m.sidebarFilterQuery)
	}
	handled, _ = m.reduceSidebarFilterInput(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if !handled || m.sidebarFilterQuery != "a" {
		t.Fatalf("expected backspace remove, got handled=%v query=%q", handled, m.sidebarFilterQuery)
	}
	handled, _ = m.reduceSidebarFilterInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || m.sidebarFilterActive {
		t.Fatalf("expected enter to close filter mode")
	}

	m.sidebarFilterActive = true
	m.sidebarFilterQuery = "x"
	handled, _ = m.reduceSidebarFilterInput(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled || m.sidebarFilterActive || m.sidebarFilterQuery != "" {
		t.Fatalf("expected esc to clear and close filter mode")
	}
}

func TestReduceSidebarFilterInputIgnoresWhenSidebarNotFocused(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNormal
	m.appState.SidebarCollapsed = false
	m.sidebarFilterActive = true
	if m.input != nil {
		m.input.FocusChatInput()
	}
	handled, _ := m.reduceSidebarFilterInput(tea.KeyPressMsg{Code: 'a'})
	if handled {
		t.Fatalf("expected input to be ignored when sidebar is not focused")
	}
}

func TestToggleSidebarFilterTogglesAndClearsQueryOnClose(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNormal
	m.appState.SidebarCollapsed = false
	m.sidebarFilterQuery = " stale "
	_ = m.toggleSidebarFilter()
	if !m.sidebarFilterActive || m.sidebarFilterQuery != "stale" {
		t.Fatalf("expected opening filter to trim query, got active=%v query=%q", m.sidebarFilterActive, m.sidebarFilterQuery)
	}
	_ = m.toggleSidebarFilter()
	if m.sidebarFilterActive || m.sidebarFilterQuery != "" {
		t.Fatalf("expected closing filter to clear query, got active=%v query=%q", m.sidebarFilterActive, m.sidebarFilterQuery)
	}
}

func TestApplySidebarSortStripActionFilterDelegatesToToggle(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNormal
	m.appState.SidebarCollapsed = false
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if cmd := m.applySidebarSortStripAction(sidebarSortStripActionFilter); cmd != nil {
		t.Fatalf("expected filter action to be synchronous")
	}
	if !m.sidebarFilterActive {
		t.Fatalf("expected filter action to enable filter")
	}
}
