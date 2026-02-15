package app

import (
	"control/internal/client"
	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) setStatus(status string) {
	m.setStatusMessage(status)
}

func (m *Model) createWorkspaceCmd(path, name string) tea.Cmd {
	return createWorkspaceCmd(m.workspaceAPI, path, name)
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
	provider := m.providerForSessionID(sessionID)
	if m.chat != nil {
		m.registerPendingSend(token, sessionID, provider)
		return sendSessionCmd(m.sessionAPI, sessionID, text, token)
	}
	m.registerPendingSend(token, sessionID, provider)
	return sendSessionCmd(m.sessionAPI, sessionID, text, token)
}

func (m *Model) startWorkspaceSessionCmd(workspaceID, worktreeID, provider, text string, runtimeOptions *types.SessionRuntimeOptions) tea.Cmd {
	return startSessionCmd(m.sessionAPI, workspaceID, worktreeID, provider, text, runtimeOptions)
}
