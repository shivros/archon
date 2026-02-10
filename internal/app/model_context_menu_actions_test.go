package app

import (
	"testing"

	"control/internal/types"
)

func TestWorkspaceContextActionCreateEntersAddWorkspace(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorkspaceContextMenuAction(ContextMenuWorkspaceCreate, contextMenuTarget{})
	if !handled {
		t.Fatalf("expected workspace action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for workspace create action")
	}
	if m.mode != uiModeAddWorkspace {
		t.Fatalf("expected add workspace mode, got %v", m.mode)
	}
}

func TestWorkspaceContextActionDeleteRequiresSelection(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorkspaceContextMenuAction(ContextMenuWorkspaceDelete, contextMenuTarget{})
	if !handled {
		t.Fatalf("expected workspace action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when workspace is missing")
	}
	if m.status != "select a workspace to delete" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestWorkspaceContextActionAddWorktreeEntersMode(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"},
	}

	handled, cmd := m.handleWorkspaceContextMenuAction(ContextMenuWorkspaceAddWorktree, contextMenuTarget{id: "ws1"})
	if !handled {
		t.Fatalf("expected workspace action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for add worktree action")
	}
	if m.mode != uiModeAddWorktree {
		t.Fatalf("expected add worktree mode, got %v", m.mode)
	}
	if m.addWorktree == nil || m.addWorktree.workspaceID != "ws1" {
		t.Fatalf("expected add worktree to target workspace ws1")
	}
}

