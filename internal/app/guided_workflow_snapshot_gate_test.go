package app

import (
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
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

func TestShouldApplyWorkflowSnapshotToWorkflowPreviewRequiresSelectedMatchingWorkflow(t *testing.T) {
	now := time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC)
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.guidedWorkflow = NewGuidedWorkflowUIController()
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	m.sessions = []*types.Session{{ID: "s1", CreatedAt: now, Status: types.SessionStatusRunning}}
	m.workflowRuns = []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", Status: guidedworkflows.WorkflowRunStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: "gwf-1"},
	}
	m.applySidebarItems()
	if m.sidebar == nil || !m.sidebar.SelectByWorkflowID("gwf-1") {
		t.Fatalf("expected workflow row to be selectable")
	}
	_ = m.onSelectionChangedImmediate()

	if !shouldApplyWorkflowSnapshotToWorkflowPreview(&m, &guidedworkflows.WorkflowRun{ID: "gwf-1"}) {
		t.Fatalf("expected selected preview workflow snapshot to apply")
	}
	if shouldApplyWorkflowSnapshotToWorkflowPreview(&m, &guidedworkflows.WorkflowRun{ID: "gwf-2"}) {
		t.Fatalf("did not expect non-selected workflow snapshot to apply")
	}

	m.mode = uiModeGuidedWorkflow
	if shouldApplyWorkflowSnapshotToWorkflowPreview(&m, &guidedworkflows.WorkflowRun{ID: "gwf-1"}) {
		t.Fatalf("did not expect passive preview gate to apply while interactive mode is active")
	}
}
