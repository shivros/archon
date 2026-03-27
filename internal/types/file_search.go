package types

import "time"

type FileSearchScope struct {
	Provider    string `json:"provider,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	WorktreeID  string `json:"worktree_id,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
}

type FileSearchStartRequest struct {
	Scope FileSearchScope `json:"scope"`
	Query string          `json:"query,omitempty"`
	Limit int             `json:"limit,omitempty"`
}

type FileSearchUpdateRequest struct {
	Scope *FileSearchScope `json:"scope,omitempty"`
	Query *string          `json:"query,omitempty"`
	Limit *int             `json:"limit,omitempty"`
}

type FileSearchStatus string

const (
	FileSearchStatusCreated FileSearchStatus = "created"
	FileSearchStatusActive  FileSearchStatus = "active"
	FileSearchStatusClosed  FileSearchStatus = "closed"
	FileSearchStatusFailed  FileSearchStatus = "failed"
)

type FileSearchSession struct {
	ID        string           `json:"id"`
	Provider  string           `json:"provider"`
	Scope     FileSearchScope  `json:"scope"`
	Query     string           `json:"query,omitempty"`
	Limit     int              `json:"limit,omitempty"`
	Status    FileSearchStatus `json:"status"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt *time.Time       `json:"updated_at,omitempty"`
	ClosedAt  *time.Time       `json:"closed_at,omitempty"`
}

type FileSearchCandidate struct {
	Path        string  `json:"path"`
	DisplayPath string  `json:"display_path,omitempty"`
	Directory   string  `json:"directory,omitempty"`
	Kind        string  `json:"kind,omitempty"`
	Score       float64 `json:"score,omitempty"`
}

type FileSearchEventKind string

const (
	FileSearchEventStarted FileSearchEventKind = "file_search.started"
	FileSearchEventUpdated FileSearchEventKind = "file_search.updated"
	FileSearchEventResults FileSearchEventKind = "file_search.results"
	FileSearchEventClosed  FileSearchEventKind = "file_search.closed"
	FileSearchEventFailed  FileSearchEventKind = "file_search.failed"
)

type FileSearchEvent struct {
	Kind       FileSearchEventKind   `json:"kind"`
	SearchID   string                `json:"search_id"`
	Provider   string                `json:"provider,omitempty"`
	Scope      FileSearchScope       `json:"scope"`
	Query      string                `json:"query,omitempty"`
	Status     FileSearchStatus      `json:"status,omitempty"`
	Limit      int                   `json:"limit,omitempty"`
	Candidates []FileSearchCandidate `json:"candidates,omitempty"`
	Error      string                `json:"error,omitempty"`
	OccurredAt *time.Time            `json:"occurred_at,omitempty"`
}
