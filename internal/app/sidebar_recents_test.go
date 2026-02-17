package app

import (
	"testing"

	"control/internal/types"
)

func TestBuildSidebarItemsWithRecentsAddsRowsAtTop(t *testing.T) {
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	items := buildSidebarItemsWithRecents(
		workspaces,
		map[string][]*types.Worktree{},
		nil,
		map[string]*types.SessionMeta{},
		false,
		sidebarRecentsState{Enabled: true, ReadyCount: 2, RunningCount: 1},
		sidebarExpansionResolver{defaultOn: true},
	)
	if len(items) < 3 {
		t.Fatalf("expected at least 3 recents rows, got %d", len(items))
	}
	row0 := items[0].(*sidebarItem)
	row1 := items[1].(*sidebarItem)
	row2 := items[2].(*sidebarItem)
	if row0.kind != sidebarRecentsAll || row0.recentsCount != 3 {
		t.Fatalf("unexpected recents all row: %#v", row0)
	}
	if row1.kind != sidebarRecentsReady || row1.recentsCount != 2 {
		t.Fatalf("unexpected recents ready row: %#v", row1)
	}
	if row2.kind != sidebarRecentsRunning || row2.recentsCount != 1 {
		t.Fatalf("unexpected recents running row: %#v", row2)
	}
}

func TestBuildSidebarItemsWithRecentsDisabledOmitsRows(t *testing.T) {
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	items := buildSidebarItemsWithRecents(
		workspaces,
		map[string][]*types.Worktree{},
		nil,
		map[string]*types.SessionMeta{},
		false,
		sidebarRecentsState{Enabled: false, ReadyCount: 2, RunningCount: 1},
		sidebarExpansionResolver{defaultOn: true},
	)
	if len(items) == 0 {
		t.Fatalf("expected at least workspace row")
	}
	first := items[0].(*sidebarItem)
	if first.kind == sidebarRecentsAll || first.kind == sidebarRecentsReady || first.kind == sidebarRecentsRunning {
		t.Fatalf("did not expect recents row when disabled, got kind=%v", first.kind)
	}
}
