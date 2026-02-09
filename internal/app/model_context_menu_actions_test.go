package app

import "testing"

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
