package app

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestSidebarProjectionBuilderFiltersBySelectedGroups(t *testing.T) {
	builder := NewDefaultSidebarProjectionBuilder()
	now := time.Now().UTC()

	workspaces := []*types.Workspace{
		{ID: "ws-a", Name: "A", GroupIDs: []string{"g1"}},
		{ID: "ws-b", Name: "B", GroupIDs: []string{"g2"}},
	}
	sessions := []*types.Session{
		{ID: "s-a", CreatedAt: now},
		{ID: "s-b", CreatedAt: now.Add(time.Second)},
	}
	meta := map[string]*types.SessionMeta{
		"s-a": {SessionID: "s-a", WorkspaceID: "ws-a"},
		"s-b": {SessionID: "s-b", WorkspaceID: "ws-b"},
	}

	projection := builder.Build(SidebarProjectionInput{
		Workspaces:         workspaces,
		Worktrees:          map[string][]*types.Worktree{},
		Sessions:           sessions,
		SessionMeta:        meta,
		ActiveWorkspaceIDs: []string{"g1"},
	})
	if len(projection.Workspaces) != 1 || projection.Workspaces[0].ID != "ws-a" {
		t.Fatalf("expected only ws-a to be visible, got %#v", projection.Workspaces)
	}
	if len(projection.Sessions) != 1 || projection.Sessions[0].ID != "s-a" {
		t.Fatalf("expected only s-a to be visible, got %#v", projection.Sessions)
	}
}

func TestSidebarProjectionBuilderIncludesUngrouped(t *testing.T) {
	builder := NewDefaultSidebarProjectionBuilder()
	now := time.Now().UTC()

	workspaces := []*types.Workspace{
		{ID: "ws-a", Name: "A"},
	}
	sessions := []*types.Session{
		{ID: "s-a", CreatedAt: now},
	}

	projection := builder.Build(SidebarProjectionInput{
		Workspaces:         workspaces,
		Worktrees:          map[string][]*types.Worktree{},
		Sessions:           sessions,
		SessionMeta:        map[string]*types.SessionMeta{},
		ActiveWorkspaceIDs: []string{"ungrouped"},
	})
	if len(projection.Workspaces) != 1 || projection.Workspaces[0].ID != "ws-a" {
		t.Fatalf("expected ungrouped workspace to be visible, got %#v", projection.Workspaces)
	}
	if len(projection.Sessions) != 1 || projection.Sessions[0].ID != "s-a" {
		t.Fatalf("expected unassigned session to be visible, got %#v", projection.Sessions)
	}
}
