package app

import (
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type staticSidebarProjectionInvalidationPolicy struct {
	shouldInvalidate bool
}

func (p staticSidebarProjectionInvalidationPolicy) ShouldInvalidate(sidebarProjectionChangeReason) bool {
	return p.shouldInvalidate
}

type staticSidebarProjectionBuilder struct {
	projection SidebarProjection
}

func (b staticSidebarProjectionBuilder) Build(SidebarProjectionInput) SidebarProjection {
	return b.projection
}

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
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-a", WorkspaceID: "ws-a"},
		{ID: "gwf-b", WorkspaceID: "ws-b"},
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
		WorkflowRuns:       workflows,
		ActiveWorkspaceIDs: []string{"g1"},
	})
	if len(projection.Workspaces) != 1 || projection.Workspaces[0].ID != "ws-a" {
		t.Fatalf("expected only ws-a to be visible, got %#v", projection.Workspaces)
	}
	if len(projection.Sessions) != 1 || projection.Sessions[0].ID != "s-a" {
		t.Fatalf("expected only s-a to be visible, got %#v", projection.Sessions)
	}
	if len(projection.WorkflowRuns) != 1 || projection.WorkflowRuns[0].ID != "gwf-a" {
		t.Fatalf("expected only gwf-a to be visible, got %#v", projection.WorkflowRuns)
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

func TestSidebarProjectionBuilderFallsBackToWorkspaceWhenWorktreeUnknown(t *testing.T) {
	builder := NewDefaultSidebarProjectionBuilder()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws-a", Name: "A", GroupIDs: []string{"g1"}},
	}
	worktrees := map[string][]*types.Worktree{
		"ws-a": {},
	}
	sessions := []*types.Session{
		{ID: "s-a", CreatedAt: now},
	}
	meta := map[string]*types.SessionMeta{
		"s-a": {SessionID: "s-a", WorkspaceID: "ws-a", WorktreeID: "wt-missing"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-a", WorkspaceID: "ws-a", WorktreeID: "wt-missing"},
	}

	projection := builder.Build(SidebarProjectionInput{
		Workspaces:         workspaces,
		Worktrees:          worktrees,
		Sessions:           sessions,
		SessionMeta:        meta,
		WorkflowRuns:       workflows,
		ActiveWorkspaceIDs: []string{"g1"},
	})
	if len(projection.Sessions) != 1 || projection.Sessions[0].ID != "s-a" {
		t.Fatalf("expected session to remain visible via workspace fallback, got %#v", projection.Sessions)
	}
	if len(projection.WorkflowRuns) != 1 || projection.WorkflowRuns[0].ID != "gwf-a" {
		t.Fatalf("expected workflow to remain visible via workspace fallback, got %#v", projection.WorkflowRuns)
	}
}

func TestDefaultSidebarProjectionInvalidationPolicyRecognizesKnownReasons(t *testing.T) {
	policy := NewDefaultSidebarProjectionInvalidationPolicy()
	reasons := []sidebarProjectionChangeReason{
		sidebarProjectionChangeSessions,
		sidebarProjectionChangeMeta,
		sidebarProjectionChangeWorkspace,
		sidebarProjectionChangeWorktree,
		sidebarProjectionChangeGroup,
		sidebarProjectionChangeDismissed,
		sidebarProjectionChangeAppState,
	}
	for _, reason := range reasons {
		if !policy.ShouldInvalidate(reason) {
			t.Fatalf("expected policy to invalidate reason %q", reason)
		}
	}
	if policy.ShouldInvalidate(sidebarProjectionChangeReason("unknown")) {
		t.Fatalf("expected unknown reason to skip invalidation")
	}
}

func TestModelInvalidateSidebarProjectionHonorsPolicy(t *testing.T) {
	m := NewModel(nil, WithSidebarProjectionInvalidationPolicy(staticSidebarProjectionInvalidationPolicy{
		shouldInvalidate: false,
	}))
	initial := m.sidebarProjectionRevision
	m.invalidateSidebarProjection(sidebarProjectionChangeSessions)
	if m.sidebarProjectionRevision != initial {
		t.Fatalf("expected revision to remain %d, got %d", initial, m.sidebarProjectionRevision)
	}
}

func TestWithSidebarProjectionBuilderAppliesCustomBuilder(t *testing.T) {
	builder := staticSidebarProjectionBuilder{
		projection: SidebarProjection{
			Workspaces: []*types.Workspace{{ID: "ws-custom"}},
		},
	}
	m := NewModel(nil, WithSidebarProjectionBuilder(builder))

	projection := m.sidebarProjectionBuilder.Build(SidebarProjectionInput{})
	if len(projection.Workspaces) != 1 || projection.Workspaces[0].ID != "ws-custom" {
		t.Fatalf("expected custom projection builder to be used, got %#v", projection.Workspaces)
	}
}

func TestWithSidebarProjectionBuilderIgnoresNilInputs(t *testing.T) {
	builder := staticSidebarProjectionBuilder{
		projection: SidebarProjection{
			Workspaces: []*types.Workspace{{ID: "ws-initial"}},
		},
	}
	model := NewModel(nil, WithSidebarProjectionBuilder(builder))
	m := &model

	WithSidebarProjectionBuilder(nil)(m)
	projection := m.sidebarProjectionBuilder.Build(SidebarProjectionInput{})
	if len(projection.Workspaces) != 1 || projection.Workspaces[0].ID != "ws-initial" {
		t.Fatalf("expected nil builder option to be a no-op, got %#v", projection.Workspaces)
	}

	WithSidebarProjectionBuilder(builder)(nil)
}
