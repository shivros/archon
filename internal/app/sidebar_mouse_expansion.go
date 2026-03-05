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
	sidebarExpansionIntentSelectedWorkflows
)

type sidebarExpansionIntent struct {
	kind       sidebarExpansionIntentKind
	expanded   bool
	worktreeID string
	workflowID string
}

type SidebarExpansionIntentPolicy interface {
	ResolveIntent(entry *sidebarItem, mouse tea.Mouse) sidebarExpansionIntent
}

type SidebarExpansionService interface {
	ApplyIntent(sidebar SidebarExpansionTarget, intent sidebarExpansionIntent) bool
}

type SidebarExpansionMutator interface {
	ToggleSelectedContainer() bool
	SetAllWorkspacesExpanded(expanded bool) bool
	SetWorktreesExpandedForWorktree(worktreeID string, expanded bool) bool
	SetWorkflowsExpanded(workflowIDs []string, expanded bool) bool
}

type SidebarWorkflowSelectionReader interface {
	SelectedKeys() []string
}

type SidebarExpansionTarget interface {
	SidebarExpansionMutator
	SidebarWorkflowSelectionReader
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
		workflowID := strings.TrimSpace(entry.workflowRunID())
		if workflowID == "" {
			intent.kind = sidebarExpansionIntentNone
			return intent
		}
		intent.kind = sidebarExpansionIntentSelectedWorkflows
		intent.workflowID = workflowID
	default:
		intent.kind = sidebarExpansionIntentSingleToggle
	}
	return intent
}

type defaultSidebarExpansionService struct{}

func (defaultSidebarExpansionService) ApplyIntent(sidebar SidebarExpansionTarget, intent sidebarExpansionIntent) bool {
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
	case sidebarExpansionIntentSelectedWorkflows:
		workflowIDs := selectedWorkflowIDs(sidebar.SelectedKeys())
		if len(workflowIDs) == 0 {
			fallback := strings.TrimSpace(intent.workflowID)
			if fallback == "" {
				return false
			}
			workflowIDs = []string{fallback}
		}
		return sidebar.SetWorkflowsExpanded(workflowIDs, intent.expanded)
	default:
		return false
	}
}

func selectedWorkflowIDs(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	ids := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if !strings.HasPrefix(key, "gwf:") {
			continue
		}
		id := strings.TrimSpace(strings.TrimPrefix(key, "gwf:"))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
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
