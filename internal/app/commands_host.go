package app

import (
	"control/internal/client"
	"control/internal/types"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) setStatus(status string) {
	m.status = status
}

func (m *Model) createWorkspaceCmd(path, name, provider string) tea.Cmd {
	return createWorkspaceCmd(m.workspaceAPI, path, name, provider)
}

func (m *Model) fetchAvailableWorktreesCmd(workspaceID, workspacePath string) tea.Cmd {
	return fetchAvailableWorktreesCmd(m.workspaceAPI, workspaceID, workspacePath)
}

func (m *Model) createWorktreeCmd(workspaceID string, req client.CreateWorktreeRequest) tea.Cmd {
	return createWorktreeCmd(m.workspaceAPI, workspaceID, req)
}

func (m *Model) addWorktreeCmd(workspaceID string, worktree *types.Worktree) tea.Cmd {
	return addWorktreeCmd(m.workspaceAPI, workspaceID, worktree)
}

func (m *Model) sendMessageCmd(sessionID, text string) tea.Cmd {
	token := m.nextSendToken()
	if m.chat != nil {
		m.registerPendingSend(token, sessionID)
		return sendSessionCmd(m.sessionAPI, sessionID, text, token)
	}
	m.registerPendingSend(token, sessionID)
	return sendSessionCmd(m.sessionAPI, sessionID, text, token)
}

func (m *Model) startWorkspaceSessionCmd(workspaceID, worktreeID, provider, text string) tea.Cmd {
	return startSessionCmd(m.sessionAPI, workspaceID, worktreeID, provider, text)
}
