package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type contextMenuTarget struct {
	id                string
	targetLabel       string
	workspaceID       string
	worktreeID        string
	sessionID         string
	workflowID        string
	workflowStatus    guidedworkflows.WorkflowRunStatus
	workflowDismissed bool
}

func contextMenuTargetSidebarKey(target contextMenuTarget) string {
	switch {
	case strings.TrimSpace(target.workflowID) != "":
		return "gwf:" + strings.TrimSpace(target.workflowID)
	case strings.TrimSpace(target.sessionID) != "":
		return "sess:" + strings.TrimSpace(target.sessionID)
	case strings.TrimSpace(target.worktreeID) != "":
		return "wt:" + strings.TrimSpace(target.worktreeID)
	case strings.TrimSpace(target.id) != "":
		return "ws:" + strings.TrimSpace(target.id)
	default:
		return ""
	}
}

func (m *Model) copySidebarSelectionIDsForContextTarget(target contextMenuTarget) (tea.Cmd, bool) {
	if m == nil || m.sidebar == nil {
		return nil, false
	}
	if m.sidebar.SelectedKeyCount() <= 1 {
		return nil, false
	}
	key := contextMenuTargetSidebarKey(target)
	if key == "" || !m.sidebar.IsKeySelected(key) {
		return nil, false
	}
	return m.copySidebarSelectionIDsCmd(), true
}

func (m *Model) handleWorkspaceContextMenuAction(action ContextMenuAction, target contextMenuTarget) (bool, tea.Cmd) {
	switch action {
	case ContextMenuWorkspaceCreate:
		m.enterAddWorkspace()
		return true, nil
	case ContextMenuWorkspaceRename:
		if target.id == "" {
			m.setValidationStatus("select a workspace to edit")
			return true, nil
		}
		m.enterEditWorkspace(target.id)
		return true, nil
	case ContextMenuWorkspaceEditGroups:
		if target.id == "" {
			m.setValidationStatus("select a workspace")
			return true, nil
		}
		m.enterEditWorkspaceGroups(target.id)
		return true, nil
	case ContextMenuWorkspaceOpenNotes:
		if target.id == "" || target.id == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace")
			return true, nil
		}
		scope := noteScopeTarget{
			Scope:       types.NoteScopeWorkspace,
			WorkspaceID: target.id,
		}
		return true, m.openNotesScope(scope)
	case ContextMenuWorkspaceAddNote:
		if target.id == "" || target.id == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace")
			return true, nil
		}
		scope := noteScopeTarget{
			Scope:       types.NoteScopeWorkspace,
			WorkspaceID: target.id,
		}
		return true, m.enterAddNoteForScope(scope)
	case ContextMenuWorkspaceAddWorktree:
		if target.id == "" || target.id == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace")
			return true, nil
		}
		m.enterAddWorktree(target.id)
		return true, nil
	case ContextMenuWorkspaceStartGuidedWorkflow:
		return true, m.startGuidedWorkflowFromSelectionTarget(
			SelectionTarget{
				Kind:        SelectionKindWorkspace,
				WorkspaceID: target.id,
			},
			GuidedWorkflowNameHints{
				WorkspaceName: target.targetLabel,
			},
		)
	case ContextMenuWorkspaceCopyPath:
		if target.id == "" || target.id == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace")
			return true, nil
		}
		if cmd, ok := m.copySidebarSelectionIDsForContextTarget(target); ok {
			return true, cmd
		}
		workspace := m.workspaceByID(target.id)
		path := ""
		if workspace != nil {
			path = strings.TrimSpace(workspace.RepoPath)
		}
		if path == "" {
			m.setCopyStatusWarning("workspace path unavailable")
			return true, nil
		}
		return true, m.copyWithStatusCmd(path, "copied workspace path")
	case ContextMenuWorkspaceDelete:
		if target.id == "" || target.id == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace to delete")
			return true, nil
		}
		m.confirmDeleteWorkspace(target.id)
		return true, nil
	default:
		return false, nil
	}
}

