package guidedworkflows

import (
	"context"
	"sync"
	"testing"
)

type captureMetadataPublisher struct {
	mu     sync.Mutex
	events []WorkflowMetadataEvent
}

func (c *captureMetadataPublisher) PublishMetadataEvent(event WorkflowMetadataEvent) {
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}

func (c *captureMetadataPublisher) Events() []WorkflowMetadataEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]WorkflowMetadataEvent, len(c.events))
	copy(out, c.events)
	return out
}

func TestRenameRunPublishesMetadataEvent(t *testing.T) {
	publisher := &captureMetadataPublisher{}
	service := NewRunService(
		Config{Enabled: true},
		WithMetadataEventPublisher(publisher),
	)
	t.Cleanup(service.Close)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		UserPrompt:  "ship it",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.RenameRun(context.Background(), run.ID, "Renamed Workflow"); err != nil {
		t.Fatalf("RenameRun: %v", err)
	}

	events := publisher.Events()
	if len(events) != 1 {
		t.Fatalf("expected one metadata event, got %d", len(events))
	}
	got := events[0]
	if got.Type != WorkflowMetadataEventTypeRunUpdated {
		t.Fatalf("unexpected event type: %q", got.Type)
	}
	if got.RunID != run.ID || got.Title != "Renamed Workflow" {
		t.Fatalf("unexpected workflow payload: %#v", got)
	}
}

func TestRenameRunSameTitleDoesNotPublishMetadataEvent(t *testing.T) {
	publisher := &captureMetadataPublisher{}
	service := NewRunService(
		Config{Enabled: true},
		WithMetadataEventPublisher(publisher),
	)
	t.Cleanup(service.Close)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		UserPrompt:  "ship it",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.RenameRun(context.Background(), run.ID, run.TemplateName); err != nil {
		t.Fatalf("RenameRun: %v", err)
	}
	if got := len(publisher.Events()); got != 0 {
		t.Fatalf("expected no metadata event for no-op rename, got %d", got)
	}
}

func TestSetMetadataEventPublisherAppliesAtRuntime(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	t.Cleanup(service.Close)
	publisher := &captureMetadataPublisher{}
	service.SetMetadataEventPublisher(publisher)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		UserPrompt:  "ship it",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.RenameRun(context.Background(), run.ID, "Renamed Workflow"); err != nil {
		t.Fatalf("RenameRun: %v", err)
	}
	if got := len(publisher.Events()); got != 1 {
		t.Fatalf("expected one metadata event after runtime publisher set, got %d", got)
	}
}
