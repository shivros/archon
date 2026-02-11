package types

import "time"

type SessionMeta struct {
	SessionID         string                 `json:"session_id"`
	WorkspaceID       string                 `json:"workspace_id"`
	WorktreeID        string                 `json:"worktree_id"`
	Title             string                 `json:"title"`
	TitleLocked       bool                   `json:"title_locked,omitempty"`
	InitialInput      string                 `json:"initial_input"`
	DismissedAt       *time.Time             `json:"dismissed_at,omitempty"`
	ThreadID          string                 `json:"thread_id,omitempty"`
	ProviderSessionID string                 `json:"provider_session_id,omitempty"`
	LastTurnID        string                 `json:"last_turn_id,omitempty"`
	RuntimeOptions    *SessionRuntimeOptions `json:"runtime_options,omitempty"`
	LastActiveAt      *time.Time             `json:"last_active_at,omitempty"`
}
