package app

import (
	"strings"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type sidebarProjectionChangeReason string

const (
	sidebarProjectionChangeSessions  sidebarProjectionChangeReason = "sessions"
	sidebarProjectionChangeMeta      sidebarProjectionChangeReason = "meta"
	sidebarProjectionChangeWorkspace sidebarProjectionChangeReason = "workspace"
	sidebarProjectionChangeWorktree  sidebarProjectionChangeReason = "worktree"
	sidebarProjectionChangeGroup     sidebarProjectionChangeReason = "group"
	sidebarProjectionChangeDismissed sidebarProjectionChangeReason = "dismissed"
	sidebarProjectionChangeWorkflow  sidebarProjectionChangeReason = "workflow"
	sidebarProjectionChangeAppState  sidebarProjectionChangeReason = "app_state"
)

type SidebarProjectionInput struct {
	Workspaces         []*types.Workspace
	Worktrees          map[string][]*types.Worktree
	Sessions           []*types.Session
	SessionMeta        map[string]*types.SessionMeta
	WorkflowRuns       []*guidedworkflows.WorkflowRun
	ActiveWorkspaceIDs []string
}

type SidebarProjection struct {
	Workspaces   []*types.Workspace
	Sessions     []*types.Session
	WorkflowRuns []*guidedworkflows.WorkflowRun
}

type SidebarProjectionBuilder interface {
	Build(input SidebarProjectionInput) SidebarProjection
}

type SidebarProjectionInvalidationPolicy interface {
	ShouldInvalidate(reason sidebarProjectionChangeReason) bool
}

type defaultSidebarProjectionBuilder struct{}

func NewDefaultSidebarProjectionBuilder() SidebarProjectionBuilder {
	return defaultSidebarProjectionBuilder{}
}

func (defaultSidebarProjectionBuilder) Build(input SidebarProjectionInput) SidebarProjection {
	selected := selectedWorkspaceGroups(input.ActiveWorkspaceIDs)
	if len(selected) == 0 {
		return SidebarProjection{
			Workspaces:   []*types.Workspace{},
			Sessions:     []*types.Session{},
			WorkflowRuns: []*guidedworkflows.WorkflowRun{},
		}
	}
	workspaces := filterWorkspacesForSidebar(input.Workspaces, selected)
	sessions := filterSessionsForSidebar(input.Sessions, input.SessionMeta, input.Worktrees, workspaces, selected)
	workflowRuns := filterWorkflowRunsForSidebar(input.WorkflowRuns, input.Worktrees, workspaces, selected)
	return SidebarProjection{
		Workspaces:   workspaces,
		Sessions:     sessions,
		WorkflowRuns: workflowRuns,
	}
}

func selectedWorkspaceGroups(ids []string) map[string]bool {
	selected := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		selected[id] = true
	}
	return selected
}

func filterWorkspacesForSidebar(workspaces []*types.Workspace, selected map[string]bool) []*types.Workspace {
	if len(selected) == 0 {
		return []*types.Workspace{}
	}
	out := make([]*types.Workspace, 0, len(workspaces))
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		groupIDs := ws.GroupIDs
		if len(groupIDs) == 0 {
			if selected["ungrouped"] {
				out = append(out, ws)
			}
			continue
		}
		for _, id := range groupIDs {
			if selected[id] {
				out = append(out, ws)
				break
			}
		}
	}
	return out
}

