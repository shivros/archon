package app

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestBuildSidebarItemsGroupsSessions(t *testing.T) {
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
		{ID: "ws2", Name: "Workspace Two"},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now.Add(-5 * time.Minute)},
		{ID: "s2", Status: types.SessionStatusExited, CreatedAt: now.Add(-4 * time.Minute)},
		{ID: "s3", Status: types.SessionStatusRunning, CreatedAt: now.Add(-3 * time.Minute)},
		{ID: "s4", Status: types.SessionStatusRunning, CreatedAt: now.Add(-2 * time.Minute)},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s3": {SessionID: "s3", WorkspaceID: "ws2"},
		"s4": {SessionID: "s4", WorkspaceID: "missing"},
	}

	items := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, meta)
	if len(items) != 6 {
		t.Fatalf("expected 6 items, got %d", len(items))
	}

	ws1 := items[0].(*sidebarItem)
	if ws1.kind != sidebarWorkspace || ws1.workspace.ID != "ws1" || ws1.sessionCount != 1 {
		t.Fatalf("expected ws1 workspace item with 1 session")
	}
	s1 := items[1].(*sidebarItem)
	if !s1.isSession() || s1.session.ID != "s1" {
		t.Fatalf("expected s1 session under ws1")
	}
	ws2 := items[2].(*sidebarItem)
	if ws2.kind != sidebarWorkspace || ws2.workspace.ID != "ws2" || ws2.sessionCount != 1 {
		t.Fatalf("expected ws2 workspace item with 1 session")
	}
	s3 := items[3].(*sidebarItem)
	if !s3.isSession() || s3.session.ID != "s3" {
		t.Fatalf("expected s3 session under ws2")
	}
	wsUnassigned := items[4].(*sidebarItem)
	if wsUnassigned.kind != sidebarWorkspace || wsUnassigned.workspace.Name != unassignedWorkspaceTag {
		t.Fatalf("expected unassigned workspace")
	}
	s4 := items[5].(*sidebarItem)
	if !s4.isSession() || s4.session.ID != "s4" {
		t.Fatalf("expected s4 session under unassigned")
	}
}

func TestSessionTitlePriority(t *testing.T) {
	session := &types.Session{ID: "s1", Title: "fallback"}
	meta := &types.SessionMeta{
		SessionID:    "s1",
		Title:        "from meta",
		InitialInput: "from input",
	}
	if got := sessionTitle(session, meta); got != "from meta" {
		t.Fatalf("expected meta title, got %q", got)
	}
	meta.Title = ""
	if got := sessionTitle(session, meta); got != "from input" {
		t.Fatalf("expected initial input title, got %q", got)
	}
	meta.InitialInput = ""
	if got := sessionTitle(session, meta); got != "fallback" {
		t.Fatalf("expected session title, got %q", got)
	}
	session.Title = ""
	if got := sessionTitle(session, nil); got != "s1" {
		t.Fatalf("expected session id fallback, got %q", got)
	}
}

func TestSelectSidebarIndexPrefersSelectionAndActiveWorkspace(t *testing.T) {
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
		{ID: "ws2", Name: "Workspace Two"},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning},
		{ID: "s2", Status: types.SessionStatusRunning},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws2"},
	}
	items := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, meta)

	selected := "sess:s1"
	if idx := selectSidebarIndex(items, selected, "ws2", ""); idx != 1 {
		t.Fatalf("expected selected session index 1, got %d", idx)
	}

	if idx := selectSidebarIndex(items, "", "ws2", ""); idx != 3 {
		t.Fatalf("expected ws2 session index 3, got %d", idx)
	}
}
