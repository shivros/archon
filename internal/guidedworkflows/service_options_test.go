package guidedworkflows

import (
	"context"
	"testing"
)

type optionQueueStub struct{}

func (optionQueueStub) Enqueue(context.Context, DispatchRequest) (DispatchQueueResult, bool) {
	return DispatchQueueResult{Done: true}, true
}
func (optionQueueStub) Close() {}

func TestRunServiceOptionsDispatchQueueAndStateStore(t *testing.T) {
	service := &InMemoryRunService{}

	queue := optionQueueStub{}
	WithDispatchQueue(queue)(service)
	if service.dispatchQueue == nil {
		t.Fatalf("expected dispatch queue option to set queue")
	}

	state := NewMemoryRunStateStore()
	WithRunStateStore(state)(service)
	if service.state == nil {
		t.Fatalf("expected run state store option to set state")
	}
}

func TestRunServiceWithCustomStateStoreSupportsWritePaths(t *testing.T) {
	state := NewMemoryRunStateStore()
	service := NewRunService(
		Config{Enabled: true},
		WithRunStateStore(state),
	)
	t.Cleanup(service.Close)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		UserPrompt:  "ship it",
	})
	if err != nil {
		t.Fatalf("CreateRun returned error: %v", err)
	}

	if _, err := service.RenameRun(context.Background(), run.ID, "Renamed Workflow"); err != nil {
		t.Fatalf("RenameRun returned error: %v", err)
	}
	if _, err := service.DismissRun(context.Background(), run.ID); err != nil {
		t.Fatalf("DismissRun returned error: %v", err)
	}

	loaded, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if loaded.TemplateName != "Renamed Workflow" {
		t.Fatalf("expected renamed template name, got %q", loaded.TemplateName)
	}
	if loaded.DismissedAt == nil {
		t.Fatalf("expected run to be dismissed")
	}

	timeline, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline returned error: %v", err)
	}
	if len(timeline) < 3 {
		t.Fatalf("expected timeline entries for create/rename/dismiss, got %d", len(timeline))
	}
}
