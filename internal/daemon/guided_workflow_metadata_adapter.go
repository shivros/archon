package daemon

import (
	"strings"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type guidedWorkflowMetadataEventAdapter struct {
	publisher MetadataEventPublisher
}

func newGuidedWorkflowMetadataEventAdapter(publisher MetadataEventPublisher) guidedworkflows.MetadataEventPublisher {
	if publisher == nil {
		return nil
	}
	return &guidedWorkflowMetadataEventAdapter{publisher: publisher}
}

func (a *guidedWorkflowMetadataEventAdapter) PublishMetadataEvent(event guidedworkflows.WorkflowMetadataEvent) {
	if a == nil || a.publisher == nil {
		return
	}
	if strings.TrimSpace(string(event.Type)) != string(guidedworkflows.WorkflowMetadataEventTypeRunUpdated) {
		return
	}
	updatedAt := event.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	a.publisher.PublishMetadataEvent(types.MetadataEvent{
		Version: types.MetadataEventSchemaVersionV1,
		Type:    types.MetadataEventTypeWorkflowRunUpdated,
		Workflow: &types.MetadataEntityUpdated{
			ID:        strings.TrimSpace(event.RunID),
			Title:     strings.TrimSpace(event.Title),
			UpdatedAt: updatedAt,
			Changed:   event.Changed,
		},
	})
}
