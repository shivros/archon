package client

import "control/internal/types"

type SessionsResponse struct {
	Sessions []*types.Session `json:"sessions"`
}

type SessionsWithMetaResponse struct {
	Sessions    []*types.Session     `json:"sessions"`
	SessionMeta []*types.SessionMeta `json:"session_meta"`
}

type WorkspacesResponse struct {
	Workspaces []*types.Workspace `json:"workspaces"`
}

type WorktreesResponse struct {
	Worktrees []*types.Worktree `json:"worktrees"`
}

type AvailableWorktreesResponse struct {
	Worktrees []*types.GitWorktree `json:"worktrees"`
}

type CreateWorktreeRequest struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Name   string `json:"name,omitempty"`
}

type StartSessionRequest struct {
	Provider    string   `json:"provider"`
	Cmd         string   `json:"cmd,omitempty"`
	Cwd         string   `json:"cwd,omitempty"`
	Args        []string `json:"args,omitempty"`
	Env         []string `json:"env,omitempty"`
	Title       string   `json:"title,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	WorktreeID  string   `json:"worktree_id,omitempty"`
}

type TailItemsResponse struct {
	Items []map[string]any `json:"items"`
}

type SendSessionRequest struct {
	Text  string           `json:"text,omitempty"`
	Input []map[string]any `json:"input,omitempty"`
}

type SendSessionResponse struct {
	OK     bool   `json:"ok"`
	TurnID string `json:"turn_id,omitempty"`
}

type HealthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
}