func (m *Model) handleWorktreeContextMenuAction(action ContextMenuAction, target contextMenuTarget) (bool, tea.Cmd) {
	switch action {
	case ContextMenuWorktreeAdd:
		if target.workspaceID == "" {
			m.setValidationStatus("select a workspace")
			return true, nil
		}
		m.enterAddWorktree(target.workspaceID)
		return true, nil
	case ContextMenuWorktreeOpenNotes:
		if target.worktreeID == "" || target.workspaceID == "" {
			m.setValidationStatus("select a worktree")
			return true, nil
		}
		scope := noteScopeTarget{
			Scope:       types.NoteScopeWorktree,
			WorkspaceID: target.workspaceID,
			WorktreeID:  target.worktreeID,
		}
		return true, m.openNotesScope(scope)
	case ContextMenuWorktreeAddNote:
		if target.worktreeID == "" || target.workspaceID == "" {
			m.setValidationStatus("select a worktree")
			return true, nil
		}
		scope := noteScopeTarget{
			Scope:       types.NoteScopeWorktree,
			WorkspaceID: target.workspaceID,
			WorktreeID:  target.worktreeID,
		}
		return true, m.enterAddNoteForScope(scope)
	case ContextMenuWorktreeStartGuidedWorkflow:
		return true, m.startGuidedWorkflowFromSelectionTarget(
			SelectionTarget{
				Kind:        SelectionKindWorktree,
				WorkspaceID: target.workspaceID,
				WorktreeID:  target.worktreeID,
			},
			GuidedWorkflowNameHints{
				WorktreeName: target.targetLabel,
			},
		)
	case ContextMenuWorktreeCopyPath:
		if target.worktreeID == "" {
			m.setValidationStatus("select a worktree")
			return true, nil
		}
		worktree := m.worktreeByID(target.worktreeID)
		path := ""
		if worktree != nil {
			path = strings.TrimSpace(worktree.Path)
		}
		if path == "" {
			m.setCopyStatusWarning("worktree path unavailable")
			return true, nil
		}
		return true, m.copyWithStatusCmd(path, "copied worktree path")
	case ContextMenuWorktreeDelete:
		if target.worktreeID == "" || target.workspaceID == "" {
			m.setValidationStatus("select a worktree")
			return true, nil
		}
		m.confirmDeleteWorktree(target.workspaceID, target.worktreeID)
		return true, nil
	default:
		return false, nil
	}
}

func (m *Model) handleSessionContextMenuAction(action ContextMenuAction, target contextMenuTarget) (bool, tea.Cmd) {
	switch action {
	case ContextMenuSessionChat:
		if target.sessionID == "" {
			m.setValidationStatus("select a session")
			return true, nil
		}
		m.enterCompose(target.sessionID)
		return true, nil
	case ContextMenuSessionRename:
		if target.sessionID == "" {
			m.setValidationStatus("select a session")
			return true, nil
		}
		m.enterRenameSession(target.sessionID)
		return true, nil
	case ContextMenuSessionOpenNotes:
		if target.sessionID == "" {
			m.setValidationStatus("select a session")
			return true, nil
		}
		scope := m.noteScopeForSession(target.sessionID, target.workspaceID, target.worktreeID)
		return true, m.openNotesScope(scope)
	case ContextMenuSessionAddNote:
		if target.sessionID == "" {
			m.setValidationStatus("select a session")
			return true, nil
		}
		scope := m.noteScopeForSession(target.sessionID, target.workspaceID, target.worktreeID)
		return true, m.enterAddNoteForScope(scope)
	case ContextMenuSessionStartGuidedWorkflow:
		return true, m.startGuidedWorkflowFromSelectionTarget(
			SelectionTarget{
				Kind:        SelectionKindSession,
				WorkspaceID: target.workspaceID,
				WorktreeID:  target.worktreeID,
				SessionID:   target.sessionID,
			},
			GuidedWorkflowNameHints{
				SessionName: target.targetLabel,
			},
		)
	case ContextMenuSessionDismiss:
		if target.sessionID == "" {
			m.setValidationStatus("select a session")
			return true, nil
		}
		m.confirmDismissSessions([]string{target.sessionID})
		return true, nil
	case ContextMenuSessionKill:
		if target.sessionID == "" {
			m.setValidationStatus("select a session")
			return true, nil
		}
		m.setStatusMessage("killing " + target.sessionID)
		return true, killSessionCmd(m.sessionAPI, target.sessionID)
	case ContextMenuSessionInterrupt:
		if target.sessionID == "" {
			m.setValidationStatus("select a session")
			return true, nil
		}
		m.setStatusMessage("interrupting " + target.sessionID)
		return true, interruptSessionCmd(m.sessionAPI, target.sessionID)
	case ContextMenuSessionCopyID:
		if target.sessionID == "" {
			m.setCopyStatusWarning("select a session")
			return true, nil
		}
		if cmd, ok := m.copySidebarSelectionIDsForContextTarget(target); ok {
			return true, cmd
		}
		return true, m.copyWithStatusCmd(target.sessionID, "copied session id")
	default:
		return false, nil
	}
}

