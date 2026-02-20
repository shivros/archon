package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

type contextMenuTarget struct {
	id                string
	workspaceID       string
	worktreeID        string
	sessionID         string
	workflowID        string
	workflowDismissed bool
}

func (m *Model) handleWorkspaceContextMenuAction(action ContextMenuAction, target contextMenuTarget) (bool, tea.Cmd) {
	switch action {
	case ContextMenuWorkspaceCreate:
		m.enterAddWorkspace()
		return true, nil
	case ContextMenuWorkspaceRename:
		if target.id == "" {
			m.setValidationStatus("select a workspace to rename")
			return true, nil
		}
		m.enterRenameWorkspace(target.id)
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
		if target.id == "" || target.id == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace")
			return true, nil
		}
		m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
			workspaceID: target.id,
		})
		return true, fetchWorkflowTemplatesCmd(m.guidedWorkflowTemplateAPI)
	case ContextMenuWorkspaceCopyPath:
		if target.id == "" || target.id == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace")
			return true, nil
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
		if strings.TrimSpace(target.workspaceID) == "" && strings.TrimSpace(target.worktreeID) == "" {
			m.setValidationStatus("select a worktree")
			return true, nil
		}
		m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
			workspaceID: target.workspaceID,
			worktreeID:  target.worktreeID,
		})
		return true, fetchWorkflowTemplatesCmd(m.guidedWorkflowTemplateAPI)
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
		if target.sessionID == "" {
			m.setValidationStatus("select a session")
			return true, nil
		}
		if strings.TrimSpace(target.workspaceID) == "" && strings.TrimSpace(target.worktreeID) == "" {
			m.setValidationStatus("session has no workspace/worktree context")
			return true, nil
		}
		m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
			workspaceID: target.workspaceID,
			worktreeID:  target.worktreeID,
			sessionID:   target.sessionID,
		})
		return true, fetchWorkflowTemplatesCmd(m.guidedWorkflowTemplateAPI)
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
		return true, m.copyWithStatusCmd(runID, "copied workflow id")
	default:
		return false, nil
	}
}
