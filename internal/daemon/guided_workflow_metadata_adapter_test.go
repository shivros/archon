package daemon

import (
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type captureDaemonMetadataPublisher struct {
	events []types.MetadataEvent
}

func (c *captureDaemonMetadataPublisher) PublishMetadataEvent(event types.MetadataEvent) {
	c.events = append(c.events, event)
}

func TestGuidedWorkflowMetadataEventAdapterMapsRunUpdated(t *testing.T) {
	target := &captureDaemonMetadataPublisher{}
	adapter := newGuidedWorkflowMetadataEventAdapter(target)
	if adapter == nil {
		t.Fatalf("expected adapter")
	}
	adapter.PublishMetadataEvent(guidedworkflows.WorkflowMetadataEvent{
		Type:  guidedworkflows.WorkflowMetadataEventTypeRunUpdated,
		RunID: "gwf-1",
		Title: "Renamed Workflow",
		Changed: map[string]any{
			"title": "Renamed Workflow",
		},
	})
	if len(target.events) != 1 {
		t.Fatalf("expected one forwarded event, got %d", len(target.events))
	}
	event := target.events[0]
	if event.Type != types.MetadataEventTypeWorkflowRunUpdated {
		t.Fatalf("unexpected event type: %q", event.Type)
	}
	if event.Workflow == nil || event.Workflow.ID != "gwf-1" || event.Workflow.Title != "Renamed Workflow" {
		t.Fatalf("unexpected workflow payload: %#v", event.Workflow)
	}
}

func TestGuidedWorkflowMetadataEventAdapterIgnoresUnknownType(t *testing.T) {
	target := &captureDaemonMetadataPublisher{}
	adapter := newGuidedWorkflowMetadataEventAdapter(target)
	adapter.PublishMetadataEvent(guidedworkflows.WorkflowMetadataEvent{
		Type:  guidedworkflows.WorkflowMetadataEventType("workflow_run.deleted"),
		RunID: "gwf-1",
		Title: "ignored",
	})
	if len(target.events) != 0 {
		t.Fatalf("expected unknown event type to be ignored, got %d", len(target.events))
	}
}

func TestGuidedWorkflowMetadataEventAdapterDefaultsUpdatedAt(t *testing.T) {
	target := &captureDaemonMetadataPublisher{}
	adapter := newGuidedWorkflowMetadataEventAdapter(target)
	adapter.PublishMetadataEvent(guidedworkflows.WorkflowMetadataEvent{
		Type:  guidedworkflows.WorkflowMetadataEventTypeRunUpdated,
		RunID: "gwf-1",
		Title: "Renamed Workflow",
	})
	if len(target.events) != 1 {
		t.Fatalf("expected one forwarded event, got %d", len(target.events))
	}
	if target.events[0].Workflow == nil || target.events[0].Workflow.UpdatedAt.IsZero() {
		t.Fatalf("expected adapter to populate updated_at")
	}
	if time.Since(target.events[0].Workflow.UpdatedAt) > 5*time.Second {
		t.Fatalf("expected recent updated_at timestamp, got %s", target.events[0].Workflow.UpdatedAt)
	}
}

func TestGuidedWorkflowMetadataEventAdapterNilPublisherReturnsNil(t *testing.T) {
	if got := newGuidedWorkflowMetadataEventAdapter(nil); got != nil {
		t.Fatalf("expected nil adapter when publisher is nil")
	}
}
