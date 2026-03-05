package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type sidebarExpansionIntentKind int

const (
	sidebarExpansionIntentNone sidebarExpansionIntentKind = iota
	sidebarExpansionIntentSingleToggle
	sidebarExpansionIntentAllWorkspaces
	sidebarExpansionIntentWorktreesForWorktree
	sidebarExpansionIntentWorkflowsForWorkspace
	sidebarExpansionIntentWorkflowsForWorktree
)

type sidebarExpansionIntent struct {
	kind        sidebarExpansionIntentKind
	expanded    bool
	workspaceID string
	worktreeID  string
}

type SidebarExpansionIntentPolicy interface {
	ResolveIntent(entry *sidebarItem, mouse tea.Mouse) sidebarExpansionIntent
}

type SidebarExpansionService interface {
	ApplyIntent(sidebar SidebarExpansionMutator, intent sidebarExpansionIntent) bool
}

type SidebarExpansionMutator interface {
	ToggleSelectedContainer() bool
	SetAllWorkspacesExpanded(expanded bool) bool
	SetWorktreesExpandedForWorktree(worktreeID string, expanded bool) bool
	SetWorkflowsExpandedForWorkspace(workspaceID string, expanded bool) bool
	SetWorkflowsExpandedForWorktree(worktreeID string, expanded bool) bool
}

type defaultSidebarExpansionIntentPolicy struct{}

func (defaultSidebarExpansionIntentPolicy) ResolveIntent(entry *sidebarItem, mouse tea.Mouse) sidebarExpansionIntent {
	if entry == nil {
		return sidebarExpansionIntent{kind: sidebarExpansionIntentNone}
	}
	intent := sidebarExpansionIntent{
		kind:     sidebarExpansionIntentSingleToggle,
		expanded: !entry.expanded,
	}
	if !mouse.Mod.Contains(tea.ModCtrl) {
		return intent
	}
	switch entry.kind {
	case sidebarWorkspace:
		intent.kind = sidebarExpansionIntentAllWorkspaces
	case sidebarWorktree:
		if entry.worktree == nil {
			intent.kind = sidebarExpansionIntentNone
			return intent
		}
		intent.kind = sidebarExpansionIntentWorktreesForWorktree
		intent.worktreeID = strings.TrimSpace(entry.worktree.ID)
	case sidebarWorkflow:
		if strings.TrimSpace(entry.workflowRunID()) == "" {
			intent.kind = sidebarExpansionIntentNone
			return intent
		}
		worktreeID := strings.TrimSpace(entry.workflowWorktreeID())
		if worktreeID != "" {
			intent.kind = sidebarExpansionIntentWorkflowsForWorktree
			intent.worktreeID = worktreeID
			return intent
		}
		workspaceID := strings.TrimSpace(entry.workflowWorkspaceID())
		if workspaceID == "" {
			workspaceID = unassignedWorkspaceID
		}
		if workspaceID == "" {
			intent.kind = sidebarExpansionIntentNone
			return intent
		}
		intent.kind = sidebarExpansionIntentWorkflowsForWorkspace
		intent.workspaceID = workspaceID
	default:
		intent.kind = sidebarExpansionIntentSingleToggle
	}
	return intent
}

type defaultSidebarExpansionService struct{}

func (defaultSidebarExpansionService) ApplyIntent(sidebar SidebarExpansionMutator, intent sidebarExpansionIntent) bool {
	if sidebar == nil {
		return false
	}
	switch intent.kind {
	case sidebarExpansionIntentSingleToggle:
		return sidebar.ToggleSelectedContainer()
	case sidebarExpansionIntentAllWorkspaces:
		return sidebar.SetAllWorkspacesExpanded(intent.expanded)
	case sidebarExpansionIntentWorktreesForWorktree:
		if strings.TrimSpace(intent.worktreeID) == "" {
			return false
		}
		return sidebar.SetWorktreesExpandedForWorktree(intent.worktreeID, intent.expanded)
	case sidebarExpansionIntentWorkflowsForWorkspace:
		workspaceID := strings.TrimSpace(intent.workspaceID)
		if workspaceID == "" {
			return false
		}
		return sidebar.SetWorkflowsExpandedForWorkspace(workspaceID, intent.expanded)
	case sidebarExpansionIntentWorkflowsForWorktree:
		worktreeID := strings.TrimSpace(intent.worktreeID)
		if worktreeID == "" {
			return false
		}
		return sidebar.SetWorkflowsExpandedForWorktree(worktreeID, intent.expanded)
	default:
		return false
	}
}

func WithSidebarExpansionIntentPolicy(policy SidebarExpansionIntentPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sidebarExpansionIntentPolicy = defaultSidebarExpansionIntentPolicy{}
			return
		}
		m.sidebarExpansionIntentPolicy = policy
	}
}

func WithSidebarExpansionService(service SidebarExpansionService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if service == nil {
			m.sidebarExpansionService = defaultSidebarExpansionService{}
			return
		}
		m.sidebarExpansionService = service
	}
}

func (m *Model) sidebarExpansionIntentPolicyOrDefault() SidebarExpansionIntentPolicy {
	if m == nil || m.sidebarExpansionIntentPolicy == nil {
		return defaultSidebarExpansionIntentPolicy{}
	}
	return m.sidebarExpansionIntentPolicy
}

func (m *Model) sidebarExpansionServiceOrDefault() SidebarExpansionService {
	if m == nil || m.sidebarExpansionService == nil {
		return defaultSidebarExpansionService{}
	}
	return m.sidebarExpansionService
}
