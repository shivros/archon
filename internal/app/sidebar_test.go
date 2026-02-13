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

	items := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, meta, false)
	if len(items) != 7 {
		t.Fatalf("expected 7 items, got %d", len(items))
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
	s2 := items[6].(*sidebarItem)
	if !s2.isSession() || s2.session.ID != "s2" {
		t.Fatalf("expected exited session s2 under unassigned")
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
	items := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, meta, false)

	selected := "sess:s1"
	if idx := selectSidebarIndex(items, selected, "ws2", ""); idx != 1 {
		t.Fatalf("expected selected session index 1, got %d", idx)
	}

	if idx := selectSidebarIndex(items, "", "ws2", ""); idx != 3 {
		t.Fatalf("expected ws2 session index 3, got %d", idx)
	}
}

func TestBuildSidebarItemsShowDismissedToggle(t *testing.T) {
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
	}
	sessions := []*types.Session{
		{ID: "active", Status: types.SessionStatusInactive, CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "dismissed", Status: types.SessionStatusExited, CreatedAt: now.Add(-1 * time.Minute)},
	}
	dismissedAt := now.Add(-30 * time.Second)
	meta := map[string]*types.SessionMeta{
		"active":    {SessionID: "active", WorkspaceID: "ws1"},
		"dismissed": {SessionID: "dismissed", WorkspaceID: "ws1", DismissedAt: &dismissedAt},
	}

	hidden := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, meta, false)
	if len(hidden) != 2 {
		t.Fatalf("expected workspace + active session when dismissed hidden, got %d", len(hidden))
	}

	visible := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, meta, true)
	if len(visible) != 3 {
		t.Fatalf("expected workspace + both sessions when dismissed shown, got %d", len(visible))
	}
}

func TestResolveProviderBadgeUsesDefaults(t *testing.T) {
	codex := resolveProviderBadge("codex", nil)
	if codex.Prefix != "[CDX]" {
		t.Fatalf("expected codex prefix [CDX], got %q", codex.Prefix)
	}
	if codex.Color != "15" {
		t.Fatalf("expected codex color 15, got %q", codex.Color)
	}

	claude := resolveProviderBadge("claude", nil)
	if claude.Prefix != "[CLD]" {
		t.Fatalf("expected claude prefix [CLD], got %q", claude.Prefix)
	}
	if claude.Color != "208" {
		t.Fatalf("expected claude color 208, got %q", claude.Color)
	}

	kilocode := resolveProviderBadge("kilocode", nil)
	if kilocode.Prefix != "[KLO]" {
		t.Fatalf("expected kilocode prefix [KLO], got %q", kilocode.Prefix)
	}
	if kilocode.Color != "226" {
		t.Fatalf("expected kilocode color 226, got %q", kilocode.Color)
	}
}

func TestResolveProviderBadgeAppliesOverrides(t *testing.T) {
	overrides := normalizeProviderBadgeOverrides(map[string]*types.ProviderBadgeConfig{
		" CoDeX ": {
			Prefix: " [GPT] ",
			Color:  " 231 ",
		},
	})
	resolved := resolveProviderBadge("codex", overrides)
	if resolved.Prefix != "[GPT]" {
		t.Fatalf("expected override prefix [GPT], got %q", resolved.Prefix)
	}
	if resolved.Color != "231" {
		t.Fatalf("expected override color 231, got %q", resolved.Color)
	}
}

func TestResolveProviderBadgeUnknownProviderFallback(t *testing.T) {
	resolved := resolveProviderBadge("open code", nil)
	if resolved.Prefix != "[OPE]" {
		t.Fatalf("expected fallback prefix [OPE], got %q", resolved.Prefix)
	}
	if resolved.Color != defaultBadgeColor {
		t.Fatalf("expected fallback color %q, got %q", defaultBadgeColor, resolved.Color)
	}
}
