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
