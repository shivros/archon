package app

import (
	"testing"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestDefaultSidebarWorkflowScopeResolverWorkspaceScope(t *testing.T) {
	resolver := NewDefaultSidebarWorkflowScopeResolver()
	input := SidebarWorkflowScopeInput{
		Workspaces: []*types.Workspace{
			{ID: "ws1", Name: "Workspace 1"},
			{ID: "ws2", Name: "Workspace 2"},
		},
		Worktrees: map[string][]*types.Worktree{
			"ws1": {{ID: "wt1", WorkspaceID: "ws1", Name: "WT1"}},
		},
		Sessions: []*types.Session{
			{ID: "s1"},
			{ID: "s2"},
			{ID: "s3"},
		},
		WorkflowRuns: []*guidedworkflows.WorkflowRun{
			{ID: "gwf-run-ws1", WorkspaceID: "ws1"},
			{ID: "gwf-run-wt1", WorkspaceID: "ws1", WorktreeID: "wt1"},
			{ID: "gwf-run-ws2", WorkspaceID: "ws2"},
		},
		Meta: map[string]*types.SessionMeta{
			"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: "gwf-run-ws1"},
			"s2": {SessionID: "s2", WorkspaceID: "ws1", WorkflowRunID: "gwf-meta-only-ws1"},
			"s3": {SessionID: "s3", WorkspaceID: "ws1", WorktreeID: "wt1", WorkflowRunID: "gwf-meta-wt1"},
		},
	}

	got := resolver.WorkflowIDsForWorkspace(input, "ws1")
	want := []string{"gwf-run-ws1", "gwf-meta-only-ws1"}
	assertWorkflowIDSet(t, got, want)
}

func TestDefaultSidebarWorkflowScopeResolverWorktreeScope(t *testing.T) {
	resolver := NewDefaultSidebarWorkflowScopeResolver()
	input := SidebarWorkflowScopeInput{
		Workspaces: []*types.Workspace{{ID: "ws1", Name: "Workspace 1"}},
		Worktrees: map[string][]*types.Worktree{
			"ws1": {
				{ID: "wt1", WorkspaceID: "ws1", Name: "WT1"},
				{ID: "wt2", WorkspaceID: "ws1", Name: "WT2"},
			},
		},
		Sessions: []*types.Session{{ID: "s1"}, {ID: "s2"}},
		WorkflowRuns: []*guidedworkflows.WorkflowRun{
			{ID: "gwf-run-wt1", WorkspaceID: "ws1", WorktreeID: "wt1"},
			{ID: "gwf-run-wt2", WorkspaceID: "ws1", WorktreeID: "wt2"},
		},
		Meta: map[string]*types.SessionMeta{
			"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1", WorkflowRunID: "gwf-meta-wt1"},
			"s2": {SessionID: "s2", WorkspaceID: "ws1", WorktreeID: "wt2", WorkflowRunID: "gwf-meta-wt2"},
		},
	}

	got := resolver.WorkflowIDsForWorktree(input, "wt1")
	want := []string{"gwf-run-wt1", "gwf-meta-wt1"}
	assertWorkflowIDSet(t, got, want)
}

func TestDefaultSidebarWorkflowScopeResolverUnassignedWorkspace(t *testing.T) {
	resolver := NewDefaultSidebarWorkflowScopeResolver()
	input := SidebarWorkflowScopeInput{
		Workspaces: []*types.Workspace{{ID: "ws1", Name: "Workspace 1"}},
		Worktrees:  map[string][]*types.Worktree{},
		Sessions:   []*types.Session{{ID: "s1"}, {ID: "s2"}},
		WorkflowRuns: []*guidedworkflows.WorkflowRun{
			{ID: "gwf-unassigned-run"},
			{ID: "gwf-stale-workspace", WorkspaceID: "ws-missing"},
		},
		Meta: map[string]*types.SessionMeta{
			"s1": {SessionID: "s1", WorkflowRunID: "gwf-unassigned-meta"},
			"s2": {SessionID: "s2", WorkspaceID: "ws-missing", WorkflowRunID: "gwf-stale-meta"},
		},
	}

	got := resolver.WorkflowIDsForWorkspace(input, "")
	want := []string{"gwf-unassigned-run", "gwf-stale-workspace", "gwf-unassigned-meta", "gwf-stale-meta"}
	assertWorkflowIDSet(t, got, want)
}

