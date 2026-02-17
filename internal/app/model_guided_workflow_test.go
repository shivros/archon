package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/client"
	"control/internal/guidedworkflows"
)

type guidedWorkflowAPIMock struct {
	createReqs        []client.CreateWorkflowRunRequest
	startRunIDs       []string
	decisionReqs      []client.WorkflowRunDecisionRequest
	createRun         *guidedworkflows.WorkflowRun
	startRun          *guidedworkflows.WorkflowRun
	decisionRun       *guidedworkflows.WorkflowRun
	snapshotRuns      []*guidedworkflows.WorkflowRun
	snapshotTimelines [][]guidedworkflows.RunTimelineEvent
	snapshotRunCalls  int
	snapshotTimeCalls int
}

func (m *guidedWorkflowAPIMock) CreateWorkflowRun(_ context.Context, req client.CreateWorkflowRunRequest) (*guidedworkflows.WorkflowRun, error) {
	m.createReqs = append(m.createReqs, req)
	if m.createRun == nil {
		return nil, nil
	}
	return cloneWorkflowRun(m.createRun), nil
}

func (m *guidedWorkflowAPIMock) StartWorkflowRun(_ context.Context, runID string) (*guidedworkflows.WorkflowRun, error) {
	m.startRunIDs = append(m.startRunIDs, runID)
	if m.startRun == nil {
		return nil, nil
	}
	return cloneWorkflowRun(m.startRun), nil
}

func (m *guidedWorkflowAPIMock) DecideWorkflowRun(_ context.Context, _ string, req client.WorkflowRunDecisionRequest) (*guidedworkflows.WorkflowRun, error) {
	m.decisionReqs = append(m.decisionReqs, req)
	if m.decisionRun == nil {
		return nil, nil
	}
	return cloneWorkflowRun(m.decisionRun), nil
}

func (m *guidedWorkflowAPIMock) GetWorkflowRun(_ context.Context, _ string) (*guidedworkflows.WorkflowRun, error) {
	if len(m.snapshotRuns) == 0 {
		return nil, nil
	}
	idx := min(m.snapshotRunCalls, len(m.snapshotRuns)-1)
	m.snapshotRunCalls++
	return cloneWorkflowRun(m.snapshotRuns[idx]), nil
}

func (m *guidedWorkflowAPIMock) GetWorkflowRunTimeline(_ context.Context, _ string) ([]guidedworkflows.RunTimelineEvent, error) {
	if len(m.snapshotTimelines) == 0 {
		return nil, nil
	}
	idx := min(m.snapshotTimeCalls, len(m.snapshotTimelines)-1)
	m.snapshotTimeCalls++
	return cloneRunTimeline(m.snapshotTimelines[idx]), nil
}

func TestGuidedWorkflowManualStartFlow(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	api := &guidedWorkflowAPIMock{
		createRun: newWorkflowRunFixture("gwf-1", guidedworkflows.WorkflowRunStatusCreated, now),
		startRun:  newWorkflowRunFixture("gwf-1", guidedworkflows.WorkflowRunStatusRunning, now.Add(2*time.Second)),
		snapshotRuns: []*guidedworkflows.WorkflowRun{
			newWorkflowRunFixture("gwf-1", guidedworkflows.WorkflowRunStatusRunning, now.Add(3*time.Second)),
		},
		snapshotTimelines: [][]guidedworkflows.RunTimelineEvent{
			{
				{At: now, Type: "run_created", RunID: "gwf-1"},
				{At: now.Add(time.Second), Type: "run_started", RunID: "gwf-1"},
				{At: now.Add(2 * time.Second), Type: "step_completed", RunID: "gwf-1", Message: "phase plan complete"},
			},
		},
	}

	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, nextModel)
	if cmd != nil {
		t.Fatalf("expected no command when entering setup")
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected setup stage")
	}

	nextModel, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, nextModel)
	if cmd == nil {
		t.Fatalf("expected create workflow command")
	}
	createMsg, ok := cmd().(workflowRunCreatedMsg)
	if !ok {
		t.Fatalf("expected workflowRunCreatedMsg, got %T", cmd())
	}

	nextModel, cmd = m.Update(createMsg)
	m = asModel(t, nextModel)
	if cmd == nil {
		t.Fatalf("expected start workflow command")
	}
	startMsg, ok := cmd().(workflowRunStartedMsg)
	if !ok {
		t.Fatalf("expected workflowRunStartedMsg, got %T", cmd())
	}

	nextModel, cmd = m.Update(startMsg)
	m = asModel(t, nextModel)
	if cmd == nil {
		t.Fatalf("expected snapshot workflow command")
	}
	snapshotMsg, ok := cmd().(workflowRunSnapshotMsg)
	if !ok {
		t.Fatalf("expected workflowRunSnapshotMsg, got %T", cmd())
	}

	nextModel, _ = m.Update(snapshotMsg)
	m = asModel(t, nextModel)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageLive {
		t.Fatalf("expected live stage")
	}
	if len(api.createReqs) != 1 {
		t.Fatalf("expected one create request, got %d", len(api.createReqs))
	}
	if api.createReqs[0].WorkspaceID != "ws1" || api.createReqs[0].WorktreeID != "wt1" || api.createReqs[0].SessionID != "s1" {
		t.Fatalf("unexpected create request context: %+v", api.createReqs[0])
	}
	if !strings.Contains(m.contentRaw, "Live Timeline") {
		t.Fatalf("expected live timeline content, got %q", m.contentRaw)
	}
}