func filterSessionsForSidebar(
	sessions []*types.Session,
	meta map[string]*types.SessionMeta,
	worktrees map[string][]*types.Worktree,
	workspaces []*types.Workspace,
	selected map[string]bool,
) []*types.Session {
	if len(selected) == 0 {
		return []*types.Session{}
	}
	visibleWorkspaces := map[string]struct{}{}
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		visibleWorkspaces[ws.ID] = struct{}{}
	}
	visibleWorktrees := map[string]struct{}{}
	for wsID := range visibleWorkspaces {
		for _, wt := range worktrees[wsID] {
			if wt == nil {
				continue
			}
			visibleWorktrees[wt.ID] = struct{}{}
		}
	}
	out := make([]*types.Session, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		entry := meta[session.ID]
		workspaceID := ""
		worktreeID := ""
		if entry != nil {
			workspaceID = strings.TrimSpace(entry.WorkspaceID)
			worktreeID = strings.TrimSpace(entry.WorktreeID)
		}
		if worktreeID != "" {
			if _, ok := visibleWorktrees[worktreeID]; ok {
				out = append(out, session)
			} else if workspaceID != "" {
				// Worktree visibility can be stale between async worktree refreshes.
				// Fall back to workspace visibility instead of transiently hiding items.
				if _, ok := visibleWorkspaces[workspaceID]; ok {
					out = append(out, session)
				}
			}
			continue
		}
		if workspaceID != "" {
			if _, ok := visibleWorkspaces[workspaceID]; ok {
				out = append(out, session)
			}
			continue
		}
		if selected["ungrouped"] {
			out = append(out, session)
		}
	}
	return out
}

func filterWorkflowRunsForSidebar(
	runs []*guidedworkflows.WorkflowRun,
	worktrees map[string][]*types.Worktree,
	workspaces []*types.Workspace,
	selected map[string]bool,
) []*guidedworkflows.WorkflowRun {
	if len(selected) == 0 {
		return []*guidedworkflows.WorkflowRun{}
	}
	visibleWorkspaces := map[string]struct{}{}
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		visibleWorkspaces[ws.ID] = struct{}{}
	}
	visibleWorktrees := map[string]struct{}{}
	for wsID := range visibleWorkspaces {
		for _, wt := range worktrees[wsID] {
			if wt == nil {
				continue
			}
			visibleWorktrees[wt.ID] = struct{}{}
		}
	}
	out := make([]*guidedworkflows.WorkflowRun, 0, len(runs))
	for _, run := range runs {
		if run == nil {
			continue
		}
		workspaceID := strings.TrimSpace(run.WorkspaceID)
		worktreeID := strings.TrimSpace(run.WorktreeID)
		if worktreeID != "" {
			if _, ok := visibleWorktrees[worktreeID]; ok {
				out = append(out, run)
			} else if workspaceID != "" {
				// Worktree visibility can be stale between async worktree refreshes.
				// Fall back to workspace visibility instead of transiently hiding items.
				if _, ok := visibleWorkspaces[workspaceID]; ok {
					out = append(out, run)
				}
			}
			continue
		}
		if workspaceID != "" {
			if _, ok := visibleWorkspaces[workspaceID]; ok {
				out = append(out, run)
			}
			continue
		}
		if selected["ungrouped"] {
			out = append(out, run)
		}
	}
	return out
}

func WithSidebarProjectionBuilder(builder SidebarProjectionBuilder) ModelOption {
	return func(m *Model) {
		if m == nil || builder == nil {
			return
		}
		m.sidebarProjectionBuilder = builder
	}
}

type defaultSidebarProjectionInvalidationPolicy struct{}

func NewDefaultSidebarProjectionInvalidationPolicy() SidebarProjectionInvalidationPolicy {
	return defaultSidebarProjectionInvalidationPolicy{}
}

func (defaultSidebarProjectionInvalidationPolicy) ShouldInvalidate(reason sidebarProjectionChangeReason) bool {
	switch reason {
	case sidebarProjectionChangeSessions,
		sidebarProjectionChangeMeta,
		sidebarProjectionChangeWorkspace,
		sidebarProjectionChangeWorktree,
		sidebarProjectionChangeGroup,
		sidebarProjectionChangeDismissed,
		sidebarProjectionChangeWorkflow,
		sidebarProjectionChangeAppState:
		return true
	default:
		return false
	}
}

func WithSidebarProjectionInvalidationPolicy(policy SidebarProjectionInvalidationPolicy) ModelOption {
	return func(m *Model) {
		if m == nil || policy == nil {
			return
		}
		m.sidebarProjectionInvalidationPolicy = policy
	}
}
