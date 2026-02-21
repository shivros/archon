package app

import (
	"testing"

	"control/internal/types"
)

func TestModelContextLabelsSessionDisplayName(t *testing.T) {
	var nilModel *Model
	if got := nilModel.sessionDisplayName("s1"); got != "" {
		t.Fatalf("expected nil model session display name to be empty, got %q", got)
	}

	m := NewModel(nil)
	if got := m.sessionDisplayName(""); got != "" {
		t.Fatalf("expected empty session id display name to be empty, got %q", got)
	}

	m.sessions = []*types.Session{
		{ID: "s1", Title: "Session Title"},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", Title: "Meta Title"},
		"s2": {SessionID: "s2", Title: "Meta Only Title"},
	}
	if got := m.sessionDisplayName("s1"); got != "Meta Title" {
		t.Fatalf("expected meta title precedence, got %q", got)
	}
	if got := m.sessionDisplayName("s2"); got != "Meta Only Title" {
		t.Fatalf("expected meta-only fallback title, got %q", got)
	}

	m.sessionMeta["s3"] = &types.SessionMeta{SessionID: "s3"}
	m.sessions = append(m.sessions, &types.Session{ID: "s3"})
	if got := m.sessionDisplayName("s3"); got != "s3" {
		t.Fatalf("expected session id fallback title, got %q", got)
	}
}

func TestModelContextLabelsWorkspaceAndWorktreeNameByID(t *testing.T) {
	var nilModel *Model
	if got := nilModel.workspaceNameByID("ws1"); got != "" {
		t.Fatalf("expected nil model workspace name to be empty, got %q", got)
	}
	if got := nilModel.worktreeNameByID("wt1"); got != "" {
		t.Fatalf("expected nil model worktree name to be empty, got %q", got)
	}

	m := NewModel(nil)
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: " Payments Workspace "},
	}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: " feature/retry-cleanup "},
		},
	}

	if got := m.workspaceNameByID(""); got != "" {
		t.Fatalf("expected empty workspace id to return empty name, got %q", got)
	}
	if got := m.worktreeNameByID(""); got != "" {
		t.Fatalf("expected empty worktree id to return empty name, got %q", got)
	}
	if got := m.workspaceNameByID("missing"); got != "" {
		t.Fatalf("expected missing workspace id to return empty name, got %q", got)
	}
	if got := m.worktreeNameByID("missing"); got != "" {
		t.Fatalf("expected missing worktree id to return empty name, got %q", got)
	}
	if got := m.workspaceNameByID("ws1"); got != "Payments Workspace" {
		t.Fatalf("expected trimmed workspace name, got %q", got)
	}
	if got := m.worktreeNameByID("wt1"); got != "feature/retry-cleanup" {
		t.Fatalf("expected trimmed worktree name, got %q", got)
	}
}
