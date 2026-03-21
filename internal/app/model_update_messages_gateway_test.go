package app

import (
	"testing"
	"time"

	"control/internal/guidedworkflows"
)

func TestReduceMutationMessagesRoutesRunStateMutationsThroughGateway(t *testing.T) {
	now := time.Date(2026, 2, 24, 9, 0, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-gateway-routing", guidedworkflows.WorkflowRunStatusRunning, now)
	paused := newWorkflowRunFixture("gwf-gateway-routing", guidedworkflows.WorkflowRunStatusPaused, now.Add(10*time.Second))

	tests := []struct {
		name          string
		setup         func(*Model)
		msg           any
		wantRunCalls  int
		wantSnapCalls int
	}{
		{
			name:         "created",
			msg:          workflowRunCreatedMsg{run: run},
			wantRunCalls: 1,
		},
		{
			name:         "started",
			msg:          workflowRunStartedMsg{run: run},
			wantRunCalls: 1,
		},
		{
			name:         "stopped",
			msg:          workflowRunStoppedMsg{run: run},
			wantRunCalls: 1,
		},
		{
			name:         "resumed",
			msg:          workflowRunResumedMsg{run: run},
			wantRunCalls: 1,
		},
		{
			name:         "decision",
			msg:          workflowRunDecisionMsg{run: paused},
			wantRunCalls: 1,
		},
		{
			name: "snapshot",
			setup: func(m *Model) {
				m.mode = uiModeGuidedWorkflow
				m.guidedWorkflow.SetRun(run)
			},
			msg:           workflowRunSnapshotMsg{run: run, timeline: []guidedworkflows.RunTimelineEvent{}},
			wantSnapCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &stubGuidedWorkflowStateTransitionGateway{}
			m := NewModel(nil, WithGuidedWorkflowStateTransitionGateway(gateway))
			if tc.setup != nil {
				tc.setup(&m)
			}

			handled, _ := m.reduceMutationMessages(tc.msg)
			if !handled {
				t.Fatalf("expected message to be handled")
			}
			if gateway.applyRunCalls != tc.wantRunCalls {
				t.Fatalf("expected ApplyRun calls=%d, got %d", tc.wantRunCalls, gateway.applyRunCalls)
			}
			if gateway.applySnapshotCalls != tc.wantSnapCalls {
				t.Fatalf("expected ApplySnapshot calls=%d, got %d", tc.wantSnapCalls, gateway.applySnapshotCalls)
			}
		})
	}
}

func TestReduceMutationMessagesSnapshotSkipsGatewayWhenSnapshotNotApplicable(t *testing.T) {
	now := time.Date(2026, 2, 24, 9, 10, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-gateway-skip", guidedworkflows.WorkflowRunStatusRunning, now)

	gateway := &stubGuidedWorkflowStateTransitionGateway{}
	m := NewModel(nil, WithGuidedWorkflowStateTransitionGateway(gateway))
	m.mode = uiModeGuidedWorkflow
	m.guidedWorkflow.SetRun(newWorkflowRunFixture("different-run", guidedworkflows.WorkflowRunStatusRunning, now))

	handled, _ := m.reduceMutationMessages(workflowRunSnapshotMsg{run: run, timeline: []guidedworkflows.RunTimelineEvent{}})
	if !handled {
		t.Fatalf("expected snapshot message to be handled")
	}
	if gateway.applySnapshotCalls != 0 {
		t.Fatalf("expected snapshot gateway calls to remain 0, got %d", gateway.applySnapshotCalls)
	}
}

func TestDismissWorkflowRunLocallyRoutesThroughGatewayForSelectedRun(t *testing.T) {
	now := time.Date(2026, 2, 24, 9, 15, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-dismiss-gateway", guidedworkflows.WorkflowRunStatusRunning, now)

	gateway := &stubGuidedWorkflowStateTransitionGateway{}
	m := NewModel(nil, WithGuidedWorkflowStateTransitionGateway(gateway))
	m.guidedWorkflow.SetRun(run)
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}

	if !m.dismissWorkflowRunLocally(run.ID) {
		t.Fatalf("expected local dismiss to succeed")
	}
	if gateway.applyRunCalls != 1 {
		t.Fatalf("expected dismiss to route through gateway once, got %d", gateway.applyRunCalls)
	}
}
