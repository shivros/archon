package app

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestSidebarControllerScrollingDisabled(t *testing.T) {
	controller := NewSidebarController()

	if got := controller.ScrollbarWidth(); got != 0 {
		t.Fatalf("expected no sidebar scrollbar width when scrolling is disabled, got %d", got)
	}
	if controller.Scroll(1) {
		t.Fatalf("expected sidebar scroll to be disabled")
	}
	if controller.ScrollbarSelect(0) {
		t.Fatalf("expected sidebar scrollbar selection to be disabled")
	}
}

func TestSidebarControllerUnreadSessions(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "s2", Status: types.SessionStatusRunning, CreatedAt: now.Add(-1 * time.Minute)},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", LastTurnID: "turn-2"},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, meta, "ws1", "", false)
	if controller.delegate.isUnread("s1") || controller.delegate.isUnread("s2") {
		t.Fatalf("did not expect unread sessions on initial load")
	}
	if !controller.SelectBySessionID("s1") {
		t.Fatalf("expected to select session s1")
	}

	updatedMeta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", LastTurnID: "turn-3"},
	}
	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, updatedMeta, "ws1", "", false)
	if !controller.delegate.isUnread("s2") {
		t.Fatalf("expected session s2 to be unread after new activity")
	}

	if !controller.SelectBySessionID("s2") {
		t.Fatalf("expected to select session s2")
	}
	if controller.delegate.isUnread("s2") {
		t.Fatalf("expected session s2 unread flag to clear after selection")
	}
}

func TestSidebarControllerToggleSelectedContainer(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, meta, "ws1", "", false)
	if item := controller.SelectedItem(); item == nil || item.kind != sidebarSession {
		t.Fatalf("expected session to be selected by default")
	}
	controller.Select(0)
	if item := controller.SelectedItem(); item == nil || item.kind != sidebarWorkspace {
		t.Fatalf("expected workspace to be selected")
	}
	if !controller.ToggleSelectedContainer() {
		t.Fatalf("expected workspace toggle to change expansion state")
	}
	items := controller.Items()
	if len(items) != 1 {
		t.Fatalf("expected collapsed workspace to hide session, got %d rows", len(items))
	}
	if got := controller.SelectedItem(); got == nil || got.kind != sidebarWorkspace {
		t.Fatalf("expected workspace row to remain selected")
	}
}

func TestSidebarControllerSelectBySessionIDAutoExpandsParents(t *testing.T) {
	controller := NewSidebarController()
	controller.SetExpandByDefault(false)
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	worktrees := map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree"},
		},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}

	controller.Apply(workspaces, worktrees, sessions, meta, "ws1", "wt1", false)
	if len(controller.Items()) != 1 {
		t.Fatalf("expected only workspace row when default collapsed")
	}
	if !controller.SelectBySessionID("s1") {
		t.Fatalf("expected SelectBySessionID to auto-expand and select s1")
	}
	if got := controller.SelectedSessionID(); got != "s1" {
		t.Fatalf("expected selected session s1, got %q", got)
	}
	if !controller.IsWorkspaceExpanded("ws1") {
		t.Fatalf("expected workspace ws1 to be expanded")
	}
	if !controller.IsWorktreeExpanded("wt1") {
		t.Fatalf("expected worktree wt1 to be expanded")
	}
}
