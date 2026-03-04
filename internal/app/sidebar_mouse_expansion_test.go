package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

func TestDefaultSidebarExpansionIntentPolicyResolveIntent(t *testing.T) {
	policy := defaultSidebarExpansionIntentPolicy{}

	if got := policy.ResolveIntent(nil, tea.Mouse{}); got.kind != sidebarExpansionIntentNone {
		t.Fatalf("expected none intent for nil entry, got %#v", got)
	}

	workspace := &sidebarItem{kind: sidebarWorkspace, expanded: true}
	if got := policy.ResolveIntent(workspace, tea.Mouse{}); got.kind != sidebarExpansionIntentSingleToggle || got.expanded {
		t.Fatalf("expected single-toggle collapse for plain workspace click, got %#v", got)
	}
	if got := policy.ResolveIntent(workspace, tea.Mouse{Mod: tea.ModCtrl}); got.kind != sidebarExpansionIntentAllWorkspaces || got.expanded {
		t.Fatalf("expected ctrl workspace intent to collapse all workspaces, got %#v", got)
	}

	worktree := &sidebarItem{kind: sidebarWorktree, expanded: false, worktree: &types.Worktree{ID: "wt1"}}
	if got := policy.ResolveIntent(worktree, tea.Mouse{Mod: tea.ModCtrl}); got.kind != sidebarExpansionIntentWorktreesForWorktree || !got.expanded || got.worktreeID != "wt1" {
		t.Fatalf("expected ctrl worktree intent for workspace-scoped expand, got %#v", got)
	}
	worktreeWithoutPayload := &sidebarItem{kind: sidebarWorktree, expanded: false}
	if got := policy.ResolveIntent(worktreeWithoutPayload, tea.Mouse{Mod: tea.ModCtrl}); got.kind != sidebarExpansionIntentNone {
		t.Fatalf("expected ctrl worktree with nil worktree payload to produce none intent, got %#v", got)
	}

	workflow := &sidebarItem{kind: sidebarWorkflow, expanded: false}
	if got := policy.ResolveIntent(workflow, tea.Mouse{Mod: tea.ModCtrl}); got.kind != sidebarExpansionIntentSingleToggle || !got.expanded {
		t.Fatalf("expected workflow ctrl-click to remain single-toggle, got %#v", got)
	}
}

type testSidebarExpansionController struct {
	toggleCalled              int
	setAllWorkspacesCalled    int
	setWorktreesForWTCalled   int
	lastAllWorkspacesExpanded bool
	lastWorktreeID            string
	lastWorktreeExpanded      bool
}

func (c *testSidebarExpansionController) ToggleSelectedContainer() bool {
	c.toggleCalled++
	return true
}

func (c *testSidebarExpansionController) SetAllWorkspacesExpanded(expanded bool) bool {
	c.setAllWorkspacesCalled++
	c.lastAllWorkspacesExpanded = expanded
	return true
}

func (c *testSidebarExpansionController) SetWorktreesExpandedForWorktree(worktreeID string, expanded bool) bool {
	c.setWorktreesForWTCalled++
	c.lastWorktreeID = worktreeID
	c.lastWorktreeExpanded = expanded
	return true
}

func TestDefaultSidebarExpansionServiceApplyIntent(t *testing.T) {
	service := defaultSidebarExpansionService{}
	controller := &testSidebarExpansionController{}

	if service.ApplyIntent(nil, sidebarExpansionIntent{kind: sidebarExpansionIntentSingleToggle}) {
		t.Fatalf("expected nil controller to reject intent")
	}

	if !service.ApplyIntent(controller, sidebarExpansionIntent{kind: sidebarExpansionIntentSingleToggle}) {
		t.Fatalf("expected single-toggle intent to be applied")
	}
	if controller.toggleCalled != 1 {
		t.Fatalf("expected toggle to be called once, got %d", controller.toggleCalled)
	}

	if !service.ApplyIntent(controller, sidebarExpansionIntent{kind: sidebarExpansionIntentAllWorkspaces, expanded: true}) {
		t.Fatalf("expected all-workspaces intent to be applied")
	}
	if controller.setAllWorkspacesCalled != 1 || !controller.lastAllWorkspacesExpanded {
		t.Fatalf("expected all-workspaces call with expanded=true, got calls=%d expanded=%v", controller.setAllWorkspacesCalled, controller.lastAllWorkspacesExpanded)
	}

	if !service.ApplyIntent(controller, sidebarExpansionIntent{
		kind:       sidebarExpansionIntentWorktreesForWorktree,
		worktreeID: "wt1",
		expanded:   false,
	}) {
		t.Fatalf("expected workspace-worktrees intent to be applied")
	}
	if controller.setWorktreesForWTCalled != 1 || controller.lastWorktreeID != "wt1" || controller.lastWorktreeExpanded {
		t.Fatalf("expected worktree scope call for wt1 expanded=false, got calls=%d id=%q expanded=%v", controller.setWorktreesForWTCalled, controller.lastWorktreeID, controller.lastWorktreeExpanded)
	}

	if service.ApplyIntent(controller, sidebarExpansionIntent{
		kind:       sidebarExpansionIntentWorktreesForWorktree,
		worktreeID: "",
		expanded:   true,
	}) {
		t.Fatalf("expected empty-worktree scope intent to be rejected")
	}
	if service.ApplyIntent(controller, sidebarExpansionIntent{kind: sidebarExpansionIntentNone}) {
		t.Fatalf("expected none intent to be rejected")
	}
}

