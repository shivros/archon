package app

import (
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
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
			if gateway.applyPreviewSnapshotCalls != 0 {
				t.Fatalf("expected ApplyPreviewSnapshot calls=0, got %d", gateway.applyPreviewSnapshotCalls)
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

func TestReduceMutationMessagesSnapshotRoutesToPreviewGatewayWhenPassiveWorkflowSelected(t *testing.T) {
	now := time.Date(2026, 2, 24, 9, 12, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-gateway-preview", guidedworkflows.WorkflowRunStatusRunning, now)

	gateway := &stubGuidedWorkflowStateTransitionGateway{}
	m := NewModel(nil, WithGuidedWorkflowStateTransitionGateway(gateway))
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	m.sessions = []*types.Session{{ID: "s1", CreatedAt: now, Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: run.ID},
	}
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}
	m.applySidebarItems()
	if m.sidebar == nil || !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected workflow row to be selectable")
	}
	_ = m.onSelectionChangedImmediate()

	handled, _ := m.reduceMutationMessages(workflowRunSnapshotMsg{
		run:      run,
		timeline: []guidedworkflows.RunTimelineEvent{{At: now, Type: "run_started", RunID: run.ID}},
	})
	if !handled {
		t.Fatalf("expected snapshot message to be handled")
	}
	if gateway.applySnapshotCalls != 0 {
		t.Fatalf("expected interactive snapshot gateway calls to remain 0, got %d", gateway.applySnapshotCalls)
	}
	if gateway.applyPreviewSnapshotCalls != 1 {
		t.Fatalf("expected preview snapshot gateway call, got %d", gateway.applyPreviewSnapshotCalls)
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

func TestReduceMutationMessagesSnapshotRoutesWithSegregatedGateways(t *testing.T) {
	now := time.Date(2026, 2, 24, 9, 20, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-gateway-segregated", guidedworkflows.WorkflowRunStatusRunning, now)

	interactive := &stubGuidedWorkflowInteractiveStateTransitionGateway{}
	preview := &stubGuidedWorkflowPreviewStateTransitionGateway{}
	m := NewModel(nil,
		WithGuidedWorkflowInteractiveStateTransitionGateway(interactive),
		WithGuidedWorkflowPreviewStateTransitionGateway(preview),
	)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	m.sessions = []*types.Session{{ID: "s1", CreatedAt: now, Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: run.ID},
	}
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}
	m.applySidebarItems()
	if m.sidebar == nil || !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected workflow row to be selectable")
	}
	_ = m.onSelectionChangedImmediate()

	handled, _ := m.reduceMutationMessages(workflowRunSnapshotMsg{
		run:      run,
		timeline: []guidedworkflows.RunTimelineEvent{{At: now, Type: "run_started", RunID: run.ID}},
	})
	if !handled {
		t.Fatalf("expected snapshot message to be handled")
	}
	if interactive.applySnapshotCalls != 0 {
		t.Fatalf("expected interactive snapshot calls to remain 0, got %d", interactive.applySnapshotCalls)
	}
	if preview.applyPreviewSnapshotCalls != 1 {
		t.Fatalf("expected preview snapshot gateway call, got %d", preview.applyPreviewSnapshotCalls)
	}
}

func TestReduceMutationMessagesSnapshotInGuidedModeDoesNotRouteToPreviewGateway(t *testing.T) {
	now := time.Date(2026, 2, 24, 9, 22, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-gateway-guided-only", guidedworkflows.WorkflowRunStatusRunning, now)

	interactive := &stubGuidedWorkflowInteractiveStateTransitionGateway{}
	preview := &stubGuidedWorkflowPreviewStateTransitionGateway{}
	m := NewModel(nil,
		WithGuidedWorkflowInteractiveStateTransitionGateway(interactive),
		WithGuidedWorkflowPreviewStateTransitionGateway(preview),
	)
	m.mode = uiModeGuidedWorkflow
	m.guidedWorkflow.SetRun(run)

	handled, _ := m.reduceMutationMessages(workflowRunSnapshotMsg{
		run:      run,
		timeline: []guidedworkflows.RunTimelineEvent{{At: now, Type: "run_started", RunID: run.ID}},
	})
	if !handled {
		t.Fatalf("expected snapshot message to be handled")
	}
	if interactive.applySnapshotCalls != 1 {
		t.Fatalf("expected interactive snapshot gateway call, got %d", interactive.applySnapshotCalls)
	}
	if preview.applyPreviewSnapshotCalls != 0 {
		t.Fatalf("expected preview snapshot gateway call to remain 0, got %d", preview.applyPreviewSnapshotCalls)
	}
}
