package app

import "testing"

type stubGuidedWorkflowLabelResolver struct {
	workspaceNames map[string]string
	worktreeNames  map[string]string
	sessionNames   map[string]string
}

func (s stubGuidedWorkflowLabelResolver) workspaceNameByID(workspaceID string) string {
	if s.workspaceNames == nil {
		return ""
	}
	return s.workspaceNames[workspaceID]
}

func (s stubGuidedWorkflowLabelResolver) worktreeNameByID(worktreeID string) string {
	if s.worktreeNames == nil {
		return ""
	}
	return s.worktreeNames[worktreeID]
}

func (s stubGuidedWorkflowLabelResolver) sessionDisplayName(sessionID string) string {
	if s.sessionNames == nil {
		return ""
	}
	return s.sessionNames[sessionID]
}

type stubGuidedWorkflowStartRule struct {
	kind       SelectionKind
	context    guidedWorkflowLaunchContext
	validation string
}

func (s stubGuidedWorkflowStartRule) Kind() SelectionKind {
	return s.kind
}

func (s stubGuidedWorkflowStartRule) Resolve(SelectionTarget) (guidedWorkflowLaunchContext, string) {
	return s.context, s.validation
}

type stubGuidedWorkflowStartService struct {
	context    guidedWorkflowLaunchContext
	validation string
}

func (s stubGuidedWorkflowStartService) ResolveLaunchContext(SelectionTarget, GuidedWorkflowNameHints) (guidedWorkflowLaunchContext, string) {
	return s.context, s.validation
}

func TestDefaultGuidedWorkflowStartServiceWorkspaceSelection(t *testing.T) {
	service := NewDefaultGuidedWorkflowStartService(stubGuidedWorkflowLabelResolver{
		workspaceNames: map[string]string{"ws1": "Payments Workspace"},
	})

	context, validation := service.ResolveLaunchContext(
		SelectionTarget{Kind: SelectionKindWorkspace, WorkspaceID: "ws1"},
		GuidedWorkflowNameHints{},
	)
	if validation != "" {
		t.Fatalf("expected no validation error, got %q", validation)
	}
	if context.workspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", context.workspaceID)
	}
	if context.workspaceName != "Payments Workspace" {
		t.Fatalf("expected resolved workspace name, got %q", context.workspaceName)
	}
}

func TestDefaultGuidedWorkflowStartServiceSessionRequiresContext(t *testing.T) {
	service := NewDefaultGuidedWorkflowStartService(nil)

	_, validation := service.ResolveLaunchContext(
		SelectionTarget{Kind: SelectionKindSession, SessionID: "s1"},
		GuidedWorkflowNameHints{},
	)
	if validation != "session has no workspace/worktree context" {
		t.Fatalf("unexpected validation %q", validation)
	}
}

func TestDefaultGuidedWorkflowStartServiceHintsOverrideLabels(t *testing.T) {
	service := NewDefaultGuidedWorkflowStartService(stubGuidedWorkflowLabelResolver{
		workspaceNames: map[string]string{"ws1": "Workspace Label"},
		worktreeNames:  map[string]string{"wt1": "Worktree Label"},
		sessionNames:   map[string]string{"s1": "Session Label"},
	})

	context, validation := service.ResolveLaunchContext(
		SelectionTarget{
			Kind:        SelectionKindSession,
			WorkspaceID: "ws1",
			WorktreeID:  "wt1",
			SessionID:   "s1",
		},
		GuidedWorkflowNameHints{
			WorkspaceName: "Workspace Hint",
			WorktreeName:  "Worktree Hint",
			SessionName:   "Session Hint",
		},
	)
	if validation != "" {
		t.Fatalf("expected no validation error, got %q", validation)
	}
	if context.workspaceName != "Workspace Hint" {
		t.Fatalf("expected workspace hint to win, got %q", context.workspaceName)
	}
	if context.worktreeName != "Worktree Hint" {
		t.Fatalf("expected worktree hint to win, got %q", context.worktreeName)
	}
	if context.sessionName != "Session Hint" {
		t.Fatalf("expected session hint to win, got %q", context.sessionName)
	}
}

func TestDefaultGuidedWorkflowStartServiceUnknownSelectionKind(t *testing.T) {
	service := NewDefaultGuidedWorkflowStartService(nil)

	_, validation := service.ResolveLaunchContext(
		SelectionTarget{Kind: SelectionKindUnknown},
		GuidedWorkflowNameHints{},
	)
	if validation != guidedWorkflowStartSelectionMessage {
		t.Fatalf("unexpected validation %q", validation)
	}
}

func TestDefaultGuidedWorkflowStartServiceRejectsUnassignedWorkspace(t *testing.T) {
	service := NewDefaultGuidedWorkflowStartService(nil)

	_, validation := service.ResolveLaunchContext(
		SelectionTarget{Kind: SelectionKindWorkspace, WorkspaceID: unassignedWorkspaceID},
		GuidedWorkflowNameHints{},
	)
	if validation != "select a workspace" {
		t.Fatalf("unexpected validation %q", validation)
	}
}