func (m *Model) handleWorkflowContextMenuAction(action ContextMenuAction, target contextMenuTarget) (bool, tea.Cmd) {
	switch action {
	case ContextMenuWorkflowOpen:
		runID := strings.TrimSpace(target.workflowID)
		if runID == "" {
			m.setValidationStatus("select a workflow")
			return true, nil
		}
		if m.sidebar != nil {
			m.sidebar.SelectByWorkflowID(runID)
		}
		item := m.selectedItem()
		if item == nil || item.kind != sidebarWorkflow {
			m.setValidationStatus("workflow not found in sidebar")
			return true, nil
		}
		return true, m.openGuidedWorkflowFromSidebar(item)
	case ContextMenuWorkflowRename:
		runID := strings.TrimSpace(target.workflowID)
		if runID == "" {
			m.setValidationStatus("select a workflow")
			return true, nil
		}
		m.enterRenameWorkflow(runID)
		return true, nil
	case ContextMenuWorkflowCreateFollowUp:
		runID := strings.TrimSpace(target.workflowID)
		if runID == "" {
			m.setValidationStatus("select a workflow")
			return true, nil
		}
		status := target.workflowStatus
		if strings.TrimSpace(string(status)) == "" {
			if resolved, ok := m.workflowRunStatus(runID); ok {
				status = resolved
			}
		}
		if !workflowStatusAllowsFollowUp(status) {
			m.setValidationStatus("follow-up workflows are only available for running, paused, or queued workflows")
			return true, nil
		}
		run := m.workflowRunByID(runID)
		ctx := guidedWorkflowLaunchContext{
			followUpRunID:    runID,
			followUpRunLabel: strings.TrimSpace(target.targetLabel),
			dependencyLocked: true,
		}
		if run != nil {
			ctx.workspaceID = strings.TrimSpace(run.WorkspaceID)
			ctx.worktreeID = strings.TrimSpace(run.WorktreeID)
			ctx.sessionID = strings.TrimSpace(run.SessionID)
		}
		if strings.TrimSpace(ctx.workspaceID) == "" || strings.TrimSpace(ctx.worktreeID) == "" || strings.TrimSpace(ctx.sessionID) == "" {
			workspaceID, worktreeID, sessionID := m.workflowRunContextFromSessions(runID)
			if strings.TrimSpace(ctx.workspaceID) == "" {
				ctx.workspaceID = strings.TrimSpace(workspaceID)
			}
			if strings.TrimSpace(ctx.worktreeID) == "" {
				ctx.worktreeID = strings.TrimSpace(worktreeID)
			}
			if strings.TrimSpace(ctx.sessionID) == "" {
				ctx.sessionID = strings.TrimSpace(sessionID)
			}
		}
		if strings.TrimSpace(ctx.workspaceID) == "" && strings.TrimSpace(ctx.worktreeID) == "" {
			m.setValidationStatus("workflow context unavailable for follow-up")
			return true, nil
		}
		m.setStatusMessage("create follow-up workflow from " + runID)
		return true, m.startGuidedWorkflowWithContext(ctx)
	case ContextMenuWorkflowStop:
		runID := strings.TrimSpace(target.workflowID)
		if runID == "" {
			m.setValidationStatus("select a workflow")
			return true, nil
		}
		m.setStatusMessage("stopping workflow " + runID)
		return true, stopWorkflowRunCmd(m.guidedWorkflowAPI, runID)
	case ContextMenuWorkflowDismiss:
		runID := strings.TrimSpace(target.workflowID)
		if runID == "" {
			m.setValidationStatus("select a workflow")
			return true, nil
		}
		m.setStatusMessage("dismissing workflow " + runID)
		return true, dismissWorkflowRunCmd(m.guidedWorkflowAPI, runID)
	case ContextMenuWorkflowUndismiss:
		runID := strings.TrimSpace(target.workflowID)
		if runID == "" {
			m.setValidationStatus("select a workflow")
			return true, nil
		}
		m.setStatusMessage("restoring workflow " + runID)
		return true, undismissWorkflowRunCmd(m.guidedWorkflowAPI, runID)
	case ContextMenuWorkflowCopyID:
		runID := strings.TrimSpace(target.workflowID)
		if runID == "" {
			m.setCopyStatusWarning("select a workflow")
			return true, nil
		}
		if cmd, ok := m.copySidebarSelectionIDsForContextTarget(target); ok {
			return true, cmd
		}
		return true, m.copyWithStatusCmd(runID, "copied workflow id")
	default:
		return false, nil
	}
}

func (m *Model) workflowRunByID(runID string) *guidedworkflows.WorkflowRun {
	if m == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	for _, run := range m.workflowRuns {
		if run == nil || strings.TrimSpace(run.ID) != runID {
			continue
		}
		return run
	}
	return nil
}