func TestWorkspaceContextActionAddWorktreeRequiresSelection(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorkspaceContextMenuAction(ContextMenuWorkspaceAddWorktree, contextMenuTarget{})
	if !handled {
		t.Fatalf("expected workspace action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when workspace is missing")
	}
	if m.status != "select a workspace" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestWorkspaceContextActionAddNoteEntersAddNoteMode(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorkspaceContextMenuAction(ContextMenuWorkspaceAddNote, contextMenuTarget{id: "ws1"})
	if !handled {
		t.Fatalf("expected workspace action to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected note prefetch command")
	}
	if m.mode != uiModeAddNote {
		t.Fatalf("expected add note mode, got %v", m.mode)
	}
	if m.notesScope.Scope != types.NoteScopeWorkspace || m.notesScope.WorkspaceID != "ws1" {
		t.Fatalf("unexpected notes scope: %#v", m.notesScope)
	}
}

func TestWorkspaceContextActionOpenNotesEntersNotesMode(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorkspaceContextMenuAction(ContextMenuWorkspaceOpenNotes, contextMenuTarget{id: "ws1"})
	if !handled {
		t.Fatalf("expected workspace action to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected notes fetch command")
	}
	if m.mode != uiModeNotes {
		t.Fatalf("expected notes mode, got %v", m.mode)
	}
	if m.notesScope.Scope != types.NoteScopeWorkspace || m.notesScope.WorkspaceID != "ws1" {
		t.Fatalf("unexpected notes scope: %#v", m.notesScope)
	}
}

func TestWorkspaceContextActionCopyPathRequiresSelection(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorkspaceContextMenuAction(ContextMenuWorkspaceCopyPath, contextMenuTarget{})
	if !handled {
		t.Fatalf("expected workspace action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when workspace is missing")
	}
	if m.status != "select a workspace" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestWorkspaceContextActionCopyPathUnavailable(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace", RepoPath: ""},
	}

	handled, cmd := m.handleWorkspaceContextMenuAction(ContextMenuWorkspaceCopyPath, contextMenuTarget{id: "ws1"})
	if !handled {
		t.Fatalf("expected workspace action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when path is unavailable")
	}
	if m.status != "workspace path unavailable" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestWorktreeContextActionAddRequiresWorkspace(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorktreeContextMenuAction(ContextMenuWorktreeAdd, contextMenuTarget{})
	if !handled {
		t.Fatalf("expected worktree action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when workspace is missing")
	}
	if m.status != "select a workspace" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestWorktreeContextActionCopyPathRequiresSelection(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorktreeContextMenuAction(ContextMenuWorktreeCopyPath, contextMenuTarget{})
	if !handled {
		t.Fatalf("expected worktree action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when worktree is missing")
	}
	if m.status != "select a worktree" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestWorktreeContextActionAddNoteEntersAddNoteMode(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorktreeContextMenuAction(ContextMenuWorktreeAddNote, contextMenuTarget{workspaceID: "ws1", worktreeID: "wt1"})
	if !handled {
		t.Fatalf("expected worktree action to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected note prefetch command")
	}
	if m.mode != uiModeAddNote {
		t.Fatalf("expected add note mode, got %v", m.mode)
	}
	if m.notesScope.Scope != types.NoteScopeWorktree || m.notesScope.WorkspaceID != "ws1" || m.notesScope.WorktreeID != "wt1" {
		t.Fatalf("unexpected notes scope: %#v", m.notesScope)
	}
}

func TestWorktreeContextActionOpenNotesEntersNotesMode(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleWorktreeContextMenuAction(ContextMenuWorktreeOpenNotes, contextMenuTarget{workspaceID: "ws1", worktreeID: "wt1"})
	if !handled {
		t.Fatalf("expected worktree action to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected notes fetch command")
	}
	if m.mode != uiModeNotes {
		t.Fatalf("expected notes mode, got %v", m.mode)
	}
	if m.notesScope.Scope != types.NoteScopeWorktree || m.notesScope.WorkspaceID != "ws1" || m.notesScope.WorktreeID != "wt1" {
		t.Fatalf("unexpected notes scope: %#v", m.notesScope)
	}
}

func TestWorktreeContextActionCopyPathUnavailable(t *testing.T) {
	m := NewModel(nil)
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree", Path: ""},
		},
	}

	handled, cmd := m.handleWorktreeContextMenuAction(ContextMenuWorktreeCopyPath, contextMenuTarget{worktreeID: "wt1"})
	if !handled {
		t.Fatalf("expected worktree action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when path is unavailable")
	}
	if m.status != "worktree path unavailable" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestSessionContextActionKillReturnsCommand(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.handleSessionContextMenuAction(ContextMenuSessionKill, contextMenuTarget{sessionID: "s1"})
	if !handled {
		t.Fatalf("expected session action to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected kill command")
	}
	if m.status != "killing s1" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestSessionContextActionAddNoteEntersAddNoteMode(t *testing.T) {
	m := NewModel(nil)
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}

	handled, cmd := m.handleSessionContextMenuAction(ContextMenuSessionAddNote, contextMenuTarget{sessionID: "s1"})
	if !handled {
		t.Fatalf("expected session action to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected note prefetch command")
	}
	if m.mode != uiModeAddNote {
		t.Fatalf("expected add note mode, got %v", m.mode)
	}
	if m.notesScope.Scope != types.NoteScopeSession || m.notesScope.SessionID != "s1" || m.notesScope.WorkspaceID != "ws1" || m.notesScope.WorktreeID != "wt1" {
		t.Fatalf("unexpected notes scope: %#v", m.notesScope)
	}
}

func TestSessionContextActionRenameEntersRenameMode(t *testing.T) {
	m := NewModel(nil)
	m.sessions = []*types.Session{{ID: "s1", Title: "Session One"}}

	handled, cmd := m.handleSessionContextMenuAction(ContextMenuSessionRename, contextMenuTarget{sessionID: "s1"})
	if !handled {
		t.Fatalf("expected session action to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command")
	}
	if m.mode != uiModeRenameSession {
		t.Fatalf("expected rename session mode, got %v", m.mode)
	}
	if m.renameSessionID != "s1" {
		t.Fatalf("expected rename session id s1, got %q", m.renameSessionID)
	}
	if m.renameInput == nil || m.renameInput.Value() != "Session One" {
		t.Fatalf("expected rename input to be prefilled")
	}
}

func TestSessionContextActionOpenNotesEntersNotesMode(t *testing.T) {
	m := NewModel(nil)
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}

	handled, cmd := m.handleSessionContextMenuAction(ContextMenuSessionOpenNotes, contextMenuTarget{sessionID: "s1"})
	if !handled {
		t.Fatalf("expected session action to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected notes fetch command")
	}
	if m.mode != uiModeNotes {
		t.Fatalf("expected notes mode, got %v", m.mode)
	}
	if m.notesScope.Scope != types.NoteScopeSession || m.notesScope.SessionID != "s1" || m.notesScope.WorkspaceID != "ws1" || m.notesScope.WorktreeID != "wt1" {
		t.Fatalf("unexpected notes scope: %#v", m.notesScope)
	}
}

func TestContextMenuControllerWorkspaceIncludesCopyPathAction(t *testing.T) {
	c := NewContextMenuController()
	c.OpenWorkspace("ws1", "Workspace", 0, 0)
	foundOpen := false
	foundNote := false
	foundAdd := false
	found := false
	for _, item := range c.items {
		if item.Action == ContextMenuWorkspaceOpenNotes {
			foundOpen = true
		}
		if item.Action == ContextMenuWorkspaceAddNote {
			foundNote = true
		}
		if item.Action == ContextMenuWorkspaceAddWorktree {
			foundAdd = true
		}
		if item.Action == ContextMenuWorkspaceCopyPath {
			found = true
		}
	}
	if !foundOpen {
		t.Fatalf("expected workspace context menu to include open notes action")
	}
	if !foundNote {
		t.Fatalf("expected workspace context menu to include add note action")
	}
	if !foundAdd {
		t.Fatalf("expected workspace context menu to include add worktree action")
	}
	if !found {
		t.Fatalf("expected workspace context menu to include copy path action")
	}
}

func TestContextMenuControllerWorktreeIncludesCopyPathAction(t *testing.T) {
	c := NewContextMenuController()
	c.OpenWorktree("wt1", "ws1", "Worktree", 0, 0)
	foundOpen := false
	foundNote := false
	found := false
	for _, item := range c.items {
		if item.Action == ContextMenuWorktreeOpenNotes {
			foundOpen = true
		}
		if item.Action == ContextMenuWorktreeAddNote {
			foundNote = true
		}
		if item.Action == ContextMenuWorktreeCopyPath {
			found = true
		}
	}
	if !foundOpen {
		t.Fatalf("expected worktree context menu to include open notes action")
	}
	if !foundNote {
		t.Fatalf("expected worktree context menu to include add note action")
	}
	if !found {
		t.Fatalf("expected worktree context menu to include copy path action")
	}
}

func TestContextMenuControllerSessionIncludesAddNoteAction(t *testing.T) {
	c := NewContextMenuController()
	c.OpenSession("s1", "ws1", "wt1", "Session", 0, 0)
	foundRename := false
	foundOpen := false
	found := false
	for _, item := range c.items {
		if item.Action == ContextMenuSessionRename {
			foundRename = true
		}
		if item.Action == ContextMenuSessionOpenNotes {
			foundOpen = true
		}
		if item.Action == ContextMenuSessionAddNote {
			found = true
		}
	}
	if !foundRename {
		t.Fatalf("expected session context menu to include rename session action")
	}
	if !foundOpen {
		t.Fatalf("expected session context menu to include open notes action")
	}
	if !found {
		t.Fatalf("expected session context menu to include add note action")
	}
}

func TestHandleContextMenuActionClosesMenuAndRoutes(t *testing.T) {
	m := NewModel(nil)
	if m.contextMenu == nil {
		t.Fatalf("expected context menu controller")
	}
	m.contextMenu.OpenWorkspace("ws1", "Workspace", 1, 1)

	cmd := m.handleContextMenuAction(ContextMenuWorkspaceRename)
	if cmd != nil {
		t.Fatalf("expected no command")
	}
	if m.mode != uiModeRenameWorkspace {
		t.Fatalf("expected rename workspace mode, got %v", m.mode)
	}
	if m.renameWorkspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", m.renameWorkspaceID)
	}
	if m.contextMenu.IsOpen() {
		t.Fatalf("expected context menu to be closed")
	}
}

func TestHandleContextMenuActionClosesMenuAndRoutesSessionRename(t *testing.T) {
	m := NewModel(nil)
	m.sessions = []*types.Session{{ID: "s1", Title: "Session"}}
	if m.contextMenu == nil {
		t.Fatalf("expected context menu controller")
	}
	m.contextMenu.OpenSession("s1", "ws1", "wt1", "Session", 1, 1)

	cmd := m.handleContextMenuAction(ContextMenuSessionRename)
	if cmd != nil {
		t.Fatalf("expected no command")
	}
	if m.mode != uiModeRenameSession {
		t.Fatalf("expected rename session mode, got %v", m.mode)
	}
	if m.renameSessionID != "s1" {
		t.Fatalf("expected session id s1, got %q", m.renameSessionID)
	}
	if m.contextMenu.IsOpen() {
		t.Fatalf("expected context menu to be closed")
	}
}
