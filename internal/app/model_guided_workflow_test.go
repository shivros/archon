package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/client"
	"control/internal/guidedworkflows"
	"control/internal/types"
)

type guidedWorkflowAPIMock struct {
	listRuns          []*guidedworkflows.WorkflowRun
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

func (m *guidedWorkflowAPIMock) ListWorkflowRuns(_ context.Context) ([]*guidedworkflows.WorkflowRun, error) {
	out := make([]*guidedworkflows.WorkflowRun, 0, len(m.listRuns))
	for _, run := range m.listRuns {
		out = append(out, cloneWorkflowRun(run))
	}
	return out, nil
}

func (m *guidedWorkflowAPIMock) ListWorkflowRunsWithOptions(_ context.Context, _ bool) ([]*guidedworkflows.WorkflowRun, error) {
	return m.ListWorkflowRuns(context.Background())
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

func (m *guidedWorkflowAPIMock) DismissWorkflowRun(_ context.Context, _ string) (*guidedworkflows.WorkflowRun, error) {
	if m.startRun == nil {
		return nil, nil
	}
	run := cloneWorkflowRun(m.startRun)
	now := time.Now().UTC()
	run.DismissedAt = &now
	return run, nil
}

func (m *guidedWorkflowAPIMock) UndismissWorkflowRun(_ context.Context, _ string) (*guidedworkflows.WorkflowRun, error) {
	if m.startRun == nil {
		return nil, nil
	}
	run := cloneWorkflowRun(m.startRun)
	run.DismissedAt = nil
	return run, nil
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
	if m.guidedWorkflowPromptInput == nil {
		t.Fatalf("expected guided workflow prompt input")
	}
	m.guidedWorkflowPromptInput.SetValue("Fix bug in request routing")
	m.syncGuidedWorkflowPromptInput()

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
	if api.createReqs[0].UserPrompt != "Fix bug in request routing" {
		t.Fatalf("expected user prompt in create request, got %q", api.createReqs[0].UserPrompt)
	}
	if !strings.Contains(m.contentRaw, "Live Timeline") {
		t.Fatalf("expected live timeline content, got %q", m.contentRaw)
	}
}

func TestGuidedWorkflowSetupRequiresUserPrompt(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected setup stage")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected start to be blocked when user prompt is empty")
	}
	if !strings.Contains(strings.ToLower(m.status), "workflow prompt") {
		t.Fatalf("expected prompt validation status, got %q", m.status)
	}
}

func TestGuidedWorkflowSetupCapturesPromptFromKeys(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected setup stage")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = asModel(t, updated)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = asModel(t, updated)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = asModel(t, updated)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = asModel(t, updated)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = asModel(t, updated)

	if got := m.guidedWorkflow.UserPrompt(); got != "hi" {
		t.Fatalf("expected prompt input to capture typed text with backspace, got %q", got)
	}
}

func TestGuidedWorkflowSetupCapturesPromptFromPaste(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected setup stage")
	}

	updated, _ = m.Update(tea.PasteMsg{Content: "Fix flaky retry logic\nand add coverage"})
	m = asModel(t, updated)
	if got := m.guidedWorkflow.UserPrompt(); got != "Fix flaky retry logic\nand add coverage" {
		t.Fatalf("expected prompt input to capture pasted text, got %q", got)
	}
	inputView, _ := m.modeInputView()
	if strings.Contains(inputView, "Task Description") {
		t.Fatalf("expected setup input panel to omit instructional footer text, got %q", inputView)
	}
}

func TestGuidedWorkflowSetupTypingQDoesNotQuit(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected setup stage")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = asModel(t, updated)
	if cmd != nil {
		if _, quitting := cmd().(tea.QuitMsg); quitting {
			t.Fatalf("expected typing q in setup prompt not to quit")
		}
	}
	if got := m.guidedWorkflow.UserPrompt(); got != "q" {
		t.Fatalf("expected q to be captured in prompt input, got %q", got)
	}
}

func TestGuidedWorkflowSetupSubmitRemapStartsRun(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	api := &guidedWorkflowAPIMock{
		createRun: newWorkflowRunFixture("gwf-remap", guidedworkflows.WorkflowRunStatusCreated, now),
	}

	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandInputSubmit:  "f6",
		KeyCommandComposeModel: "f6",
	}))
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected setup stage")
	}

	m.guidedWorkflowPromptInput.SetValue("Fix bug in request routing")
	m.syncGuidedWorkflowPromptInput()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyF6})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected create workflow command from remapped submit key")
	}
	if _, ok := cmd().(workflowRunCreatedMsg); !ok {
		t.Fatalf("expected workflowRunCreatedMsg, got %T", cmd())
	}
	if len(api.createReqs) != 1 {
		t.Fatalf("expected one create request, got %d", len(api.createReqs))
	}
	if api.createReqs[0].UserPrompt != "Fix bug in request routing" {
		t.Fatalf("expected user prompt in create request, got %q", api.createReqs[0].UserPrompt)
	}
}

