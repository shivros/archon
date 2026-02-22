package types

import "time"

type Workspace struct {
	ID                    string    `json:"id"`
	Name                  string    `json:"name"`
	RepoPath              string    `json:"repo_path"`
	SessionSubpath        string    `json:"session_subpath,omitempty"`
	AdditionalDirectories []string  `json:"additional_directories,omitempty"`
	GroupIDs              []string  `json:"group_ids,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}
