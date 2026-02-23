package daemon

import (
	"context"
	"errors"
	"testing"

	"control/internal/guidedworkflows"
)

func TestWorkflowRunStopCoordinatorStopsRunAndInterruptsSessions(t *testing.T) {
	ctx := context.Background()
	runService := guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	created, err := runService.CreateRun(ctx, guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runService.StartRun(ctx, created.ID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	interrupt := &recordCoordinatorInterruptService{}
	coordinator := newWorkflowRunStopCoordinator(runService, interrupt, nil)

	stopped, err := coordinator.StopWorkflowRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("stop workflow run: %v", err)
	}
	if stopped == nil || stopped.Status != guidedworkflows.WorkflowRunStatusStopped {
		t.Fatalf("expected stopped run payload, got %#v", stopped)
	}
	if interrupt.calls != 1 {
		t.Fatalf("expected one interrupt invocation, got %d", interrupt.calls)
	}
	if len(interrupt.runIDs) != 1 || interrupt.runIDs[0] != created.ID {
		t.Fatalf("expected interrupt invocation for run %q, got %#v", created.ID, interrupt.runIDs)
	}
}

func TestWorkflowRunStopCoordinatorIgnoresInterruptErrors(t *testing.T) {
	ctx := context.Background()
	runService := guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	created, err := runService.CreateRun(ctx, guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := runService.StartRun(ctx, created.ID); err != nil {
		t.Fatalf("start run: %v", err)
	}
	coordinator := newWorkflowRunStopCoordinator(runService, &recordCoordinatorInterruptService{
		err: errors.New("interrupt failed"),
	}, nil)

	stopped, err := coordinator.StopWorkflowRun(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected interrupt errors to be best-effort, got %v", err)
	}
	if stopped == nil || stopped.Status != guidedworkflows.WorkflowRunStatusStopped {
		t.Fatalf("expected stopped run payload, got %#v", stopped)
	}
}

func TestWorkflowRunStopCoordinatorDoesNotInterruptOnStopFailure(t *testing.T) {
	ctx := context.Background()
	runService := guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	interrupt := &recordCoordinatorInterruptService{}
	coordinator := newWorkflowRunStopCoordinator(runService, interrupt, nil)

	stopped, err := coordinator.StopWorkflowRun(ctx, "gwf-missing")
	if !errors.Is(err, guidedworkflows.ErrRunNotFound) {
		t.Fatalf("expected run not found error, got %v", err)
	}
	if stopped != nil {
		t.Fatalf("expected nil run payload on stop failure, got %#v", stopped)
	}
	if interrupt.calls != 0 {
		t.Fatalf("expected no interrupt call when stop fails, got %d", interrupt.calls)
	}
}

func TestNewWorkflowRunStopCoordinatorNilRunServiceReturnsNil(t *testing.T) {
	coordinator := newWorkflowRunStopCoordinator(nil, &recordCoordinatorInterruptService{}, nil)
	if coordinator != nil {
		t.Fatalf("expected nil coordinator when run service is nil")
	}
}

type recordCoordinatorInterruptService struct {
	calls  int
	runIDs []string
	err    error
}

func (r *recordCoordinatorInterruptService) InterruptWorkflowRunSessions(_ context.Context, run *guidedworkflows.WorkflowRun) error {
	if r == nil {
		return nil
	}
	r.calls++
	if run != nil {
		r.runIDs = append(r.runIDs, run.ID)
	}
	return r.err
}
