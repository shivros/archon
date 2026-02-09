package types

import "time"

type NoteKind string

const (
	NoteKindNote NoteKind = "note"
	NoteKindPin  NoteKind = "pin"
)

type NoteScope string

const (
	NoteScopeWorkspace NoteScope = "workspace"
	NoteScopeWorktree  NoteScope = "worktree"
	NoteScopeSession   NoteScope = "session"
)

type NoteStatus string

const (
	NoteStatusIdea     NoteStatus = "idea"
	NoteStatusTodo     NoteStatus = "todo"
	NoteStatusDecision NoteStatus = "decision"
)

type NoteSource struct {
	SessionID string `json:"session_id,omitempty"`
	BlockID   string `json:"block_id,omitempty"`
	Role      string `json:"role,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
}

type Note struct {
	ID          string      `json:"id"`
	Kind        NoteKind    `json:"kind"`
	Scope       NoteScope   `json:"scope"`
	WorkspaceID string      `json:"workspace_id,omitempty"`
	WorktreeID  string      `json:"worktree_id,omitempty"`
	SessionID   string      `json:"session_id,omitempty"`
	Title       string      `json:"title,omitempty"`
	Body        string      `json:"body,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Status      NoteStatus  `json:"status,omitempty"`
	Source      *NoteSource `json:"source,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}
