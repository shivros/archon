package types

import "time"

const (
	MetadataEventSchemaVersionV1 = "v1"

	MetadataEventTypeSessionUpdated     = "session.updated"
	MetadataEventTypeWorkflowRunUpdated = "workflow_run.updated"
)

type MetadataEntityUpdated struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	UpdatedAt time.Time      `json:"updated_at"`
	Revision  string         `json:"revision,omitempty"`
	Changed   map[string]any `json:"changed,omitempty"`
}

type MetadataEvent struct {
	Version    string                 `json:"version"`
	Type       string                 `json:"type"`
	Revision   string                 `json:"revision"`
	OccurredAt time.Time              `json:"occurred_at"`
	Session    *MetadataEntityUpdated `json:"session,omitempty"`
	Workflow   *MetadataEntityUpdated `json:"workflow_run,omitempty"`
}
