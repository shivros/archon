package guidedworkflows

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateRunWithMissingDependencyFails(t *testing.T) {
	t.Parallel()

	service := NewRunService(Config{Enabled: true})
	_, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID:     "ws-1",
		WorktreeID:      "wt-1",
		DependsOnRunIDs: []string{"gwf-missing"},
	})
	if !errors.Is(err, ErrDependencyNotFound) {
		t.Fatalf("expected dependency not found error, got %v", err)
	}
}

func TestStartRunQueuesWhenDependencyUnmet(t *testing.T) {
	t.Parallel()

	service := NewRunService(Config{Enabled: true})
	upstream, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("create upstream: %v", err)
	}
	downstream, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID:     "ws-1",
		WorktreeID:      "wt-2",
		DependsOnRunIDs: []string{upstream.ID},
	})
	if err != nil {
		t.Fatalf("create downstream: %v", err)
	}
	started, err := service.StartRun(context.Background(), downstream.ID)
	if err != nil {
		t.Fatalf("start downstream: %v", err)
	}
	if started.Status != WorkflowRunStatusQueued {
		t.Fatalf("expected queued status, got %q", started.Status)
	}
	if started.DependencyState.Ready {
		t.Fatalf("expected dependency state ready=false")
	}
	if len(started.DependencyState.Unmet) == 0 {
		t.Fatalf("expected unmet dependency details")
	}
}

func TestQueuedRunAutoStartsAfterDependencyCompletes(t *testing.T) {
	t.Parallel()

	service := NewRunService(Config{Enabled: true})
	upstream, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("create upstream: %v", err)
	}
	downstream, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID:     "ws-1",
		WorktreeID:      "wt-2",
		DependsOnRunIDs: []string{upstream.ID},
	})
	if err != nil {
		t.Fatalf("create downstream: %v", err)
	}
	queued, err := service.StartRun(context.Background(), downstream.ID)
	if err != nil {
		t.Fatalf("start downstream: %v", err)
	}
	if queued.Status != WorkflowRunStatusQueued {
		t.Fatalf("expected downstream queued, got %q", queued.Status)
	}

	if _, err := service.StartRun(context.Background(), upstream.ID); err != nil {
		t.Fatalf("start upstream: %v", err)
	}
	for i := 0; i < 64; i++ {
		current, getErr := service.GetRun(context.Background(), upstream.ID)
		if getErr != nil {
			t.Fatalf("get upstream: %v", getErr)
		}
		if current.Status == WorkflowRunStatusCompleted {
			break
		}
		if current.Status != WorkflowRunStatusRunning {
			t.Fatalf("unexpected upstream status while completing: %q", current.Status)
		}
		if _, advErr := service.AdvanceRun(context.Background(), upstream.ID); advErr != nil {
			t.Fatalf("advance upstream: %v", advErr)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		current, getErr := service.GetRun(context.Background(), downstream.ID)
		if getErr != nil {
			t.Fatalf("get downstream: %v", getErr)
		}
		if current.Status == WorkflowRunStatusRunning || current.Status == WorkflowRunStatusCompleted {
			if !current.DependencyState.Ready {
				t.Fatalf("expected downstream dependency state ready=true after activation")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for queued downstream to auto-start, last status=%q", current.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
