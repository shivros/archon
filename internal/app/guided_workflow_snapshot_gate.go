package app

import (
	"strings"

	"control/internal/guidedworkflows"
)

func shouldApplyWorkflowSnapshotToGuidedController(m *Model, run *guidedworkflows.WorkflowRun) bool {
	if m == nil || m.mode != uiModeGuidedWorkflow || m.guidedWorkflow == nil || run == nil {
		return false
	}
	incomingRunID := strings.TrimSpace(run.ID)
	if incomingRunID == "" {
		return false
	}
	currentRunID := strings.TrimSpace(m.guidedWorkflow.RunID())
	return currentRunID != "" && incomingRunID == currentRunID
}

func shouldApplyWorkflowSnapshotErrorToGuidedController(m *Model) bool {
	if m == nil || m.mode != uiModeGuidedWorkflow || m.guidedWorkflow == nil {
		return false
	}
	return strings.TrimSpace(m.guidedWorkflow.RunID()) != ""
}