func TestGuidedWorkflowSetupResizesViewportOnEnterAndInputGrowth(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	m.resize(120, 40)
	launcherViewportHeight := m.viewport.Height()
	if launcherViewportHeight <= 0 {
		t.Fatalf("expected launcher viewport height > 0")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected setup stage")
	}
	setupViewportHeight := m.viewport.Height()
	if setupViewportHeight >= launcherViewportHeight {
		t.Fatalf("expected setup viewport to shrink for input panel: launcher=%d setup=%d", launcherViewportHeight, setupViewportHeight)
	}

	updated, _ = m.Update(tea.PasteMsg{Content: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11"})
	m = asModel(t, updated)
	if got := m.guidedWorkflowPromptInput.Height(); got < 8 {
		t.Fatalf("expected prompt input to auto-grow on multiline paste, got height=%d", got)
	}
	grownViewportHeight := m.viewport.Height()
	if grownViewportHeight >= setupViewportHeight {
		t.Fatalf("expected viewport to shrink after prompt growth: before=%d after=%d", setupViewportHeight, grownViewportHeight)
	}

	visibleLines := 1 + m.viewport.Height() + 1 + m.guidedWorkflowSetupInputLineCount()
	maxContentLines := m.height - 1
	if visibleLines > maxContentLines {
		t.Fatalf("expected guided setup layout to fit viewport; visible=%d max=%d", visibleLines, maxContentLines)
	}
}

func TestGuidedWorkflowSetupContentNotOverwrittenBySidebarRefresh(t *testing.T) {
	m := newPhase0ModelWithSession("codex")

	workspaceRow := -1
	for idx, item := range m.sidebar.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.kind != sidebarWorkspace {
			continue
		}
		workspaceRow = idx
		break
	}
	if workspaceRow < 0 {
		t.Fatalf("expected workspace row in sidebar")
	}
	m.sidebar.Select(workspaceRow)

	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
	})
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected setup stage")
	}
	if !strings.Contains(m.contentRaw, "Run Setup") {
		t.Fatalf("expected setup content before sidebar refresh, got %q", m.contentRaw)
	}

	m.applySidebarItems()
	if strings.Contains(m.contentRaw, "Select a session.") {
		t.Fatalf("expected guided workflow content not to be overwritten by sidebar refresh, got %q", m.contentRaw)
	}
	if !strings.Contains(m.contentRaw, "Run Setup") {
		t.Fatalf("expected setup content to remain visible after sidebar refresh, got %q", m.contentRaw)
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

func TestGuidedWorkflowTimelineShowsStepSessionTraceability(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 45, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-trace", guidedworkflows.WorkflowRunStatusRunning, now)
	run.CurrentPhaseIndex = 0
	run.CurrentStepIndex = 1
	run.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
		SessionID:      "s1",
		Provider:       "codex",
		Model:          "gpt-5",
		TurnID:         "turn-42",
		PromptSnapshot: "implementation prompt",
		TraceID:        "gwf-trace:phase_delivery:implementation:attempt-1",
	}
	run.Phases[0].Steps[1].ExecutionState = guidedworkflows.StepExecutionStateLinked

	m := newPhase0ModelWithSession("codex")
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
	})
	updated, _ := m.Update(workflowRunSnapshotMsg{
		run: run,
		timeline: []guidedworkflows.RunTimelineEvent{
			{At: now, Type: "step_dispatched", RunID: "gwf-trace", Message: "awaiting turn completion"},
		},
	})
	m = asModel(t, updated)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = asModel(t, updated)
	if !strings.Contains(m.contentRaw, "[session:s1 turn:turn-42]") {
		t.Fatalf("expected session chip in timeline, got %q", m.contentRaw)
	}
	if !strings.Contains(m.contentRaw, "Execution Details") {
		t.Fatalf("expected execution details section")
	}
	if !strings.Contains(m.contentRaw, "Trace id: gwf-trace:phase_delivery:implementation:attempt-1") {
		t.Fatalf("expected trace id in execution details")
	}
}

