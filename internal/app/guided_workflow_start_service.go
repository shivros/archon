package app

import "strings"

const guidedWorkflowStartSelectionMessage = "select a workspace, worktree, or session to start guided workflow"

type GuidedWorkflowNameHints struct {
	WorkspaceName string
	WorktreeName  string
	SessionName   string
}

type GuidedWorkflowStartService interface {
	ResolveLaunchContext(target SelectionTarget, hints GuidedWorkflowNameHints) (guidedWorkflowLaunchContext, string)
}

type guidedWorkflowLabelResolver interface {
	workspaceNameByID(workspaceID string) string
	worktreeNameByID(worktreeID string) string
	sessionDisplayName(sessionID string) string
}

type GuidedWorkflowStartRule interface {
	Kind() SelectionKind
	Resolve(target SelectionTarget) (guidedWorkflowLaunchContext, string)
}

type guidedWorkflowStartService struct {
	labels guidedWorkflowLabelResolver
	rules  map[SelectionKind]GuidedWorkflowStartRule
}

type guidedWorkflowWorkspaceStartRule struct{}

func (guidedWorkflowWorkspaceStartRule) Kind() SelectionKind {
	return SelectionKindWorkspace
}

func (guidedWorkflowWorkspaceStartRule) Resolve(target SelectionTarget) (guidedWorkflowLaunchContext, string) {
	workspaceID := normalizeGuidedWorkflowWorkspaceID(target.WorkspaceID)
	if workspaceID == "" {
		return guidedWorkflowLaunchContext{}, "select a workspace"
	}
	return guidedWorkflowLaunchContext{workspaceID: workspaceID}, ""
}

type guidedWorkflowWorktreeStartRule struct{}

func (guidedWorkflowWorktreeStartRule) Kind() SelectionKind {
	return SelectionKindWorktree
}

func (guidedWorkflowWorktreeStartRule) Resolve(target SelectionTarget) (guidedWorkflowLaunchContext, string) {
	workspaceID := normalizeGuidedWorkflowWorkspaceID(target.WorkspaceID)
	worktreeID := strings.TrimSpace(target.WorktreeID)
	if workspaceID == "" && worktreeID == "" {
		return guidedWorkflowLaunchContext{}, "select a worktree"
	}
	return guidedWorkflowLaunchContext{
		workspaceID: workspaceID,
		worktreeID:  worktreeID,
	}, ""
}

type guidedWorkflowSessionStartRule struct{}

func (guidedWorkflowSessionStartRule) Kind() SelectionKind {
	return SelectionKindSession
}

func (guidedWorkflowSessionStartRule) Resolve(target SelectionTarget) (guidedWorkflowLaunchContext, string) {
	sessionID := strings.TrimSpace(target.SessionID)
	if sessionID == "" {
		return guidedWorkflowLaunchContext{}, "select a session"
	}
	workspaceID := normalizeGuidedWorkflowWorkspaceID(target.WorkspaceID)
	worktreeID := strings.TrimSpace(target.WorktreeID)
	if workspaceID == "" && worktreeID == "" {
		return guidedWorkflowLaunchContext{}, "session has no workspace/worktree context"
	}
	return guidedWorkflowLaunchContext{
		workspaceID: workspaceID,
		worktreeID:  worktreeID,
		sessionID:   sessionID,
	}, ""
}

func NewGuidedWorkflowStartService(labels guidedWorkflowLabelResolver, rules ...GuidedWorkflowStartRule) GuidedWorkflowStartService {
	registry := map[SelectionKind]GuidedWorkflowStartRule{}
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		registry[rule.Kind()] = rule
	}
	return guidedWorkflowStartService{
		labels: labels,
		rules:  registry,
	}
}

func defaultGuidedWorkflowStartRules() []GuidedWorkflowStartRule {
	return []GuidedWorkflowStartRule{
		guidedWorkflowWorkspaceStartRule{},
		guidedWorkflowWorktreeStartRule{},
		guidedWorkflowSessionStartRule{},
	}
}

func NewDefaultGuidedWorkflowStartService(labels guidedWorkflowLabelResolver) GuidedWorkflowStartService {
	return NewGuidedWorkflowStartService(labels, defaultGuidedWorkflowStartRules()...)
}

func (s guidedWorkflowStartService) ResolveLaunchContext(target SelectionTarget, hints GuidedWorkflowNameHints) (guidedWorkflowLaunchContext, string) {
	rule, ok := s.rules[target.Kind]
	if !ok || rule == nil {
		return guidedWorkflowLaunchContext{}, guidedWorkflowStartSelectionMessage
	}
	context, validation := rule.Resolve(target)
	if strings.TrimSpace(validation) != "" {
		return guidedWorkflowLaunchContext{}, validation
	}
	context.workspaceID = normalizeGuidedWorkflowWorkspaceID(context.workspaceID)
	context.worktreeID = strings.TrimSpace(context.worktreeID)
	context.sessionID = strings.TrimSpace(context.sessionID)
	context.workspaceName = strings.TrimSpace(context.workspaceName)
	context.worktreeName = strings.TrimSpace(context.worktreeName)
	context.sessionName = strings.TrimSpace(context.sessionName)

	if s.labels != nil {
		if context.workspaceName == "" {
			context.workspaceName = strings.TrimSpace(s.labels.workspaceNameByID(context.workspaceID))
		}
		if context.worktreeName == "" {
			context.worktreeName = strings.TrimSpace(s.labels.worktreeNameByID(context.worktreeID))
		}
		if context.sessionName == "" {
			context.sessionName = strings.TrimSpace(s.labels.sessionDisplayName(context.sessionID))
		}
	}

	if workspaceName := strings.TrimSpace(hints.WorkspaceName); workspaceName != "" {
		context.workspaceName = workspaceName
	}
	if worktreeName := strings.TrimSpace(hints.WorktreeName); worktreeName != "" {
		context.worktreeName = worktreeName
	}
	if sessionName := strings.TrimSpace(hints.SessionName); sessionName != "" {
		context.sessionName = sessionName
	}

	return context, ""
}

func normalizeGuidedWorkflowWorkspaceID(workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == unassignedWorkspaceID {
		return ""
	}
	return workspaceID
}

func WithGuidedWorkflowStartService(service GuidedWorkflowStartService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.guidedWorkflowStartService = service
	}
}

func (m *Model) guidedWorkflowStartServiceOrDefault() GuidedWorkflowStartService {
	if m == nil || m.guidedWorkflowStartService == nil {
		return NewDefaultGuidedWorkflowStartService(m)
	}
	return m.guidedWorkflowStartService
}