func TestDefaultSidebarWorkflowScopeResolverStaleWorktreeFallsBackToWorkspace(t *testing.T) {
	resolver := NewDefaultSidebarWorkflowScopeResolver()
	input := SidebarWorkflowScopeInput{
		Workspaces: []*types.Workspace{{ID: "ws1", Name: "Workspace 1"}},
		Worktrees:  map[string][]*types.Worktree{"ws1": {{ID: "wt1", WorkspaceID: "ws1", Name: "WT1"}}},
		Sessions:   []*types.Session{{ID: "s1"}},
		Meta: map[string]*types.SessionMeta{
			"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt-missing", WorkflowRunID: "gwf-stale-worktree"},
		},
	}

	gotWorkspace := resolver.WorkflowIDsForWorkspace(input, "ws1")
	assertWorkflowIDSet(t, gotWorkspace, []string{"gwf-stale-worktree"})

	gotWorktree := resolver.WorkflowIDsForWorktree(input, "wt1")
	assertWorkflowIDSet(t, gotWorktree, nil)
}

func TestDefaultSidebarWorkflowScopeResolverGuardsAndNoisyRoots(t *testing.T) {
	resolver := NewDefaultSidebarWorkflowScopeResolver()
	input := SidebarWorkflowScopeInput{
		Workspaces: []*types.Workspace{
			nil,
			{ID: "", Name: "Blank"},
			{ID: "ws1", Name: "Workspace 1"},
		},
		Worktrees: map[string][]*types.Worktree{
			"": {
				{ID: "ignored-wt", WorkspaceID: "", Name: "Ignored"},
			},
			"ws1": {
				nil,
				{ID: "", WorkspaceID: "ws1", Name: "Blank"},
				{ID: "wt1", WorkspaceID: "ws1", Name: "WT1"},
			},
		},
		Sessions: []*types.Session{
			nil,
			{ID: "s-empty-workflow"},
			{ID: "s-no-meta"},
			{ID: "s-wt"},
		},
		WorkflowRuns: []*guidedworkflows.WorkflowRun{
			nil,
			{ID: "", WorkspaceID: "ws1"},
			{ID: "gwf-ws", WorkspaceID: "ws1"},
			{ID: "gwf-wt", WorkspaceID: "ws1", WorktreeID: "wt1"},
			{ID: "gwf-stale-worktree", WorkspaceID: "ws1", WorktreeID: "wt-missing"},
		},
		Meta: map[string]*types.SessionMeta{
			"s-empty-workflow": {SessionID: "s-empty-workflow", WorkspaceID: "ws1", WorkflowRunID: "   "},
			"s-wt":             {SessionID: "s-wt", WorkspaceID: "ws1", WorktreeID: "wt1", WorkflowRunID: "gwf-meta-wt"},
		},
	}

	gotWorkspace := resolver.WorkflowIDsForWorkspace(input, "ws1")
	assertWorkflowIDSet(t, gotWorkspace, []string{"gwf-ws", "gwf-stale-worktree"})

	gotWorktree := resolver.WorkflowIDsForWorktree(input, "wt1")
	assertWorkflowIDSet(t, gotWorktree, []string{"gwf-wt", "gwf-meta-wt"})

	if got := resolver.WorkflowIDsForWorkspace(input, "   "); len(got) != 0 {
		t.Fatalf("expected blank workspace query to produce no matches, got %v", got)
	}
	if got := resolver.WorkflowIDsForWorktree(input, "   "); len(got) != 0 {
		t.Fatalf("expected blank worktree query to produce no matches, got %v", got)
	}
}

func assertWorkflowIDSet(t *testing.T, got, want []string) {
	t.Helper()
	gotSet := map[string]struct{}{}
	for _, id := range got {
		gotSet[id] = struct{}{}
	}
	wantSet := map[string]struct{}{}
	for _, id := range want {
		wantSet[id] = struct{}{}
	}
	if len(gotSet) != len(wantSet) {
		t.Fatalf("unexpected workflow set size: got=%v want=%v", got, want)
	}
	for id := range wantSet {
		if _, ok := gotSet[id]; !ok {
			t.Fatalf("missing workflow id %q in %v", id, got)
		}
	}
}
