package app

import (
	"testing"

	"control/internal/guidedworkflows"
)

func TestShouldApplyWorkflowSnapshotToGuidedControllerRequiresMatchingRunID(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeGuidedWorkflow
	m.guidedWorkflow = NewGuidedWorkflowUIController()
	m.guidedWorkflow.SetRun(&guidedworkflows.WorkflowRun{
		ID:     "gwf-1",
		Status: guidedworkflows.WorkflowRunStatusRunning,
	})

	if !shouldApplyWorkflowSnapshotToGuidedController(&m, &guidedworkflows.WorkflowRun{ID: "gwf-1"}) {
		t.Fatalf("expected snapshot for active run to apply")
	}
	if shouldApplyWorkflowSnapshotToGuidedController(&m, &guidedworkflows.WorkflowRun{ID: "gwf-2"}) {
		t.Fatalf("did not expect snapshot for different run to apply")
	}
}

func TestShouldApplyWorkflowSnapshotErrorToGuidedControllerRequiresActiveRun(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeGuidedWorkflow
	m.guidedWorkflow = NewGuidedWorkflowUIController()

	if shouldApplyWorkflowSnapshotErrorToGuidedController(&m) {
		t.Fatalf("did not expect snapshot error to apply without active run")
	}

	m.guidedWorkflow.SetRun(&guidedworkflows.WorkflowRun{
		ID:     "gwf-1",
		Status: guidedworkflows.WorkflowRunStatusRunning,
	})
	if !shouldApplyWorkflowSnapshotErrorToGuidedController(&m) {
		t.Fatalf("expected snapshot error to apply for active run")
	}
}