func TestGuidedWorkflowOpenSelectedStepSession(t *testing.T) {
	now := time.Date(2026, 2, 17, 13, 15, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-open", guidedworkflows.WorkflowRunStatusRunning, now)
	run.CurrentPhaseIndex = 0
	run.CurrentStepIndex = 1
	run.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
		SessionID: "s1",
		TurnID:    "turn-99",
	}
	run.Phases[0].Steps[1].ExecutionState = guidedworkflows.StepExecutionStateLinked

	m := newPhase0ModelWithSession("codex")
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
	})
	updated, _ := m.Update(workflowRunSnapshotMsg{run: run})
	m = asModel(t, updated)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode before open action")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = asModel(t, updated)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected session open command")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected guided workflow to close after opening linked session, got mode=%v", m.mode)
	}
	if selected := m.selectedSessionID(); selected != "s1" {
		t.Fatalf("expected linked session s1 to be selected, got %q", selected)
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

func TestSelectingWorkflowSidebarNodeOpensGuidedWorkflowView(t *testing.T) {
	now := time.Date(2026, 2, 17, 13, 30, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-sidebar", guidedworkflows.WorkflowRunStatusRunning, now)
	api := &guidedWorkflowAPIMock{
		snapshotRuns: []*guidedworkflows.WorkflowRun{run},
		snapshotTimelines: [][]guidedworkflows.RunTimelineEvent{
			{
				{At: now, Type: "run_started", RunID: run.ID},
			},
		},
	}
	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}
	m.sessionMeta["s1"] = &types.SessionMeta{
		SessionID:     "s1",
		WorkspaceID:   "ws1",
		WorkflowRunID: run.ID,
	}
	m.applySidebarItems()

	workflowRow := -1
	for idx, item := range m.sidebar.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.kind != sidebarWorkflow {
			continue
		}
		if entry.workflowRunID() == run.ID {
			workflowRow = idx
			break
		}
	}
	if workflowRow < 0 {
		t.Fatalf("expected workflow row in sidebar")
	}

	m.sidebar.Select(workflowRow)
	cmd := m.onSelectionChangedImmediate()
	if cmd == nil {
		t.Fatalf("expected snapshot command after selecting workflow row")
	}
	msg, ok := cmd().(workflowRunSnapshotMsg)
	if !ok {
		t.Fatalf("expected workflowRunSnapshotMsg, got %T", cmd())
	}
	updated, _ := m.Update(msg)
	m = asModel(t, updated)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.RunID() != run.ID {
		t.Fatalf("expected guided workflow run %q, got %#v", run.ID, m.guidedWorkflow)
	}
}

func TestSelectingWorkflowChildSessionLoadsChat(t *testing.T) {
	now := time.Date(2026, 2, 17, 13, 35, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-child", guidedworkflows.WorkflowRunStatusRunning, now)
	m := newPhase0ModelWithSession("codex")
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}
	m.sessionMeta["s1"] = &types.SessionMeta{
		SessionID:     "s1",
		WorkspaceID:   "ws1",
		WorkflowRunID: run.ID,
	}
	m.applySidebarItems()

	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected workflow child session row to be selectable")
	}
	cmd := m.onSelectionChangedImmediate()
	if cmd == nil {
		t.Fatalf("expected session load command for child session")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected normal mode to remain active when selecting child session")
	}
}

func TestGuidedWorkflowSummaryRendersReadableLineBreaks(t *testing.T) {
	now := time.Date(2026, 2, 18, 9, 0, 0, 0, time.UTC)
	completedAt := now.Add(10 * time.Minute)
	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{workspaceID: "ws1"})
	controller.SetRun(&guidedworkflows.WorkflowRun{
		ID:           "gwf-format",
		TemplateID:   guidedworkflows.TemplateIDSolidPhaseDelivery,
		TemplateName: "SOLID Phase Delivery",
		Status:       guidedworkflows.WorkflowRunStatusCompleted,
		CompletedAt:  &completedAt,
		Phases: []guidedworkflows.PhaseRun{
			{
				ID:   "phase_delivery",
				Name: "Phase Delivery",
				Steps: []guidedworkflows.StepRun{
					{ID: "phase_plan", Name: "phase plan", Status: guidedworkflows.StepRunStatusCompleted},
					{ID: "implementation", Name: "implementation", Status: guidedworkflows.StepRunStatusCompleted},
				},
			},
		},
	})
	content := controller.Render()
	if !strings.Contains(content, "- Final status: completed  \n- Completed steps: 2/2") {
		t.Fatalf("expected summary fields to render on separate lines, got %q", content)
	}
	if !strings.Contains(content, "- Completed steps: 2/2  \n- Decisions requested: 0") {
		t.Fatalf("expected decision count on a separate line, got %q", content)
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
