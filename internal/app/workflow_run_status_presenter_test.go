package app

import (
	"testing"
	"time"

	"control/internal/guidedworkflows"
)

func TestWorkflowRunSemanticStateForRunMappingAndDismissedPrecedence(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name string
		run  *guidedworkflows.WorkflowRun
		want workflowRunSemanticState
	}{
		{name: "nil", run: nil, want: workflowRunStateUnknown},
		{name: "created", run: &guidedworkflows.WorkflowRun{Status: guidedworkflows.WorkflowRunStatusCreated}, want: workflowRunStateCreated},
		{name: "queued", run: &guidedworkflows.WorkflowRun{Status: guidedworkflows.WorkflowRunStatusQueued}, want: workflowRunStateQueued},
		{name: "running", run: &guidedworkflows.WorkflowRun{Status: guidedworkflows.WorkflowRunStatusRunning}, want: workflowRunStateRunning},
		{name: "paused", run: &guidedworkflows.WorkflowRun{Status: guidedworkflows.WorkflowRunStatusPaused}, want: workflowRunStatePaused},
		{name: "stopped", run: &guidedworkflows.WorkflowRun{Status: guidedworkflows.WorkflowRunStatusStopped}, want: workflowRunStateStopped},
		{name: "completed", run: &guidedworkflows.WorkflowRun{Status: guidedworkflows.WorkflowRunStatusCompleted}, want: workflowRunStateCompleted},
		{name: "failed", run: &guidedworkflows.WorkflowRun{Status: guidedworkflows.WorkflowRunStatusFailed}, want: workflowRunStateFailed},
		{name: "unknown", run: &guidedworkflows.WorkflowRun{Status: guidedworkflows.WorkflowRunStatus("custom")}, want: workflowRunStateUnknown},
		{
			name: "dismissed precedence",
			run: &guidedworkflows.WorkflowRun{
				Status:      guidedworkflows.WorkflowRunStatusRunning,
				DismissedAt: &now,
			},
			want: workflowRunStateDismissed,
		},
	}
	for _, tc := range cases {
		if got := workflowRunSemanticStateForRun(tc.run); got != tc.want {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, got)
		}
	}
}

func TestWorkflowRunStatusPresentationsCoverExpectedSemanticStates(t *testing.T) {
	expectedStates := []workflowRunSemanticState{
		workflowRunStateDismissed,
		workflowRunStateCreated,
		workflowRunStateQueued,
		workflowRunStateRunning,
		workflowRunStatePaused,
		workflowRunStateStopped,
		workflowRunStateCompleted,
		workflowRunStateFailed,
	}
	presentations := []struct {
		name   string
		labels map[workflowRunSemanticState]string
	}{
		{name: "compact", labels: workflowRunCompactStatusPresentation.labels},
		{name: "detailed", labels: workflowRunDetailedStatusPresentation.labels},
	}
	for _, presentation := range presentations {
		for _, state := range expectedStates {
			label, ok := presentation.labels[state]
			if !ok {
				t.Fatalf("%s presentation missing label for semantic state %v", presentation.name, state)
			}
			if label == "" {
				t.Fatalf("%s presentation has empty label for semantic state %v", presentation.name, state)
			}
		}
		if _, ok := presentation.labels[workflowRunStateUnknown]; ok {
			t.Fatalf("%s presentation should not define a label for unknown semantic state", presentation.name)
		}
	}
}
