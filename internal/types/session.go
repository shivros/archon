package types

import "time"

type SessionStatus string

const (
	SessionStatusCreated  SessionStatus = "created"
	SessionStatusStarting SessionStatus = "starting"
	SessionStatusRunning  SessionStatus = "running"
	SessionStatusInactive SessionStatus = "inactive"
	SessionStatusExited   SessionStatus = "exited"
	SessionStatusFailed   SessionStatus = "failed"
	SessionStatusKilled   SessionStatus = "killed"
	SessionStatusOrphaned SessionStatus = "orphaned"
)

type Session struct {
	ID        string        `json:"id"`
	Provider  string        `json:"provider"`
	Cwd       string        `json:"cwd,omitempty"`
	Cmd       string        `json:"cmd"`
	Args      []string      `json:"args,omitempty"`
	Env       []string      `json:"env,omitempty"`
	Status    SessionStatus `json:"status"`
	PID       int           `json:"pid,omitempty"`
	ExitCode  *int          `json:"exit_code,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	StartedAt *time.Time    `json:"started_at,omitempty"`
	ExitedAt  *time.Time    `json:"exited_at,omitempty"`
	Title     string        `json:"title,omitempty"`
	Tags      []string      `json:"tags,omitempty"`
}
