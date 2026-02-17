package daemon

import (
	"context"
	"testing"

	"control/internal/guidedworkflows"
)

func TestWithGuidedWorkflowOrchestratorOption(t *testing.T) {
	orchestrator := guidedworkflows.New(guidedworkflows.Config{Enabled: true})
	svc := &SessionService{}
	WithGuidedWorkflowOrchestrator(orchestrator)(svc)
	if svc.guided != orchestrator {
		t.Fatalf("expected guided workflow option to set orchestrator")
	}
}

func TestSessionServiceStartGuidedWorkflowRunDisabledByDefault(t *testing.T) {
	svc := &SessionService{}
	_, err := svc.StartGuidedWorkflowRun(context.Background(), guidedworkflows.StartRunRequest{
		WorktreeID: "wt-1",
	})
	if err != guidedworkflows.ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestSessionServiceStartGuidedWorkflowRunDelegatesWhenConfigured(t *testing.T) {
	orchestrator := guidedworkflows.New(guidedworkflows.Config{Enabled: true})
	svc := &SessionService{guided: orchestrator}
	run, err := svc.StartGuidedWorkflowRun(context.Background(), guidedworkflows.StartRunRequest{
		WorktreeID: "wt-1",
		TaskID:     "task-1",
	})
	if err != nil {
		t.Fatalf("StartGuidedWorkflowRun: %v", err)
	}
	if run == nil || run.ID == "" {
		t.Fatalf("expected run result")
	}
}
