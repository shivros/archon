package types

import "time"

type SessionMeta struct {
	SessionID    string     `json:"session_id"`
	WorkspaceID  string     `json:"workspace_id"`
	WorktreeID   string     `json:"worktree_id"`
	Title        string     `json:"title"`
	InitialInput string     `json:"initial_input"`
	LastActiveAt *time.Time `json:"last_active_at,omitempty"`
}
