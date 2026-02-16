package types

type AppState struct {
	ActiveWorkspaceID         string                            `json:"active_workspace_id"`
	ActiveWorktreeID          string                            `json:"active_worktree_id"`
	ActiveWorkspaceGroupIDs   []string                          `json:"active_workspace_group_ids"`
	SidebarCollapsed          bool                              `json:"sidebar_collapsed"`
	SidebarWorkspaceExpanded  map[string]bool                   `json:"sidebar_workspace_expanded,omitempty"`
	SidebarWorktreeExpanded   map[string]bool                   `json:"sidebar_worktree_expanded,omitempty"`
	ComposeHistory            map[string][]string               `json:"compose_history,omitempty"`
	ComposeDrafts             map[string]string                 `json:"compose_drafts,omitempty"`
	NoteDrafts                map[string]string                 `json:"note_drafts,omitempty"`
	ComposeDefaultsByProvider map[string]*SessionRuntimeOptions `json:"compose_defaults_by_provider,omitempty"`
	ProviderBadges            map[string]*ProviderBadgeConfig   `json:"provider_badges,omitempty"`
}

type ProviderBadgeConfig struct {
	Prefix string `json:"prefix,omitempty"`
	Color  string `json:"color,omitempty"`
}