func TestDefaultGuidedWorkflowStartServiceWorktreeAllowsUnassignedWorkspaceWhenWorktreeProvided(t *testing.T) {
	service := NewDefaultGuidedWorkflowStartService(nil)

	context, validation := service.ResolveLaunchContext(
		SelectionTarget{Kind: SelectionKindWorktree, WorkspaceID: unassignedWorkspaceID, WorktreeID: "wt1"},
		GuidedWorkflowNameHints{},
	)
	if validation != "" {
		t.Fatalf("expected no validation error, got %q", validation)
	}
	if context.workspaceID != "" {
		t.Fatalf("expected unassigned workspace to normalize to empty, got %q", context.workspaceID)
	}
	if context.worktreeID != "wt1" {
		t.Fatalf("expected worktree id wt1, got %q", context.worktreeID)
	}
}

func TestDefaultGuidedWorkflowStartServiceWorktreeRequiresSelection(t *testing.T) {
	service := NewDefaultGuidedWorkflowStartService(nil)

	_, validation := service.ResolveLaunchContext(
		SelectionTarget{Kind: SelectionKindWorktree},
		GuidedWorkflowNameHints{},
	)
	if validation != "select a worktree" {
		t.Fatalf("unexpected validation %q", validation)
	}
}

func TestDefaultGuidedWorkflowStartServiceSessionRequiresSessionID(t *testing.T) {
	service := NewDefaultGuidedWorkflowStartService(nil)

	_, validation := service.ResolveLaunchContext(
		SelectionTarget{Kind: SelectionKindSession, WorkspaceID: "ws1"},
		GuidedWorkflowNameHints{},
	)
	if validation != "select a session" {
		t.Fatalf("unexpected validation %q", validation)
	}
}

func TestGuidedWorkflowStartServiceSupportsRuleExtension(t *testing.T) {
	service := NewGuidedWorkflowStartService(nil, stubGuidedWorkflowStartRule{
		kind: SelectionKindWorkflow,
		context: guidedWorkflowLaunchContext{
			workspaceID: "ws1",
			worktreeID:  "wt1",
			sessionID:   "s1",
		},
	})

	context, validation := service.ResolveLaunchContext(
		SelectionTarget{Kind: SelectionKindWorkflow, WorkflowRunID: "gwf-1"},
		GuidedWorkflowNameHints{},
	)
	if validation != "" {
		t.Fatalf("expected no validation error, got %q", validation)
	}
	if context.workspaceID != "ws1" || context.worktreeID != "wt1" || context.sessionID != "s1" {
		t.Fatalf("unexpected context: %#v", context)
	}
}

func TestGuidedWorkflowStartServiceConstructorSkipsNilRules(t *testing.T) {
	service := NewGuidedWorkflowStartService(nil, nil)

	_, validation := service.ResolveLaunchContext(
		SelectionTarget{Kind: SelectionKindWorkspace, WorkspaceID: "ws1"},
		GuidedWorkflowNameHints{},
	)
	if validation != guidedWorkflowStartSelectionMessage {
		t.Fatalf("unexpected validation %q", validation)
	}
}

func TestGuidedWorkflowStartServiceModelOptionsInstallAndResetDefaults(t *testing.T) {
	custom := stubGuidedWorkflowStartService{
		context: guidedWorkflowLaunchContext{workspaceID: "ws-custom"},
	}
	m := NewModel(nil, WithGuidedWorkflowStartService(custom))
	if _, ok := m.guidedWorkflowStartService.(stubGuidedWorkflowStartService); !ok {
		t.Fatalf("expected custom guided workflow start service, got %T", m.guidedWorkflowStartService)
	}
	if resolved, ok := m.guidedWorkflowStartServiceOrDefault().(stubGuidedWorkflowStartService); !ok {
		t.Fatalf("expected resolved custom guided workflow start service, got %T", m.guidedWorkflowStartServiceOrDefault())
	} else if resolved.context.workspaceID != "ws-custom" {
		t.Fatalf("expected custom service workspace id ws-custom, got %q", resolved.context.workspaceID)
	}

	mDefault := NewModel(nil, WithGuidedWorkflowStartService(nil))
	if mDefault.guidedWorkflowStartService != nil {
		t.Fatalf("expected explicit nil option to clear concrete service")
	}
	if mDefault.guidedWorkflowStartServiceOrDefault() == nil {
		t.Fatalf("expected guided workflow start default service fallback")
	}

	opt := WithGuidedWorkflowStartService(custom)
	var nilModel *Model
	opt(nilModel)
	if nilModel.guidedWorkflowStartServiceOrDefault() == nil {
		t.Fatalf("expected nil model fallback to still return default service")
	}
}
