package types

import "time"

type SessionMeta struct {
	SessionID    string     `json:"session_id"`
	WorkspaceID  string     `json:"workspace_id"`
	WorktreeID   string     `json:"worktree_id"`
	Title        string     `json:"title"`
	InitialInput string     `json:"initial_input"`
	ThreadID     string     `json:"thread_id,omitempty"`
	LastTurnID   string     `json:"last_turn_id,omitempty"`
	LastActiveAt *time.Time `json:"last_active_at,omitempty"`
}
