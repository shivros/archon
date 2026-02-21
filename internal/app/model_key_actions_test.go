package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestRenameHotkeyRoutesWorkspaceSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for entering rename")
	}
	if m.mode != uiModeRenameWorkspace {
		t.Fatalf("expected workspace rename mode, got %v", m.mode)
	}
	if m.renameWorkspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", m.renameWorkspaceID)
	}
}

func TestRenameHotkeyRoutesWorktreeSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree", Path: "/tmp/ws1/wt1"},
		},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorktree)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for entering rename")
	}
	if m.mode != uiModeRenameWorktree {
		t.Fatalf("expected worktree rename mode, got %v", m.mode)
	}
	if m.renameWorktreeID != "wt1" || m.renameWorktreeWorkspaceID != "ws1" {
		t.Fatalf("expected worktree target ws1/wt1, got %q/%q", m.renameWorktreeWorkspaceID, m.renameWorktreeID)
	}
}

func TestRenameHotkeyRoutesSessionSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for entering rename")
	}
	if m.mode != uiModeRenameSession {
		t.Fatalf("expected session rename mode, got %v", m.mode)
	}
	if m.renameSessionID != "s1" {
		t.Fatalf("expected session id s1, got %q", m.renameSessionID)
	}
}

func TestDeleteHotkeyRoutesWorkspaceSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for delete confirmation")
	}
	action, ok := m.pendingSelectionAction.(deleteWorkspaceSelectionAction)
	if !ok {
		t.Fatalf("expected workspace delete selection action, got %T", m.pendingSelectionAction)
	}
	if action.workspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", action.workspaceID)
	}
}

func TestDeleteHotkeyRoutesWorktreeSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree", Path: "/tmp/ws1/wt1"},
		},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorktree)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for delete confirmation")
	}
	action, ok := m.pendingSelectionAction.(deleteWorktreeSelectionAction)
	if !ok {
		t.Fatalf("expected worktree delete selection action, got %T", m.pendingSelectionAction)
	}
	if action.workspaceID != "ws1" || action.worktreeID != "wt1" {
		t.Fatalf("expected worktree target ws1/wt1, got %q/%q", action.workspaceID, action.worktreeID)
	}
}

func TestDeleteHotkeyRoutesSessionSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for dismiss confirmation")
	}
	action, ok := m.pendingSelectionAction.(dismissSessionSelectionAction)
	if !ok {
		t.Fatalf("expected session dismiss selection action, got %T", m.pendingSelectionAction)
	}
	if action.sessionID != "s1" {
		t.Fatalf("expected session target s1, got %q", action.sessionID)
	}
}

func TestDeleteHotkeyRoutesWorkflowSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, workflows, map[string]*types.SessionMeta{}, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkflow)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for dismiss confirmation")
	}
	action, ok := m.pendingSelectionAction.(dismissWorkflowSelectionAction)
	if !ok {
		t.Fatalf("expected workflow dismiss selection action, got %T", m.pendingSelectionAction)
	}
	if action.runID != "gwf-1" {
		t.Fatalf("expected workflow run id gwf-1, got %q", action.runID)
	}
}

func TestRenameHotkeyRequiresSelection(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when selection is missing")
	}
	if m.status != "select an item to rename" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestDeleteHotkeyRequiresSelection(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when selection is missing")
	}
	if m.status != "select an item to dismiss or delete" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMenuKeyUsesCtrlMCanonicalBinding(t *testing.T) {
	m := NewModel(nil)
	handled, _ := m.reduceMenuAndAppKeys(keyRune('m'))
	if handled {
		t.Fatalf("expected plain m to not trigger menu with default bindings")
	}

	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandMenu: "m",
	}))
	handled, _ = m.reduceMenuAndAppKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected menu override key to be handled")
	}
	if m.menu == nil || !m.menu.IsActive() {
		t.Fatalf("expected menu to open")
	}
}

func TestNotesNewOverrideWorksFromSidebarSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandNotesNew:   "ctrl+n",
		KeyCommandNewSession: "ctrl+s",
	}))

	handled, cmd := m.reduceNormalModeKey(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected notes-new override to be handled")
	}
	_ = cmd
	if m.mode != uiModeAddNote {
		t.Fatalf("expected add note mode, got %v", m.mode)
	}
	if m.notesScope.Scope != types.NoteScopeSession || m.notesScope.SessionID != "s1" {
		t.Fatalf("unexpected note scope: %#v", m.notesScope)
	}
}

