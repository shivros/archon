package app

import "testing"

func TestExitGuidedWorkflowNilModelNoPanic(t *testing.T) {
	var m *Model
	m.exitGuidedWorkflow("")
}
