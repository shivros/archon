package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"control/internal/guidedworkflows"
)

func TestWorkflowRunStoreRoundTrip(t *testing.T) {
	store := NewFileWorkflowRunStore(filepath.Join(t.TempDir(), "workflow_runs.json"))
	snapshot := guidedworkflows.RunStatusSnapshot{
		Run: &guidedworkflows.WorkflowRun{
			ID:         "gwf-test-1",
			TemplateID: guidedworkflows.TemplateIDSolidPhaseDelivery,
			Status:     guidedworkflows.WorkflowRunStatusRunning,
			CreatedAt:  time.Now().UTC(),
			Phases: []guidedworkflows.PhaseRun{
				{
					ID:     "phase-1",
					Name:   "Phase 1",
					Status: guidedworkflows.PhaseRunStatusRunning,
					Steps: []guidedworkflows.StepRun{
						{
							ID:     "step-1",
							Name:   "Step 1",
							Status: guidedworkflows.StepRunStatusRunning,
						},
					},
				},
			},
		},
		Timeline: []guidedworkflows.RunTimelineEvent{
			{
				At:      time.Now().UTC(),
				Type:    "run_created",
				RunID:   "gwf-test-1",
				Message: "created",
			},
		},
	}
	if err := store.UpsertWorkflowRun(context.Background(), snapshot); err != nil {
		t.Fatalf("UpsertWorkflowRun: %v", err)
	}
	runs, err := store.ListWorkflowRuns(context.Background())
	if err != nil {
		t.Fatalf("ListWorkflowRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run snapshot, got %d", len(runs))
	}
	if runs[0].Run == nil || runs[0].Run.ID != "gwf-test-1" {
		t.Fatalf("unexpected run snapshot: %#v", runs[0])
	}
	if len(runs[0].Timeline) != 1 || runs[0].Timeline[0].Type != "run_created" {
		t.Fatalf("unexpected run timeline: %#v", runs[0].Timeline)
	}
}

func TestWorkflowRunStoreUpsertRequiresRunID(t *testing.T) {
	store := NewFileWorkflowRunStore(filepath.Join(t.TempDir(), "workflow_runs.json"))
	err := store.UpsertWorkflowRun(context.Background(), guidedworkflows.RunStatusSnapshot{
		Run: &guidedworkflows.WorkflowRun{},
	})
	if err == nil {
		t.Fatalf("expected error for missing run id")
	}
}
