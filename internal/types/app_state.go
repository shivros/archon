package types

type AppState struct {
	ActiveWorkspaceID         string                            `json:"active_workspace_id"`
	ActiveWorktreeID          string                            `json:"active_worktree_id"`
	ActiveWorkspaceGroupIDs   []string                          `json:"active_workspace_group_ids"`
	SidebarCollapsed          bool                              `json:"sidebar_collapsed"`
	ComposeHistory            map[string][]string               `json:"compose_history,omitempty"`
	ComposeDefaultsByProvider map[string]*SessionRuntimeOptions `json:"compose_defaults_by_provider,omitempty"`
}
