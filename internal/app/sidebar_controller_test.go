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
