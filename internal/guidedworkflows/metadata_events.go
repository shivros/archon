package guidedworkflows

import "time"

type WorkflowMetadataEventType string

const (
	WorkflowMetadataEventTypeRunUpdated WorkflowMetadataEventType = "workflow_run.updated"
)

type WorkflowMetadataEvent struct {
	Type      WorkflowMetadataEventType
	RunID     string
	Title     string
	UpdatedAt time.Time
	Changed   map[string]any
}

type MetadataEventPublisher interface {
	PublishMetadataEvent(event WorkflowMetadataEvent)
}