type testSidebarExpansionIntentPolicy struct {
	intent sidebarExpansionIntent
	calls  int
}

func (p *testSidebarExpansionIntentPolicy) ResolveIntent(entry *sidebarItem, mouse tea.Mouse) sidebarExpansionIntent {
	p.calls++
	return p.intent
}

type testSidebarExpansionService struct {
	calls      int
	lastIntent sidebarExpansionIntent
	lastTarget SidebarExpansionController
	result     bool
}

func (s *testSidebarExpansionService) ApplyIntent(sidebar SidebarExpansionController, intent sidebarExpansionIntent) bool {
	s.calls++
	s.lastTarget = sidebar
	s.lastIntent = intent
	return s.result
}

func TestModelToggleSidebarContainerFromMouseUsesPolicyAndService(t *testing.T) {
	policy := &testSidebarExpansionIntentPolicy{
		intent: sidebarExpansionIntent{kind: sidebarExpansionIntentAllWorkspaces, expanded: false},
	}
	service := &testSidebarExpansionService{result: true}
	m := NewModel(nil, WithSidebarExpansionIntentPolicy(policy), WithSidebarExpansionService(service))

	changed := m.toggleSidebarContainerFromMouse(&sidebarItem{
		kind:     sidebarWorkspace,
		expanded: true,
	}, tea.Mouse{Mod: tea.ModCtrl})
	if !changed {
		t.Fatalf("expected service result to be returned")
	}
	if policy.calls != 1 {
		t.Fatalf("expected policy to be called once, got %d", policy.calls)
	}
	if service.calls != 1 {
		t.Fatalf("expected service to be called once, got %d", service.calls)
	}
	if service.lastTarget != m.sidebar {
		t.Fatalf("expected service to receive model sidebar target")
	}
	if service.lastIntent.kind != sidebarExpansionIntentAllWorkspaces || service.lastIntent.expanded {
		t.Fatalf("unexpected intent passed to service: %#v", service.lastIntent)
	}
}

func TestModelSidebarExpansionOptionAndDefaults(t *testing.T) {
	var nilModel *Model
	if policy := nilModel.sidebarExpansionIntentPolicyOrDefault(); policy == nil {
		t.Fatalf("expected default policy for nil model")
	}
	if service := nilModel.sidebarExpansionServiceOrDefault(); service == nil {
		t.Fatalf("expected default service for nil model")
	}

	m := NewModel(nil)
	if m.sidebarExpansionIntentPolicy == nil || m.sidebarExpansionService == nil {
		t.Fatalf("expected non-nil default expansion dependencies")
	}

	customPolicy := &testSidebarExpansionIntentPolicy{intent: sidebarExpansionIntent{kind: sidebarExpansionIntentNone}}
	customService := &testSidebarExpansionService{result: true}
	WithSidebarExpansionIntentPolicy(customPolicy)(&m)
	WithSidebarExpansionService(customService)(&m)
	if m.sidebarExpansionIntentPolicy != customPolicy {
		t.Fatalf("expected custom policy assignment")
	}
	if m.sidebarExpansionService != customService {
		t.Fatalf("expected custom service assignment")
	}

	WithSidebarExpansionIntentPolicy(nil)(&m)
	WithSidebarExpansionService(nil)(&m)
	if _, ok := m.sidebarExpansionIntentPolicy.(defaultSidebarExpansionIntentPolicy); !ok {
		t.Fatalf("expected nil policy option to restore default policy, got %T", m.sidebarExpansionIntentPolicy)
	}
	if _, ok := m.sidebarExpansionService.(defaultSidebarExpansionService); !ok {
		t.Fatalf("expected nil service option to restore default service, got %T", m.sidebarExpansionService)
	}
}
