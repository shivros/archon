package app

import (
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

type contextMenuTarget struct {
	id          string
	workspaceID string
	worktreeID  string
	sessionID   string
}

func (m *Model) handleWorkspaceContextMenuAction(action ContextMenuAction, target contextMenuTarget) (bool, tea.Cmd) {
	switch action {
	case ContextMenuWorkspaceCreate:
		m.enterAddWorkspace()
		return true, nil
	case ContextMenuWorkspaceRename:
		if target.id == "" {
			m.status = "select a workspace to rename"
			return true, nil
		}
		m.enterRenameWorkspace(target.id)
		return true, nil
	case ContextMenuWorkspaceEditGroups:
		if target.id == "" {
			m.status = "select a workspace"
			return true, nil
		}
		m.enterEditWorkspaceGroups(target.id)
		return true, nil
	case ContextMenuWorkspaceDelete:
		if target.id == "" || target.id == unassignedWorkspaceID {
			m.status = "select a workspace to delete"
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
			m.status = "select a workspace"
			return true, nil
		}
		m.enterAddWorktree(target.workspaceID)
		return true, nil
	case ContextMenuWorktreeDelete:
		if target.worktreeID == "" || target.workspaceID == "" {
			m.status = "select a worktree"
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
			m.status = "select a session"
			return true, nil
		}
		m.enterCompose(target.sessionID)
		return true, nil
	case ContextMenuSessionDismiss:
		if target.sessionID == "" {
			m.status = "select a session"
			return true, nil
		}
		m.confirmDismissSessions([]string{target.sessionID})
		return true, nil
	case ContextMenuSessionKill:
		if target.sessionID == "" {
			m.status = "select a session"
			return true, nil
		}
		m.status = "killing " + target.sessionID
		return true, killSessionCmd(m.sessionAPI, target.sessionID)
	case ContextMenuSessionInterrupt:
		if target.sessionID == "" {
			m.status = "select a session"
			return true, nil
		}
		m.status = "interrupting " + target.sessionID
		return true, interruptSessionCmd(m.sessionAPI, target.sessionID)
	case ContextMenuSessionCopyID:
		if target.sessionID == "" {
			m.status = "select a session"
			return true, nil
		}
		if err := clipboard.WriteAll(target.sessionID); err != nil {
			m.status = "copy failed: " + err.Error()
			return true, nil
		}
		m.status = "copied session id"
		return true, nil
	default:
		return false, nil
	}
}