func TestGuidedWorkflowTimelineSnapshotUpdatesArtifacts(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 30, 0, 0, time.UTC)
	m := NewModel(nil)
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
	})

	updated, cmd := m.Update(workflowRunSnapshotMsg{
		run: newWorkflowRunFixture("gwf-2", guidedworkflows.WorkflowRunStatusRunning, now),
		timeline: []guidedworkflows.RunTimelineEvent{
			{At: now, Type: "step_completed", RunID: "gwf-2", Message: "implementation complete"},
			{At: now.Add(2 * time.Second), Type: "step_completed", RunID: "gwf-2", Message: "quality checks complete"},
		},
	})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected no follow-up command")
	}
	if m.guidedWorkflow == nil || len(m.guidedWorkflow.timeline) != 2 {
		t.Fatalf("expected timeline to be stored on controller")
	}
	if !strings.Contains(m.contentRaw, "quality checks complete") {
		t.Fatalf("expected artifact text in content, got %q", m.contentRaw)
	}
}

func TestGuidedWorkflowDecisionApproveFromInbox(t *testing.T) {
	now := time.Date(2026, 2, 17, 13, 0, 0, 0, time.UTC)
	paused := newWorkflowRunFixture("gwf-3", guidedworkflows.WorkflowRunStatusPaused, now)
	paused.LatestDecision = &guidedworkflows.CheckpointDecision{
		ID:       "cd-1",
		RunID:    paused.ID,
		Decision: "decision_needed",
		Metadata: guidedworkflows.CheckpointDecisionMetadata{
			Action:     guidedworkflows.CheckpointActionPause,
			Severity:   guidedworkflows.DecisionSeverityMedium,
			Tier:       guidedworkflows.DecisionTier1,
			Confidence: 0.42,
			Score:      0.64,
			Reasons: []guidedworkflows.CheckpointReason{
				{Code: "confidence_below_threshold", Message: "confidence below threshold"},
			},
		},
	}
	running := newWorkflowRunFixture("gwf-3", guidedworkflows.WorkflowRunStatusRunning, now.Add(3*time.Second))
	api := &guidedWorkflowAPIMock{
		decisionRun: running,
		snapshotRuns: []*guidedworkflows.WorkflowRun{
			running,
		},
		snapshotTimelines: [][]guidedworkflows.RunTimelineEvent{
			{
				{At: now, Type: "run_paused", RunID: "gwf-3"},
				{At: now.Add(time.Second), Type: "decision_approved_continue", RunID: "gwf-3"},
			},
		},
	}

	m := NewModel(nil)
	m.guidedWorkflowAPI = api
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{workspaceID: "ws1", worktreeID: "wt1"})
	updated, _ := m.Update(workflowRunSnapshotMsg{
		run: paused,
		timeline: []guidedworkflows.RunTimelineEvent{
			{At: now, Type: "run_paused", RunID: "gwf-3"},
		},
	})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || !m.guidedWorkflow.NeedsDecision() {
		t.Fatalf("expected pending decision in guided workflow")
	}
	if !strings.Contains(m.contentRaw, "Decision Inbox") {
		t.Fatalf("expected decision inbox content")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected decision command")
	}
	decisionMsg, ok := cmd().(workflowRunDecisionMsg)
	if !ok {
		t.Fatalf("expected workflowRunDecisionMsg, got %T", cmd())
	}
	updated, cmd = m.Update(decisionMsg)
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected snapshot refresh after decision")
	}
	snapshotMsg, ok := cmd().(workflowRunSnapshotMsg)
	if !ok {
		t.Fatalf("expected workflowRunSnapshotMsg, got %T", cmd())
	}
	updated, _ = m.Update(snapshotMsg)
	m = asModel(t, updated)

	if len(api.decisionReqs) != 1 {
		t.Fatalf("expected one decision request, got %d", len(api.decisionReqs))
	}
	if api.decisionReqs[0].Action != guidedworkflows.DecisionActionApproveContinue {
		t.Fatalf("unexpected decision action: %q", api.decisionReqs[0].Action)
	}
	if api.decisionReqs[0].DecisionID != "cd-1" {
		t.Fatalf("expected decision id cd-1, got %q", api.decisionReqs[0].DecisionID)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.NeedsDecision() {
		t.Fatalf("expected decision inbox to clear after approval")
	}
}

func newWorkflowRunFixture(id string, status guidedworkflows.WorkflowRunStatus, now time.Time) *guidedworkflows.WorkflowRun {
	startedAt := now
	return &guidedworkflows.WorkflowRun{
		ID:              id,
		TemplateID:      guidedworkflows.TemplateIDSolidPhaseDelivery,
		TemplateName:    "SOLID Phase Delivery",
		WorkspaceID:     "ws1",
		WorktreeID:      "wt1",
		SessionID:       "s1",
		Mode:            "guarded_autopilot",
		CheckpointStyle: guidedworkflows.DefaultCheckpointStyle,
		Status:          status,
		StartedAt:       &startedAt,
		Phases: []guidedworkflows.PhaseRun{
			{
				ID:     "phase_delivery",
				Name:   "Phase Delivery",
				Status: guidedworkflows.PhaseRunStatusRunning,
				Steps: []guidedworkflows.StepRun{
					{ID: "phase_plan", Name: "phase plan", Status: guidedworkflows.StepRunStatusCompleted},
					{ID: "implementation", Name: "implementation", Status: guidedworkflows.StepRunStatusRunning},
					{ID: "solid_audit", Name: "SOLID audit", Status: guidedworkflows.StepRunStatusPending},
				},
			},
		},
	}
}
