package app

import (
	"strings"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type SidebarWorkflowScopeInput struct {
	Workspaces   []*types.Workspace
	Worktrees    map[string][]*types.Worktree
	Sessions     []*types.Session
	WorkflowRuns []*guidedworkflows.WorkflowRun
	Meta         map[string]*types.SessionMeta
}

type SidebarWorkflowScopeResolver interface {
	WorkflowIDsForWorkspace(input SidebarWorkflowScopeInput, workspaceID string) []string
	WorkflowIDsForWorktree(input SidebarWorkflowScopeInput, worktreeID string) []string
}

type defaultSidebarWorkflowScopeResolver struct{}

func NewDefaultSidebarWorkflowScopeResolver() SidebarWorkflowScopeResolver {
	return defaultSidebarWorkflowScopeResolver{}
}

func (defaultSidebarWorkflowScopeResolver) WorkflowIDsForWorkspace(input SidebarWorkflowScopeInput, workspaceID string) []string {
	workspaceID = normalizeWorkflowScopeWorkspaceID(workspaceID)
	if workspaceID == "" {
		return nil
	}
	knownWorkspaces, knownWorktrees := knownWorkflowScopeRoots(input.Workspaces, input.Worktrees)
	seen := map[string]struct{}{}
	ids := []string{}

	appendID := func(id, resolvedWorkspaceID, resolvedWorktreeID string) {
		id = strings.TrimSpace(id)
		if id == "" || strings.TrimSpace(resolvedWorktreeID) != "" {
			return
		}
		if normalizeWorkflowScopeWorkspaceID(resolvedWorkspaceID) != workspaceID {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	for _, run := range input.WorkflowRuns {
		runID, resolvedWorkspaceID, resolvedWorktreeID := resolveWorkflowRunScope(run, knownWorkspaces, knownWorktrees)
		appendID(runID, resolvedWorkspaceID, resolvedWorktreeID)
	}
	for _, session := range input.Sessions {
		workflowID, resolvedWorkspaceID, resolvedWorktreeID := resolveSessionWorkflowScope(session, input.Meta, knownWorkspaces, knownWorktrees)
		appendID(workflowID, resolvedWorkspaceID, resolvedWorktreeID)
	}
	return ids
}

func (defaultSidebarWorkflowScopeResolver) WorkflowIDsForWorktree(input SidebarWorkflowScopeInput, worktreeID string) []string {
	worktreeID = strings.TrimSpace(worktreeID)
	if worktreeID == "" {
		return nil
	}
	knownWorkspaces, knownWorktrees := knownWorkflowScopeRoots(input.Workspaces, input.Worktrees)
	seen := map[string]struct{}{}
	ids := []string{}

	appendID := func(id, resolvedWorktreeID string) {
		id = strings.TrimSpace(id)
		if id == "" || strings.TrimSpace(resolvedWorktreeID) != worktreeID {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	for _, run := range input.WorkflowRuns {
		runID, _, resolvedWorktreeID := resolveWorkflowRunScope(run, knownWorkspaces, knownWorktrees)
		appendID(runID, resolvedWorktreeID)
	}
	for _, session := range input.Sessions {
		workflowID, _, resolvedWorktreeID := resolveSessionWorkflowScope(session, input.Meta, knownWorkspaces, knownWorktrees)
		appendID(workflowID, resolvedWorktreeID)
	}
	return ids
}

func knownWorkflowScopeRoots(workspaces []*types.Workspace, worktrees map[string][]*types.Worktree) (map[string]struct{}, map[string]string) {
	knownWorkspaces := map[string]struct{}{}
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		id := strings.TrimSpace(ws.ID)
		if id == "" {
			continue
		}
		knownWorkspaces[id] = struct{}{}
	}
	knownWorktrees := map[string]string{}
	for workspaceID, entries := range worktrees {
		workspaceID = strings.TrimSpace(workspaceID)
		if workspaceID == "" {
			continue
		}
		for _, wt := range entries {
			if wt == nil {
				continue
			}
			worktreeID := strings.TrimSpace(wt.ID)
			if worktreeID == "" {
				continue
			}
			knownWorktrees[worktreeID] = workspaceID
		}
	}
	return knownWorkspaces, knownWorktrees
}

func resolveWorkflowRunScope(run *guidedworkflows.WorkflowRun, knownWorkspaces map[string]struct{}, knownWorktrees map[string]string) (string, string, string) {
	if run == nil {
		return "", "", ""
	}
	runID := strings.TrimSpace(run.ID)
	workspaceID := strings.TrimSpace(run.WorkspaceID)
	worktreeID := strings.TrimSpace(run.WorktreeID)
	workspaceID, worktreeID = normalizeWorkflowScope(workspaceID, worktreeID, knownWorkspaces, knownWorktrees)
	return runID, workspaceID, worktreeID
}

func resolveSessionWorkflowScope(session *types.Session, meta map[string]*types.SessionMeta, knownWorkspaces map[string]struct{}, knownWorktrees map[string]string) (string, string, string) {
	if session == nil {
		return "", "", ""
	}
	entry := meta[session.ID]
	if entry == nil {
		return "", "", ""
	}
	workflowID := strings.TrimSpace(entry.WorkflowRunID)
	if workflowID == "" {
		return "", "", ""
	}
	workspaceID := strings.TrimSpace(entry.WorkspaceID)
	worktreeID := strings.TrimSpace(entry.WorktreeID)
	workspaceID, worktreeID = normalizeWorkflowScope(workspaceID, worktreeID, knownWorkspaces, knownWorktrees)
	return workflowID, workspaceID, worktreeID
}

func normalizeWorkflowScope(workspaceID, worktreeID string, knownWorkspaces map[string]struct{}, knownWorktrees map[string]string) (string, string) {
	workspaceID = strings.TrimSpace(workspaceID)
	worktreeID = strings.TrimSpace(worktreeID)
	if workspaceID != "" {
		if _, ok := knownWorkspaces[workspaceID]; !ok {
			workspaceID = ""
		}
	}
	if worktreeID != "" {
		if owner, ok := knownWorktrees[worktreeID]; ok {
			if workspaceID == "" {
				workspaceID = owner
			}
		} else {
			worktreeID = ""
		}
	}
	return workspaceID, worktreeID
}

func normalizeWorkflowScopeWorkspaceID(workspaceID string) string {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return unassignedWorkspaceID
	}
	return workspaceID
}