func TestSpaceDoesNotEnableSessionMultiSelect(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{
		{ID: "s1", Title: "Session One", Status: types.SessionStatusExited},
		{ID: "s2", Title: "Session Two", Status: types.SessionStatusExited},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session One"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", Title: "Session Two"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd != nil {
		t.Fatalf("expected no command for space key in normal mode")
	}
	if got := m.selectedSessionID(); got != "s1" {
		t.Fatalf("expected focused session to remain unchanged, got %q", got)
	}
}

func TestEnterTogglesWorkspaceExpansion(t *testing.T) {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	selectSidebarItemKind(t, &m, sidebarWorkspace)

	handled, _ := m.reduceComposeAndWorkspaceEntryKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter key to be handled")
	}
	if len(m.sidebar.Items()) != 1 {
		t.Fatalf("expected workspace collapse to hide nested session, got %d rows", len(m.sidebar.Items()))
	}
	if expanded := m.appState.SidebarWorkspaceExpanded["ws1"]; expanded {
		t.Fatalf("expected persisted workspace expansion override ws1=false")
	}
}

func TestEnterTogglesWorkflowExpansion(t *testing.T) {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: "gwf-1"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, workflows, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkflow)

	handled, _ := m.reduceComposeAndWorkspaceEntryKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter key to be handled for workflow")
	}
	if len(m.sidebar.Items()) != 2 {
		t.Fatalf("expected workflow collapse to hide nested session, got %d rows", len(m.sidebar.Items()))
	}
	if expanded := m.appState.SidebarWorkflowExpanded["gwf-1"]; expanded {
		t.Fatalf("expected persisted workflow expansion override gwf-1=false")
	}
}

func TestWorkflowOpenShortcutUsesLowercaseO(t *testing.T) {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, workflows, map[string]*types.SessionMeta{}, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkflow)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('o'))
	if !handled {
		t.Fatalf("expected workflow open shortcut to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected workflow open shortcut to return snapshot command")
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.RunID() != "gwf-1" {
		t.Fatalf("expected selected guided workflow run gwf-1 to be opened")
	}
}

func TestSidebarArrowLeftRightCollapseAndExpandSelection(t *testing.T) {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	selectSidebarItemKind(t, &m, sidebarWorkspace)

	handled, _ := m.reduceSidebarArrowKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if !handled {
		t.Fatalf("expected left arrow to be handled")
	}
	if len(m.sidebar.Items()) != 1 {
		t.Fatalf("expected collapsed workspace list length 1, got %d", len(m.sidebar.Items()))
	}

	handled, _ = m.reduceSidebarArrowKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if !handled {
		t.Fatalf("expected right arrow to be handled")
	}
	if len(m.sidebar.Items()) != 2 {
		t.Fatalf("expected expanded workspace list length 2, got %d", len(m.sidebar.Items()))
	}
}

func TestSelectionHistoryHotkeysNavigateBackAndForward(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning},
		{ID: "s2", Status: types.SessionStatusRunning},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()

	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to select s1")
	}
	_ = m.onSystemSelectionChangedImmediate()
	if !m.sidebar.SelectBySessionID("s2") {
		t.Fatalf("expected to select s2")
	}
	_ = m.onSelectionChangedImmediate()

	handled, _ := m.reduceSelectionKeys(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	if !handled {
		t.Fatalf("expected alt+left to be handled")
	}
	if got := m.selectedSessionID(); got != "s1" {
		t.Fatalf("expected history back hotkey to select s1, got %q", got)
	}

	handled, _ = m.reduceSelectionKeys(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	if !handled {
		t.Fatalf("expected alt+right to be handled")
	}
	if got := m.selectedSessionID(); got != "s2" {
		t.Fatalf("expected history forward hotkey to select s2, got %q", got)
	}
}

func selectSidebarItemKind(t *testing.T, m *Model, kind sidebarItemKind) {
	t.Helper()
	if m == nil || m.sidebar == nil {
		t.Fatalf("missing sidebar")
	}
	for i, item := range m.sidebar.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil {
			continue
		}
		if entry.kind == kind {
			m.sidebar.Select(i)
			return
		}
	}
	t.Fatalf("did not find sidebar item kind %v", kind)
}

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: string(r)}
}
