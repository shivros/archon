package types

type AppState struct {
	ActiveWorkspaceID string `json:"active_workspace_id"`
	ActiveWorktreeID  string `json:"active_worktree_id"`
	SidebarCollapsed  bool   `json:"sidebar_collapsed"`
}
