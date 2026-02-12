package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"control/internal/types"
)

func TestRenameHotkeyRoutesWorkspaceSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, "", "", false)

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
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, "", "", false)
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
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, m.sessionMeta, "", "", false)
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
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, "", "", false)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for delete confirmation")
	}
	if m.pendingConfirm.kind != confirmDeleteWorkspace {
		t.Fatalf("expected workspace delete confirmation, got %v", m.pendingConfirm.kind)
	}
	if m.pendingConfirm.workspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", m.pendingConfirm.workspaceID)
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
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorktree)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for delete confirmation")
	}
	if m.pendingConfirm.kind != confirmDeleteWorktree {
		t.Fatalf("expected worktree delete confirmation, got %v", m.pendingConfirm.kind)
	}
	if m.pendingConfirm.workspaceID != "ws1" || m.pendingConfirm.worktreeID != "wt1" {
		t.Fatalf("expected worktree target ws1/wt1, got %q/%q", m.pendingConfirm.workspaceID, m.pendingConfirm.worktreeID)
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
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for dismiss confirmation")
	}
	if m.pendingConfirm.kind != confirmDismissSessions {
		t.Fatalf("expected dismiss confirmation, got %v", m.pendingConfirm.kind)
	}
	if len(m.pendingConfirm.sessionIDs) != 1 || m.pendingConfirm.sessionIDs[0] != "s1" {
		t.Fatalf("expected session target s1, got %#v", m.pendingConfirm.sessionIDs)
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
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandNotesNew:   "ctrl+n",
		KeyCommandNewSession: "ctrl+s",
	}))

	handled, cmd := m.reduceNormalModeKey(tea.KeyMsg{Type: tea.KeyCtrlN})
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
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd != nil {
		t.Fatalf("expected no command for space key in normal mode")
	}
	if got := m.selectedSessionID(); got != "s1" {
		t.Fatalf("expected focused session to remain unchanged, got %q", got)
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

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}
