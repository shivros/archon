package app

import (
	"testing"
	"time"

	"control/internal/guidedworkflows"
)

func TestReduceMutationMessagesWorkflowRunStartedQueuedSetsQueuedStatusMessage(t *testing.T) {
	m := NewModel(nil)
	m.guidedWorkflow = NewGuidedWorkflowUIController()
	m.guidedWorkflow.Enter(guidedWorkflowLaunchContext{
		workspaceID: "ws-1",
		worktreeID:  "wt-1",
	})

	run := &guidedworkflows.WorkflowRun{
		ID:        "gwf-queued",
		Status:    guidedworkflows.WorkflowRunStatusQueued,
		CreatedAt: time.Now().UTC(),
	}
	handled, cmd := m.reduceMutationMessages(workflowRunStartedMsg{run: run})
	if !handled {
		t.Fatalf("expected workflowRunStartedMsg to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected follow-up snapshot refresh command")
	}
	if m.status != "guided workflow queued: waiting for dependencies" {
		t.Fatalf("unexpected status message for queued workflow start: %q", m.status)
	}
}
