package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"control/internal/client"
	"control/internal/config"
	"control/internal/guidedworkflows"
	"control/internal/types"
)

type guidedWorkflowAPIMock struct {
	listRuns          []*guidedworkflows.WorkflowRun
	listTemplates     []guidedworkflows.WorkflowTemplate
	listTemplatesErr  error
	createReqs        []client.CreateWorkflowRunRequest
	startRunIDs       []string
	stopRunIDs        []string
	resumeRunIDs      []string
	decisionReqs      []client.WorkflowRunDecisionRequest
	resumeReqs        []client.WorkflowRunResumeRequest
	createRun         *guidedworkflows.WorkflowRun
	startRun          *guidedworkflows.WorkflowRun
	stopRun           *guidedworkflows.WorkflowRun
	resumeRun         *guidedworkflows.WorkflowRun
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

func (m *guidedWorkflowAPIMock) ListWorkflowTemplates(_ context.Context) ([]guidedworkflows.WorkflowTemplate, error) {
	if m.listTemplatesErr != nil {
		return nil, m.listTemplatesErr
	}
	out := make([]guidedworkflows.WorkflowTemplate, 0, len(m.listTemplates))
	for _, tpl := range m.listTemplates {
		out = append(out, tpl)
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

func (m *guidedWorkflowAPIMock) StopWorkflowRun(_ context.Context, runID string) (*guidedworkflows.WorkflowRun, error) {
	m.stopRunIDs = append(m.stopRunIDs, runID)
	if m.stopRun == nil {
		return nil, nil
	}
	return cloneWorkflowRun(m.stopRun), nil
}

func (m *guidedWorkflowAPIMock) ResumeFailedWorkflowRun(_ context.Context, runID string, req client.WorkflowRunResumeRequest) (*guidedworkflows.WorkflowRun, error) {
	m.resumeRunIDs = append(m.resumeRunIDs, runID)
	m.resumeReqs = append(m.resumeReqs, req)
	if m.resumeRun == nil {
		return nil, nil
	}
	return cloneWorkflowRun(m.resumeRun), nil
}

func (m *guidedWorkflowAPIMock) RenameWorkflowRun(_ context.Context, runID, name string) (*guidedworkflows.WorkflowRun, error) {
	_ = runID
	if m.startRun == nil {
		return nil, nil
	}
	run := cloneWorkflowRun(m.startRun)
	run.TemplateName = strings.TrimSpace(name)
	return run, nil
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

func enterGuidedWorkflowForTest(m *Model, context guidedWorkflowLaunchContext) {
	if m == nil {
		return
	}
	m.enterGuidedWorkflow(context)
	if m.guidedWorkflow == nil {
		return
	}
	m.guidedWorkflow.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{
			ID:          guidedworkflows.TemplateIDSolidPhaseDelivery,
			Name:        "SOLID Phase Delivery",
			Description: "Default guided workflow template.",
		},
		{
			ID:          "custom_triage",
			Name:        "Bug Triage",
			Description: "Fast triage template.",
		},
	})
	m.renderGuidedWorkflowContent()
}

func advanceGuidedWorkflowToComposerForTest(t *testing.T, m *Model) {
	t.Helper()
	if m == nil {
		t.Fatalf("model is nil")
	}
	for i := 0; i < 3; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		*m = asModel(t, next)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected setup stage after launcher/provider/policy flow")
	}
}

func workflowRunSnapshotMsgFromCmd(t *testing.T, cmd tea.Cmd) workflowRunSnapshotMsg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected workflow snapshot command")
	}
	var findSnapshot func(msg tea.Msg) (workflowRunSnapshotMsg, bool)
	findSnapshot = func(msg tea.Msg) (workflowRunSnapshotMsg, bool) {
		switch typed := msg.(type) {
		case workflowRunSnapshotMsg:
			return typed, true
		case tea.BatchMsg:
			for _, nested := range typed {
				if nested == nil {
					continue
				}
				nestedMsg := nested()
				if snapshot, ok := findSnapshot(nestedMsg); ok {
					return snapshot, true
				}
			}
		}
		return workflowRunSnapshotMsg{}, false
	}
	msg := cmd()
	snapshot, ok := findSnapshot(msg)
	if !ok {
		t.Fatalf("expected workflowRunSnapshotMsg, got %T", msg)
	}
	return snapshot
}

func TestGuidedWorkflowManualStartFlow(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	createRun := newWorkflowRunFixture("gwf-1", guidedworkflows.WorkflowRunStatusCreated, now)
	createRun.UserPrompt = "Fix bug in request routing"
	startRun := newWorkflowRunFixture("gwf-1", guidedworkflows.WorkflowRunStatusRunning, now.Add(2*time.Second))
	startRun.UserPrompt = "Fix bug in request routing"
	snapshotRun := newWorkflowRunFixture("gwf-1", guidedworkflows.WorkflowRunStatusRunning, now.Add(3*time.Second))
	snapshotRun.UserPrompt = "Fix bug in request routing"
	api := &guidedWorkflowAPIMock{
		createRun: createRun,
		startRun:  startRun,
		snapshotRuns: []*guidedworkflows.WorkflowRun{
			snapshotRun,
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
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)
	if m.guidedWorkflowPromptInput == nil {
		t.Fatalf("expected guided workflow prompt input")
	}
	m.guidedWorkflowPromptInput.SetValue("Fix bug in request routing")
	m.syncGuidedWorkflowPromptInput()

	var (
		nextModel tea.Model
		cmd       tea.Cmd
	)
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
	snapshotMsg := workflowRunSnapshotMsgFromCmd(t, cmd)

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
	if !strings.Contains(m.contentRaw, "Original prompt: Fix bug in request routing") {
		t.Fatalf("expected original prompt in live content, got %q", m.contentRaw)
	}
}

func TestGuidedWorkflowSetupRequiresUserPrompt(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected start to be blocked when user prompt is empty")
	}
	if !strings.Contains(strings.ToLower(m.status), "workflow prompt") {
		t.Fatalf("expected prompt validation status, got %q", m.status)
	}
}

func TestGuidedWorkflowLauncherBlocksSetupUntilTemplatesLoaded(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected no command while templates are loading")
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageLauncher {
		t.Fatalf("expected launcher stage while templates are loading")
	}
	if !strings.Contains(strings.ToLower(m.status), "loading") {
		t.Fatalf("expected loading validation status, got %q", m.status)
	}
}

func TestGuidedWorkflowSetupUsesSelectedTemplate(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	api := &guidedWorkflowAPIMock{
		createRun: newWorkflowRunFixture("gwf-template-select", guidedworkflows.WorkflowRunStatusCreated, now),
	}

	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = asModel(t, updated)
	if !strings.Contains(m.contentRaw, "Bug Triage") {
		t.Fatalf("expected launcher to show alternate template option")
	}

	advanceGuidedWorkflowToComposerForTest(t, &m)

	m.guidedWorkflowPromptInput.SetValue("Triage flaky parser test")
	m.syncGuidedWorkflowPromptInput()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected create workflow command")
	}
	if _, ok := cmd().(workflowRunCreatedMsg); !ok {
		t.Fatalf("expected workflowRunCreatedMsg, got %T", cmd())
	}
	if len(api.createReqs) != 1 {
		t.Fatalf("expected one create request, got %d", len(api.createReqs))
	}
	if api.createReqs[0].TemplateID != guidedworkflows.TemplateIDSolidPhaseDelivery {
		t.Fatalf("expected selected template id %q, got %q", guidedworkflows.TemplateIDSolidPhaseDelivery, api.createReqs[0].TemplateID)
	}
}

func TestGuidedWorkflowLauncherDisplaysContextNames(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Payments Workspace"},
	}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "feature/retry-cleanup"},
		},
	}
	m.sessions = []*types.Session{
		{
			ID:       "s1",
			Provider: "codex",
			Title:    "Stabilize retry policy",
		},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {
			SessionID:   "s1",
			WorkspaceID: "ws1",
			WorktreeID:  "wt1",
			Title:       "Retry policy cleanup",
		},
	}

	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	if !strings.Contains(m.contentRaw, "/") || !strings.Contains(m.contentRaw, "Bug Triage") {
		t.Fatalf("expected bare template picker content, got %q", m.contentRaw)
	}
}

func TestGuidedWorkflowLauncherTemplatePickerSupportsTypeAhead(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	var updated tea.Model
	for _, key := range []string{"t", "r", "i", "a", "g", "e"} {
		var cmd tea.Cmd
		updated, cmd = m.Update(tea.KeyPressMsg{Text: key})
		m = asModel(t, updated)
		if cmd != nil {
			t.Fatalf("expected no command while typing launcher filter, got %T", cmd())
		}
	}
	if !strings.Contains(m.contentRaw, "Bug Triage") {
		t.Fatalf("expected filtered template selection, got %q", m.contentRaw)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = asModel(t, updated)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected first esc to clear query but keep launcher open, got mode=%v", m.mode)
	}
	if !strings.Contains(strings.ToLower(m.status), "filter cleared") {
		t.Fatalf("expected filter cleared status, got %q", m.status)
	}
}

func TestGuidedWorkflowLauncherPreservesANSIPickerRendering(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	if !m.guidedWorkflow.LauncherRequiresRawANSIRender() {
		t.Fatalf("expected launcher to require ANSI passthrough when templates are loaded")
	}
	if !m.contentRenderRaw {
		t.Fatalf("expected launcher content to use raw ANSI rendering")
	}
	if !strings.Contains(m.renderedText, "\x1b[") {
		t.Fatalf("expected rendered launcher text to retain ANSI styling, got %q", m.renderedText)
	}
}

func TestGuidedWorkflowLauncherUsesRawPickerRenderingWithoutANSIStyles(t *testing.T) {
	previousSelectedStyle := selectedStyle
	selectedStyle = lipgloss.NewStyle()
	t.Cleanup(func() {
		selectedStyle = previousSelectedStyle
	})

	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	if !m.guidedWorkflow.LauncherRequiresRawANSIRender() {
		t.Fatalf("expected launcher picker to require raw rendering even without ANSI styles")
	}
	if !m.contentRenderRaw {
		t.Fatalf("expected launcher content to use raw rendering when styles are plain text")
	}
	if strings.Contains(m.renderedText, "\x1b[") {
		t.Fatalf("expected rendered launcher text without ANSI styling, got %q", m.renderedText)
	}
}

func TestGuidedWorkflowLauncherTemplatePickerLayoutMetadata(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	layout, ok := m.guidedWorkflow.LauncherTemplatePickerLayout()
	if !ok {
		t.Fatalf("expected launcher picker layout metadata")
	}
	if layout.height < 2 {
		t.Fatalf("expected picker layout height >= 2, got %d", layout.height)
	}
	if strings.TrimSpace(layout.queryLine) != "/" {
		t.Fatalf("expected query-line anchor '/', got %q", layout.queryLine)
	}
	if start := m.guidedWorkflowLauncherPickerStartRow(layout); start < 0 {
		t.Fatalf("expected picker start row from rendered content")
	}
}

func TestGuidedWorkflowSetupCapturesPromptFromKeys(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
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
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)

	updated, _ := m.Update(tea.PasteMsg{Content: "Fix flaky retry logic\nand add coverage"})
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
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)

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

func TestGuidedWorkflowSetupClearCommandClearsPrompt(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)
	if m.guidedWorkflowPromptInput == nil {
		t.Fatalf("expected setup prompt input")
	}
	m.guidedWorkflowPromptInput.SetValue("Fix flaky retry logic")
	m.syncGuidedWorkflowPromptInput()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected no command for clear action")
	}
	if got := m.guidedWorkflowPromptInput.Value(); got != "" {
		t.Fatalf("expected setup prompt input to clear, got %q", got)
	}
	if got := m.guidedWorkflow.UserPrompt(); got != "" {
		t.Fatalf("expected guided workflow prompt to clear, got %q", got)
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
		KeyCommandInputSubmit: "f6",
	}))
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)

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

func TestGuidedWorkflowSetupUsesConfiguredDefaultResolutionBoundary(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	api := &guidedWorkflowAPIMock{
		createRun: newWorkflowRunFixture("gwf-boundary", guidedworkflows.WorkflowRunStatusCreated, now),
	}

	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	m.applyCoreConfig(config.CoreConfig{
		GuidedWorkflows: config.CoreGuidedWorkflowsConfig{
			Defaults: config.CoreGuidedWorkflowsDefaultsConfig{
				ResolutionBoundary: "high",
			},
		},
	})
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)

	m.guidedWorkflowPromptInput.SetValue("Use configured boundary defaults")
	m.syncGuidedWorkflowPromptInput()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected create workflow command")
	}
	if _, ok := cmd().(workflowRunCreatedMsg); !ok {
		t.Fatalf("expected workflowRunCreatedMsg, got %T", cmd())
	}
	if len(api.createReqs) != 1 {
		t.Fatalf("expected one create request, got %d", len(api.createReqs))
	}
	override := api.createReqs[0].PolicyOverrides
	if override == nil {
		t.Fatalf("expected default resolution boundary to set policy overrides")
	}
	if override.ConfidenceThreshold == nil || *override.ConfidenceThreshold != 0.75 {
		t.Fatalf("expected high boundary confidence threshold 0.75, got %#v", override.ConfidenceThreshold)
	}
	if override.PauseThreshold == nil || *override.PauseThreshold != 0.45 {
		t.Fatalf("expected high boundary pause threshold 0.45, got %#v", override.PauseThreshold)
	}
}

func TestGuidedWorkflowSetupKeepsBalancedSensitivityWhenDefaultsInvalid(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	api := &guidedWorkflowAPIMock{
		createRun: newWorkflowRunFixture("gwf-balanced", guidedworkflows.WorkflowRunStatusCreated, now),
	}

	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	m.applyCoreConfig(config.CoreConfig{
		GuidedWorkflows: config.CoreGuidedWorkflowsConfig{
			Defaults: config.CoreGuidedWorkflowsDefaultsConfig{
				ResolutionBoundary: "also-not-a-preset",
			},
		},
	})
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)

	m.guidedWorkflowPromptInput.SetValue("Use balanced fallback")
	m.syncGuidedWorkflowPromptInput()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected create workflow command")
	}
	if _, ok := cmd().(workflowRunCreatedMsg); !ok {
		t.Fatalf("expected workflowRunCreatedMsg, got %T", cmd())
	}
	if len(api.createReqs) != 1 {
		t.Fatalf("expected one create request, got %d", len(api.createReqs))
	}
	if api.createReqs[0].PolicyOverrides != nil {
		t.Fatalf("expected balanced fallback to keep policy overrides unset, got %#v", api.createReqs[0].PolicyOverrides)
	}
}

func TestGuidedPolicySensitivityFromPreset(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		want   guidedPolicySensitivity
	}{
		{name: "low", preset: "low", want: guidedPolicySensitivityLow},
		{name: "high", preset: "HIGH", want: guidedPolicySensitivityHigh},
		{name: "balanced_alias", preset: "default", want: guidedPolicySensitivityBalanced},
		{name: "balanced_alias_separated", preset: "de-fault", want: guidedPolicySensitivityBalanced},
		{name: "empty", preset: "", want: guidedPolicySensitivityBalanced},
		{name: "invalid", preset: "unexpected", want: guidedPolicySensitivityBalanced},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := guidedPolicySensitivityFromPreset(tt.preset); got != tt.want {
				t.Fatalf("guidedPolicySensitivityFromPreset(%q) = %v, want %v", tt.preset, got, tt.want)
			}
		})
	}
}

func TestGuidedWorkflowSetupResizesViewportOnEnterAndInputGrowth(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	m.resize(120, 40)
	launcherViewportHeight := m.viewport.Height()
	if launcherViewportHeight <= 0 {
		t.Fatalf("expected launcher viewport height > 0")
	}

	advanceGuidedWorkflowToComposerForTest(t, &m)
	setupViewportHeight := m.viewport.Height()
	if setupViewportHeight >= launcherViewportHeight {
		t.Fatalf("expected setup viewport to shrink for input panel: launcher=%d setup=%d", launcherViewportHeight, setupViewportHeight)
	}

	updated, _ := m.Update(tea.PasteMsg{Content: "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11"})
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

	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
	})
	advanceGuidedWorkflowToComposerForTest(t, &m)
	if !strings.Contains(m.contentRaw, "Prompt Composer") {
		t.Fatalf("expected setup content before sidebar refresh, got %q", m.contentRaw)
	}

	m.applySidebarItems()
	if strings.Contains(m.contentRaw, "Select a session.") {
		t.Fatalf("expected guided workflow content not to be overwritten by sidebar refresh, got %q", m.contentRaw)
	}
	if !strings.Contains(m.contentRaw, "Prompt Composer") {
		t.Fatalf("expected setup content to remain visible after sidebar refresh, got %q", m.contentRaw)
	}
}

func TestGuidedWorkflowProviderSwitchFallsBackForIncompatibleSelections(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.providerOptions = map[string]*types.ProviderOptionCatalog{
		"codex": {
			Provider:        "codex",
			Models:          []string{"gpt-5.1-codex", "gpt-5.3-codex"},
			ReasoningLevels: []types.ReasoningLevel{types.ReasoningLow, types.ReasoningMedium, types.ReasoningHigh},
			AccessLevels:    []types.AccessLevel{types.AccessReadOnly, types.AccessOnRequest, types.AccessFull},
			Defaults: types.SessionRuntimeOptions{
				Model:     "gpt-5.1-codex",
				Reasoning: types.ReasoningMedium,
				Access:    types.AccessOnRequest,
			},
		},
		"claude": {
			Provider:     "claude",
			Models:       []string{"sonnet", "opus"},
			AccessLevels: []types.AccessLevel{types.AccessReadOnly, types.AccessOnRequest},
			Defaults: types.SessionRuntimeOptions{
				Model:  "sonnet",
				Access: types.AccessOnRequest,
			},
		},
	}
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	advanceGuidedWorkflowToComposerForTest(t, &m)

	m.newSession = &newSessionTarget{
		provider: "codex",
		runtimeOptions: &types.SessionRuntimeOptions{
			Model:     "gpt-5.3-codex",
			Reasoning: types.ReasoningHigh,
			Access:    types.AccessFull,
		},
	}
	m.guidedWorkflow.SetRuntimeOptions(m.newSession.runtimeOptions)
	m.guidedWorkflow.SetProvider("claude")
	m.prepareGuidedWorkflowComposerDefaults()

	runtime := m.guidedWorkflow.RuntimeOptions()
	if runtime == nil {
		t.Fatalf("expected runtime options")
	}
	if runtime.Model != "sonnet" {
		t.Fatalf("expected model fallback to selected provider default, got %q", runtime.Model)
	}
	if runtime.Access != types.AccessOnRequest {
		t.Fatalf("expected access fallback to selected provider default, got %q", runtime.Access)
	}
	if runtime.Reasoning != "" {
		t.Fatalf("expected reasoning to clear for provider without reasoning levels, got %q", runtime.Reasoning)
	}
}

func TestGuidedWorkflowProviderSwitchKeepsCompatibleSelections(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.providerOptions = map[string]*types.ProviderOptionCatalog{
		"codex": {
			Provider:        "codex",
			Models:          []string{"shared-model", "codex-only"},
			ReasoningLevels: []types.ReasoningLevel{types.ReasoningLow, types.ReasoningHigh},
			AccessLevels:    []types.AccessLevel{types.AccessOnRequest, types.AccessFull},
			Defaults: types.SessionRuntimeOptions{
				Model:     "codex-only",
				Reasoning: types.ReasoningLow,
				Access:    types.AccessOnRequest,
			},
		},
		"opencode": {
			Provider:        "opencode",
			Models:          []string{"shared-model", "opencode-only"},
			ReasoningLevels: []types.ReasoningLevel{types.ReasoningLow, types.ReasoningHigh},
			AccessLevels:    []types.AccessLevel{types.AccessOnRequest, types.AccessFull},
			Defaults: types.SessionRuntimeOptions{
				Model:     "opencode-only",
				Reasoning: types.ReasoningLow,
				Access:    types.AccessOnRequest,
			},
		},
	}
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	advanceGuidedWorkflowToComposerForTest(t, &m)

	selected := &types.SessionRuntimeOptions{
		Model:     "shared-model",
		Reasoning: types.ReasoningHigh,
		Access:    types.AccessFull,
	}
	m.newSession = &newSessionTarget{
		provider:       "codex",
		runtimeOptions: types.CloneRuntimeOptions(selected),
	}
	m.guidedWorkflow.SetRuntimeOptions(selected)
	m.guidedWorkflow.SetProvider("opencode")
	m.prepareGuidedWorkflowComposerDefaults()

	runtime := m.guidedWorkflow.RuntimeOptions()
	if runtime == nil {
		t.Fatalf("expected runtime options")
	}
	if runtime.Model != "shared-model" || runtime.Reasoning != types.ReasoningHigh || runtime.Access != types.AccessFull {
		t.Fatalf("expected compatible selections to remain unchanged, got %#v", runtime)
	}
}

func TestGuidedWorkflowSetupSelectionsDoNotSnapBackAfterProviderOptionsRefresh(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	advanceGuidedWorkflowToComposerForTest(t, &m)

	selected := &types.SessionRuntimeOptions{
		Model:     "gpt-5.3-codex",
		Reasoning: types.ReasoningHigh,
		Access:    types.AccessFull,
	}
	m.newSession = &newSessionTarget{
		provider:       "codex",
		runtimeOptions: types.CloneRuntimeOptions(selected),
	}
	m.guidedWorkflow.SetRuntimeOptions(selected)

	updated, _ := m.Update(providerOptionsMsg{
		provider: "codex",
		options: &types.ProviderOptionCatalog{
			Provider:        "codex",
			Models:          []string{"gpt-5.1-codex", "gpt-5.3-codex"},
			ReasoningLevels: []types.ReasoningLevel{types.ReasoningLow, types.ReasoningMedium, types.ReasoningHigh},
			AccessLevels:    []types.AccessLevel{types.AccessReadOnly, types.AccessOnRequest, types.AccessFull},
			Defaults: types.SessionRuntimeOptions{
				Model:     "gpt-5.1-codex",
				Reasoning: types.ReasoningMedium,
				Access:    types.AccessOnRequest,
			},
		},
	})
	m = asModel(t, updated)

	runtime := m.guidedWorkflow.RuntimeOptions()
	if runtime == nil {
		t.Fatalf("expected runtime options")
	}
	if runtime.Model != "gpt-5.3-codex" || runtime.Reasoning != types.ReasoningHigh || runtime.Access != types.AccessFull {
		t.Fatalf("expected user selections to persist after provider options refresh, got %#v", runtime)
	}
}

func TestGuidedWorkflowSetupUIRemovesRedundantMetadataSections(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	advanceGuidedWorkflowToComposerForTest(t, &m)

	if !strings.Contains(m.contentRaw, "### Launch Selections") {
		t.Fatalf("expected launch selections section, got %q", m.contentRaw)
	}
	if strings.Contains(m.contentRaw, "### Workflow Prompt") {
		t.Fatalf("expected workflow prompt metadata section removed, got %q", m.contentRaw)
	}
	if strings.Contains(m.contentRaw, "### Runtime Options") {
		t.Fatalf("expected runtime options metadata section removed, got %q", m.contentRaw)
	}
	if strings.Contains(m.contentRaw, "### Controls") {
		t.Fatalf("expected controls legend removed from setup metadata, got %q", m.contentRaw)
	}
}

func TestWorkflowDismissNotFoundFallsBackToLocalDismiss(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	meta := m.sessionMeta["s1"]
	if meta == nil {
		t.Fatalf("expected session meta fixture")
	}
	meta.WorkflowRunID = "gwf-missing"
	m.applySidebarItems()
	if !sidebarHasWorkflowItem(m, "gwf-missing") {
		t.Fatalf("expected missing workflow placeholder to be visible before dismiss")
	}

	updated, cmd := m.Update(workflowRunVisibilityMsg{
		runID:     "gwf-missing",
		err:       &client.APIError{StatusCode: 404, Message: "workflow run not found"},
		dismissed: true,
	})
	m = asModel(t, updated)
	if cmd != nil {
		if _, ok := cmd().(appStateSaveFlushMsg); !ok {
			t.Fatalf("expected app state save scheduling command, got %T", cmd())
		}
	}
	if !strings.Contains(strings.ToLower(m.status), "dismissed locally") {
		t.Fatalf("expected local dismiss status, got %q", m.status)
	}
	if sidebarHasWorkflowItem(m, "gwf-missing") {
		t.Fatalf("expected missing workflow placeholder to be hidden after local dismiss")
	}
	found := false
	for _, run := range m.workflowRuns {
		if run == nil || strings.TrimSpace(run.ID) != "gwf-missing" {
			continue
		}
		found = true
		if run.DismissedAt == nil {
			t.Fatalf("expected local fallback run to be marked dismissed")
		}
	}
	if !found {
		t.Fatalf("expected synthetic dismissed workflow run to be tracked")
	}
	if len(m.appState.DismissedMissingWorkflowRunIDs) != 1 || m.appState.DismissedMissingWorkflowRunIDs[0] != "gwf-missing" {
		t.Fatalf("expected dismissed missing workflow id persisted in app state, got %#v", m.appState.DismissedMissingWorkflowRunIDs)
	}
}

func TestWorkflowDismissNotFoundStaysHiddenAfterRunsRefresh(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	meta := m.sessionMeta["s1"]
	if meta == nil {
		t.Fatalf("expected session meta fixture")
	}
	meta.WorkflowRunID = "gwf-missing"
	m.applySidebarItems()
	if !sidebarHasWorkflowItem(m, "gwf-missing") {
		t.Fatalf("expected missing workflow placeholder to be visible before dismiss")
	}

	updated, _ := m.Update(workflowRunVisibilityMsg{
		runID:     "gwf-missing",
		err:       &client.APIError{StatusCode: 404, Message: "workflow run not found"},
		dismissed: true,
	})
	m = asModel(t, updated)
	updated, _ = m.Update(workflowRunsMsg{runs: []*guidedworkflows.WorkflowRun{}})
	m = asModel(t, updated)

	if sidebarHasWorkflowItem(m, "gwf-missing") {
		t.Fatalf("expected missing workflow placeholder to stay hidden after workflow refresh")
	}
}

func TestDismissedWorkflowFetchedFromSidebarStaysHiddenAfterRunsRefresh(t *testing.T) {
	now := time.Date(2026, 2, 21, 10, 0, 0, 0, time.UTC)
	dismissedAt := now.Add(2 * time.Minute)
	run := newWorkflowRunFixture("gwf-dismissed-fetched", guidedworkflows.WorkflowRunStatusCompleted, now)
	run.DismissedAt = &dismissedAt
	run.CompletedAt = &dismissedAt
	api := &guidedWorkflowAPIMock{
		snapshotRuns: []*guidedworkflows.WorkflowRun{cloneWorkflowRun(run)},
		snapshotTimelines: [][]guidedworkflows.RunTimelineEvent{
			{
				{At: now, Type: "run_started", RunID: run.ID},
				{At: dismissedAt, Type: "run_completed", RunID: run.ID},
			},
		},
	}

	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	if m.sessionMeta["s1"] == nil {
		t.Fatalf("expected session meta fixture")
	}
	m.sessionMeta["s1"].WorkflowRunID = run.ID
	m.applySidebarItems()
	if !sidebarHasWorkflowItem(m, run.ID) {
		t.Fatalf("expected workflow placeholder before fetch")
	}
	if !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected dismissed workflow placeholder to be selectable")
	}
	item := m.selectedItem()
	if item == nil || item.kind != sidebarWorkflow {
		t.Fatalf("expected selected workflow item")
	}
	cmd := fetchWorkflowRunSnapshotCmd(m.guidedWorkflowAPI, run.ID)
	msg := workflowRunSnapshotMsgFromCmd(t, cmd)
	updated, _ := m.Update(msg)
	m = asModel(t, updated)

	if sidebarHasWorkflowItem(m, run.ID) {
		t.Fatalf("expected dismissed workflow to be hidden after snapshot fetch")
	}
	if strings.Contains(strings.ToLower(m.status), "guided workflow completed") {
		t.Fatalf("expected dismissed workflow snapshot to suppress completion toast, got %q", m.status)
	}
	if len(m.appState.DismissedMissingWorkflowRunIDs) != 1 || m.appState.DismissedMissingWorkflowRunIDs[0] != run.ID {
		t.Fatalf("expected dismissed workflow id to persist for placeholder suppression, got %#v", m.appState.DismissedMissingWorkflowRunIDs)
	}

	updated, _ = m.Update(workflowRunsMsg{runs: []*guidedworkflows.WorkflowRun{}})
	m = asModel(t, updated)
	if sidebarHasWorkflowItem(m, run.ID) {
		t.Fatalf("expected dismissed workflow placeholder to stay hidden after workflow refresh")
	}
}

func TestWorkflowDismissNotFoundSurvivesAppStateRestore(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	meta := m.sessionMeta["s1"]
	if meta == nil {
		t.Fatalf("expected session meta fixture")
	}
	meta.WorkflowRunID = "gwf-missing"

	updated, _ := m.Update(workflowRunVisibilityMsg{
		runID:     "gwf-missing",
		err:       &client.APIError{StatusCode: 404, Message: "workflow run not found"},
		dismissed: true,
	})
	m = asModel(t, updated)
	state := m.appState

	restored := newPhase0ModelWithSession("codex")
	if restored.sessionMeta["s1"] == nil {
		t.Fatalf("expected restored session meta fixture")
	}
	restored.sessionMeta["s1"].WorkflowRunID = "gwf-missing"
	restored.applyAppState(&state)
	restored.setWorkflowRunsData(nil)
	restored.applySidebarItems()

	if sidebarHasWorkflowItem(restored, "gwf-missing") {
		t.Fatalf("expected missing workflow placeholder to remain hidden after app-state restore")
	}
}

func TestGuidedWorkflowTimelineSnapshotUpdatesArtifacts(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 30, 0, 0, time.UTC)
	m := NewModel(nil)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
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

func sidebarHasWorkflowItem(m Model, runID string) bool {
	if m.sidebar == nil {
		return false
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	for _, item := range m.sidebar.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.kind != sidebarWorkflow {
			continue
		}
		if strings.TrimSpace(entry.workflowRunID()) == runID {
			return true
		}
	}
	return false
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
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
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
	if !strings.Contains(m.contentRaw, "[user turn turn-42](archon://session/s1?turn=turn-42)") {
		t.Fatalf("expected user turn link in timeline details")
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
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
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
	if m.pendingWorkflowTurnFocus == nil {
		t.Fatalf("expected pending workflow turn focus")
	}
	if m.pendingWorkflowTurnFocus.sessionID != "s1" || m.pendingWorkflowTurnFocus.turnID != "turn-99" {
		t.Fatalf("unexpected pending workflow turn focus: %#v", m.pendingWorkflowTurnFocus)
	}

	items := []map[string]any{
		{"type": "userMessage", "turn_id": "turn-99", "content": []any{map[string]any{"type": "text", "text": "step request"}}},
		{"type": "assistant", "turn_id": "turn-99", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "agent reply"}}}},
	}
	blocks := projectSessionBlocksFromItems("codex", items, nil, nil, nil)
	m.applySessionProjection(sessionProjectionSourceHistory, "s1", "", blocks)
	if m.pendingWorkflowTurnFocus != nil {
		t.Fatalf("expected pending workflow turn focus to clear after projection")
	}
	idx := m.selectedMessageRenderIndex()
	if idx < 0 || idx >= len(m.contentBlocks) {
		t.Fatalf("expected selected workflow-linked message block, got index=%d len=%d", idx, len(m.contentBlocks))
	}
	if m.contentBlocks[idx].Role != ChatRoleUser {
		t.Fatalf("expected selected workflow turn block to be user role, got %v", m.contentBlocks[idx].Role)
	}
	if m.contentBlocks[idx].TurnID != "turn-99" {
		t.Fatalf("expected selected workflow turn id turn-99, got %q", m.contentBlocks[idx].TurnID)
	}
}

func TestGuidedWorkflowOpenSelectedStepSessionResolvesProviderAndThreadSessionIDs(t *testing.T) {
	cases := []struct {
		name              string
		executionSession  string
		providerSessionID string
		threadID          string
	}{
		{
			name:              "provider session id",
			executionSession:  "provider-session-1",
			providerSessionID: "provider-session-1",
		},
		{
			name:             "thread id",
			executionSession: "thread-1",
			threadID:         "thread-1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 2, 17, 13, 16, 0, 0, time.UTC)
			run := newWorkflowRunFixture("gwf-open-resolve", guidedworkflows.WorkflowRunStatusRunning, now)
			run.CurrentPhaseIndex = 0
			run.CurrentStepIndex = 1
			run.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
				SessionID: tc.executionSession,
				TurnID:    "turn-99",
			}
			run.Phases[0].Steps[1].ExecutionState = guidedworkflows.StepExecutionStateLinked

			m := newPhase0ModelWithSession("codex")
			m.sessionMeta["s1"] = &types.SessionMeta{
				SessionID:         "s1",
				WorkspaceID:       "ws1",
				ProviderSessionID: tc.providerSessionID,
				ThreadID:          tc.threadID,
			}
			enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
				workspaceID: "ws1",
				worktreeID:  "wt1",
			})
			updated, _ := m.Update(workflowRunSnapshotMsg{run: run})
			m = asModel(t, updated)
			updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
			m = asModel(t, updated)

			updated, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
			m = asModel(t, updated)
			if cmd == nil {
				t.Fatalf("expected session open command")
			}
			if selected := m.selectedSessionID(); selected != "s1" {
				t.Fatalf("expected linked provider/thread session to resolve to s1, got %q", selected)
			}
			if m.pendingWorkflowTurnFocus == nil {
				t.Fatalf("expected pending workflow turn focus")
			}
			if m.pendingWorkflowTurnFocus.sessionID != "s1" || m.pendingWorkflowTurnFocus.turnID != "turn-99" {
				t.Fatalf("unexpected pending workflow turn focus: %#v", m.pendingWorkflowTurnFocus)
			}
		})
	}
}

func TestGuidedWorkflowOpenSelectedStepSessionGuards(t *testing.T) {
	var nilModel *Model
	if cmd := nilModel.openGuidedWorkflowSelectedSession(); cmd != nil {
		t.Fatalf("expected nil model guard to return nil command")
	}

	m := NewModel(nil)
	m.guidedWorkflow = nil
	if cmd := m.openGuidedWorkflowSelectedSession(); cmd != nil {
		t.Fatalf("expected nil guided workflow guard to return nil command")
	}

	m.guidedWorkflow = NewGuidedWorkflowUIController()
	m.sidebar = nil
	if cmd := m.openGuidedWorkflowSelectedSession(); cmd != nil {
		t.Fatalf("expected nil sidebar guard to return nil command")
	}
}

func TestGuidedWorkflowOpenSelectedStepSessionValidatesLinkedSession(t *testing.T) {
	now := time.Date(2026, 2, 17, 13, 20, 0, 0, time.UTC)

	missingLinkRun := newWorkflowRunFixture("gwf-open-missing", guidedworkflows.WorkflowRunStatusRunning, now)
	missingLinkRun.CurrentPhaseIndex = 0
	missingLinkRun.CurrentStepIndex = 1
	missingLinkRun.Phases[0].Steps[1].Execution = nil
	missingLinkRun.Phases[0].Steps[1].TurnID = "turn-no-session"

	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1", worktreeID: "wt1"})
	m.guidedWorkflow.SetRun(missingLinkRun)
	m.guidedWorkflow.selectedPhase = 0
	m.guidedWorkflow.selectedStep = 1
	m.renderGuidedWorkflowContent()
	if cmd := m.openGuidedWorkflowSelectedSession(); cmd != nil {
		t.Fatalf("expected no command when selected step has no linked session")
	}
	if m.status != "selected step has no linked session" {
		t.Fatalf("unexpected status for missing linked session: %q", m.status)
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode to remain active on validation failure")
	}
	if m.pendingWorkflowTurnFocus != nil {
		t.Fatalf("expected pending workflow turn focus to remain nil")
	}

	missingSidebarRun := newWorkflowRunFixture("gwf-open-not-found", guidedworkflows.WorkflowRunStatusRunning, now)
	missingSidebarRun.CurrentPhaseIndex = 0
	missingSidebarRun.CurrentStepIndex = 1
	missingSidebarRun.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
		SessionID: "s-missing",
		TurnID:    "turn-404",
	}
	m.sessionSelectionAPI = &sessionListWithMetaQueryMock{
		sessionsToReturn: []*types.Session{
			{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now, Title: "Session"},
		},
		metaToReturn: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1"},
		},
	}
	m.guidedWorkflow.SetRun(missingSidebarRun)
	m.guidedWorkflow.selectedPhase = 0
	m.guidedWorkflow.selectedStep = 1
	m.renderGuidedWorkflowContent()
	cmd := m.openGuidedWorkflowSelectedSession()
	if cmd == nil {
		t.Fatalf("expected refresh command when linked session is not yet selectable")
	}
	refreshMsg, ok := cmd().(sessionsWithMetaMsg)
	if !ok {
		t.Fatalf("expected sessionsWithMetaMsg refresh, got %T", cmd())
	}
	updated, _ := m.Update(refreshMsg)
	m = asModel(t, updated)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode to remain active when session lookup fails")
	}
	if selected := m.selectedSessionID(); selected != "s1" {
		t.Fatalf("expected existing sidebar selection to remain unchanged, got %q", selected)
	}
	if m.pendingGuidedWorkflowSessionLookup != nil {
		t.Fatalf("expected pending linked session lookup to be cleared after failed refresh")
	}
	if m.pendingWorkflowTurnFocus != nil {
		t.Fatalf("expected pending workflow turn focus to remain nil when session lookup fails")
	}
}

func TestGuidedWorkflowOpenSelectedStepSessionExpandsUngroupedFilter(t *testing.T) {
	now := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"g1"}
	if m.menu != nil {
		m.menu.SetSelectedGroupIDs([]string{"g1"})
	}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace 1", GroupIDs: []string{"g1"}},
	}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now, Title: "Anchor"},
		{ID: "s-linked", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now.Add(time.Second), Title: "Linked"},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s-linked": {
			SessionID: "s-linked",
			// No workspace/worktree context yet; this should be treated as ungrouped.
		},
	}
	m.applySidebarItems()
	if m.sidebar.SelectBySessionID("s-linked") {
		t.Fatalf("expected linked session to be hidden while ungrouped is filtered out")
	}

	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	run := newWorkflowRunFixture("gwf-open-ungrouped", guidedworkflows.WorkflowRunStatusRunning, now)
	run.CurrentPhaseIndex = 0
	run.CurrentStepIndex = 1
	run.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
		SessionID: "s-linked",
		TurnID:    "turn-ungrouped",
	}
	m.guidedWorkflow.SetRun(run)
	m.guidedWorkflow.selectedPhase = 0
	m.guidedWorkflow.selectedStep = 1
	m.renderGuidedWorkflowContent()

	cmd := m.openGuidedWorkflowSelectedSession()
	if cmd == nil {
		t.Fatalf("expected linked session open command")
	}
	if selected := m.selectedSessionID(); selected != "s-linked" {
		t.Fatalf("expected linked session selection after group expansion, got %q", selected)
	}
	if m.pendingWorkflowTurnFocus == nil || m.pendingWorkflowTurnFocus.sessionID != "s-linked" || m.pendingWorkflowTurnFocus.turnID != "turn-ungrouped" {
		t.Fatalf("expected pending turn focus for linked session, got %#v", m.pendingWorkflowTurnFocus)
	}
	if !containsString(m.appState.ActiveWorkspaceGroupIDs, "ungrouped") {
		t.Fatalf("expected ungrouped group to be activated, got %#v", m.appState.ActiveWorkspaceGroupIDs)
	}
	if m.menu != nil && !containsString(m.menu.SelectedGroupIDs(), "ungrouped") {
		t.Fatalf("expected menu to include ungrouped selection, got %#v", m.menu.SelectedGroupIDs())
	}
}

func TestGuidedWorkflowOpenSelectedStepSessionExpandsWorkspaceGroupFilter(t *testing.T) {
	now := time.Date(2026, 2, 22, 10, 5, 0, 0, time.UTC)
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"g1"}
	if m.menu != nil {
		m.menu.SetSelectedGroupIDs([]string{"g1"})
	}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace 1", GroupIDs: []string{"g1"}},
		{ID: "ws2", Name: "Workspace 2", GroupIDs: []string{"g2"}},
	}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now, Title: "Anchor"},
		{ID: "s-linked", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now.Add(time.Second), Title: "Linked"},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1":       {SessionID: "s1", WorkspaceID: "ws1"},
		"s-linked": {SessionID: "s-linked", WorkspaceID: "ws2"},
	}
	m.applySidebarItems()
	if m.sidebar.SelectBySessionID("s-linked") {
		t.Fatalf("expected linked session to be hidden while g2 is filtered out")
	}

	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	run := newWorkflowRunFixture("gwf-open-group", guidedworkflows.WorkflowRunStatusRunning, now)
	run.CurrentPhaseIndex = 0
	run.CurrentStepIndex = 1
	run.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
		SessionID: "s-linked",
		TurnID:    "turn-group",
	}
	m.guidedWorkflow.SetRun(run)
	m.guidedWorkflow.selectedPhase = 0
	m.guidedWorkflow.selectedStep = 1
	m.renderGuidedWorkflowContent()

	cmd := m.openGuidedWorkflowSelectedSession()
	if cmd == nil {
		t.Fatalf("expected linked session open command")
	}
	if selected := m.selectedSessionID(); selected != "s-linked" {
		t.Fatalf("expected linked session selection after group expansion, got %q", selected)
	}
	if !containsString(m.appState.ActiveWorkspaceGroupIDs, "g2") {
		t.Fatalf("expected g2 group to be activated, got %#v", m.appState.ActiveWorkspaceGroupIDs)
	}
	if m.menu != nil && !containsString(m.menu.SelectedGroupIDs(), "g2") {
		t.Fatalf("expected menu to include g2 selection, got %#v", m.menu.SelectedGroupIDs())
	}
}

func TestGuidedWorkflowOpenSelectedStepSessionRefreshesWhenSessionMissingLocally(t *testing.T) {
	now := time.Date(2026, 2, 22, 11, 0, 0, 0, time.UTC)
	m := newPhase0ModelWithSession("codex")
	m.sessionSelectionAPI = &sessionListWithMetaQueryMock{
		sessionsToReturn: []*types.Session{
			{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now, Title: "Anchor"},
			{ID: "s-linked", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now.Add(time.Second), Title: "Linked"},
		},
		metaToReturn: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1"},
			{SessionID: "s-linked", WorkspaceID: "ws1"},
		},
	}

	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	run := newWorkflowRunFixture("gwf-open-refresh", guidedworkflows.WorkflowRunStatusRunning, now)
	run.CurrentPhaseIndex = 0
	run.CurrentStepIndex = 1
	run.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
		SessionID: "s-linked",
		TurnID:    "turn-refresh",
	}
	m.guidedWorkflow.SetRun(run)
	m.guidedWorkflow.selectedPhase = 0
	m.guidedWorkflow.selectedStep = 1
	m.renderGuidedWorkflowContent()

	cmd := m.openGuidedWorkflowSelectedSession()
	if cmd == nil {
		t.Fatalf("expected sessions refresh command when linked session is not yet local")
	}
	refreshMsg, ok := cmd().(sessionsWithMetaMsg)
	if !ok {
		t.Fatalf("expected sessionsWithMetaMsg refresh, got %T", cmd())
	}

	updated, nextCmd := m.Update(refreshMsg)
	m = asModel(t, updated)
	if nextCmd == nil {
		t.Fatalf("expected follow-up selection load command")
	}
	if selected := m.selectedSessionID(); selected != "s-linked" {
		t.Fatalf("expected linked session selection after refresh, got %q", selected)
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected guided workflow to close after selecting linked session, got %v", m.mode)
	}
	if m.pendingGuidedWorkflowSessionLookup != nil {
		t.Fatalf("expected pending linked session lookup to clear")
	}
	if m.pendingWorkflowTurnFocus == nil || m.pendingWorkflowTurnFocus.sessionID != "s-linked" || m.pendingWorkflowTurnFocus.turnID != "turn-refresh" {
		t.Fatalf("expected pending turn focus to target linked session after refresh, got %#v", m.pendingWorkflowTurnFocus)
	}
}

func TestGuidedWorkflowOpenSelectedStepSessionReResolvesProviderIDAfterRefresh(t *testing.T) {
	now := time.Date(2026, 2, 22, 11, 5, 0, 0, time.UTC)
	const providerSessionID = "provider-session-42"
	m := newPhase0ModelWithSession("codex")
	m.sessionSelectionAPI = &sessionListWithMetaQueryMock{
		sessionsToReturn: []*types.Session{
			{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now, Title: "Anchor"},
			{ID: "s-linked", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now.Add(time.Second), Title: "Linked"},
		},
		metaToReturn: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1"},
			{SessionID: "s-linked", WorkspaceID: "ws1", ProviderSessionID: providerSessionID},
		},
	}

	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	run := newWorkflowRunFixture("gwf-open-provider-refresh", guidedworkflows.WorkflowRunStatusRunning, now)
	run.CurrentPhaseIndex = 0
	run.CurrentStepIndex = 1
	run.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
		SessionID: providerSessionID,
		TurnID:    "turn-provider-refresh",
	}
	m.guidedWorkflow.SetRun(run)
	m.guidedWorkflow.selectedPhase = 0
	m.guidedWorkflow.selectedStep = 1
	m.renderGuidedWorkflowContent()

	cmd := m.openGuidedWorkflowSelectedSession()
	if cmd == nil {
		t.Fatalf("expected sessions refresh command when provider session id has not been mapped yet")
	}
	refreshMsg, ok := cmd().(sessionsWithMetaMsg)
	if !ok {
		t.Fatalf("expected sessionsWithMetaMsg refresh, got %T", cmd())
	}

	updated, nextCmd := m.Update(refreshMsg)
	m = asModel(t, updated)
	if nextCmd == nil {
		t.Fatalf("expected follow-up selection load command")
	}
	if selected := m.selectedSessionID(); selected != "s-linked" {
		t.Fatalf("expected provider id to resolve to local linked session after refresh, got %q", selected)
	}
	if m.pendingWorkflowTurnFocus == nil || m.pendingWorkflowTurnFocus.sessionID != "s-linked" || m.pendingWorkflowTurnFocus.turnID != "turn-provider-refresh" {
		t.Fatalf("expected pending turn focus to use resolved local session id, got %#v", m.pendingWorkflowTurnFocus)
	}
	if m.pendingGuidedWorkflowSessionLookup != nil {
		t.Fatalf("expected pending provider-id lookup to clear after refresh")
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
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1", worktreeID: "wt1"})
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
	snapshotMsg := workflowRunSnapshotMsgFromCmd(t, cmd)
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

func TestGuidedWorkflowFailedSummaryPrimesDefaultResumeMessage(t *testing.T) {
	now := time.Date(2026, 2, 22, 14, 0, 0, 0, time.UTC)
	failed := newWorkflowRunFixture("gwf-failed-default-resume", guidedworkflows.WorkflowRunStatusFailed, now)
	failed.UserPrompt = "Fix parser bug in workflow summaries"
	failed.LastError = "workflow run interrupted by daemon restart"
	completedAt := now.Add(3 * time.Second)
	failed.CompletedAt = &completedAt

	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1", worktreeID: "wt1", sessionID: "s1"})
	updated, _ := m.Update(workflowRunSnapshotMsg{run: failed})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSummary {
		t.Fatalf("expected failed run to enter summary stage")
	}
	if m.guidedWorkflowResumeInput == nil {
		t.Fatalf("expected resume input to be initialized")
	}
	if got := strings.TrimSpace(m.guidedWorkflowResumeInput.Value()); got != guidedworkflows.DefaultResumeFailedRunMessage {
		t.Fatalf("expected default resume message, got %q", got)
	}
	if !strings.Contains(m.contentRaw, "Resume Failed Run") {
		t.Fatalf("expected failed summary to render resume section, got %q", m.contentRaw)
	}
	if !strings.Contains(m.contentRaw, "### Original Prompt") || !strings.Contains(m.contentRaw, "> "+failed.UserPrompt) {
		t.Fatalf("expected original prompt in summary content, got %q", m.contentRaw)
	}
}

func TestGuidedWorkflowFailedSummaryResumeUsesEditedMessage(t *testing.T) {
	now := time.Date(2026, 2, 22, 14, 5, 0, 0, time.UTC)
	failed := newWorkflowRunFixture("gwf-failed-resume", guidedworkflows.WorkflowRunStatusFailed, now)
	failed.LastError = "workflow run interrupted by daemon restart"
	completedAt := now.Add(4 * time.Second)
	failed.CompletedAt = &completedAt
	running := newWorkflowRunFixture("gwf-failed-resume", guidedworkflows.WorkflowRunStatusRunning, now.Add(10*time.Second))
	api := &guidedWorkflowAPIMock{
		resumeRun: running,
		snapshotRuns: []*guidedworkflows.WorkflowRun{
			running,
		},
		snapshotTimelines: [][]guidedworkflows.RunTimelineEvent{
			{
				{At: now, Type: "run_resumed_after_failure", RunID: "gwf-failed-resume"},
			},
		},
	}

	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1", worktreeID: "wt1", sessionID: "s1"})
	updated, _ := m.Update(workflowRunSnapshotMsg{run: failed})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || !m.guidedWorkflow.CanResumeFailedRun() {
		t.Fatalf("expected failed summary to allow resume")
	}
	if m.guidedWorkflowResumeInput == nil || !m.guidedWorkflowResumeInput.Focused() {
		t.Fatalf("expected resume input to be focused")
	}
	m.guidedWorkflowResumeInput.SetValue("Please continue the interrupted run and report status.")
	m.syncGuidedWorkflowResumeInput()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected failed resume command")
	}
	resumeMsg, ok := cmd().(workflowRunResumedMsg)
	if !ok {
		t.Fatalf("expected workflowRunResumedMsg, got %T", cmd())
	}
	updated, cmd = m.Update(resumeMsg)
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected snapshot refresh command after failed resume")
	}
	snapshotMsg := workflowRunSnapshotMsgFromCmd(t, cmd)
	updated, _ = m.Update(snapshotMsg)
	m = asModel(t, updated)

	if len(api.resumeReqs) != 1 {
		t.Fatalf("expected one resume request, got %d", len(api.resumeReqs))
	}
	if !api.resumeReqs[0].ResumeFailed {
		t.Fatalf("expected resume_failed=true in resume request")
	}
	if api.resumeReqs[0].Message != "Please continue the interrupted run and report status." {
		t.Fatalf("unexpected resume message payload: %q", api.resumeReqs[0].Message)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageLive {
		t.Fatalf("expected guided workflow to return to live stage after resume")
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
	_ = m.onSelectionChangedImmediate()
	if m.mode == uiModeGuidedWorkflow {
		t.Fatalf("expected workflow selection to remain passive until explicit open")
	}
	handled, cmd := m.activateSelectionFromSidebar()
	if !handled {
		t.Fatalf("expected workflow selection activation to open selected workflow")
	}
	msg := workflowRunSnapshotMsgFromCmd(t, cmd)
	updated, _ := m.Update(msg)
	m = asModel(t, updated)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.RunID() != run.ID {
		t.Fatalf("expected guided workflow run %q, got %#v", run.ID, m.guidedWorkflow)
	}
}

func TestGuidedWorkflowModeDismissHotkeyTargetsSelectedWorkflow(t *testing.T) {
	now := time.Date(2026, 2, 17, 13, 32, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-dismiss-hotkey", guidedworkflows.WorkflowRunStatusRunning, now)
	api := &guidedWorkflowAPIMock{
		snapshotRuns: []*guidedworkflows.WorkflowRun{run},
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

	if !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected workflow row to be selectable")
	}
	_ = m.onSelectionChangedImmediate()
	handled, cmd := m.activateSelectionFromSidebar()
	if !handled {
		t.Fatalf("expected workflow selection activation to open selected workflow")
	}
	msg := workflowRunSnapshotMsgFromCmd(t, cmd)
	updated, _ := m.Update(msg)
	m = asModel(t, updated)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected no async command before confirm choice")
	}
	action, ok := m.pendingSelectionAction.(dismissWorkflowSelectionAction)
	if !ok {
		t.Fatalf("expected workflow dismiss selection action, got %T", m.pendingSelectionAction)
	}
	if action.runID != run.ID {
		t.Fatalf("expected workflow run id %q, got %q", run.ID, action.runID)
	}
}

func TestSelectingCompletedWorkflowDoesNotRepeatCompletedToast(t *testing.T) {
	now := time.Date(2026, 2, 21, 8, 0, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-completed-sidebar", guidedworkflows.WorkflowRunStatusCompleted, now)
	completedAt := now.Add(2 * time.Minute)
	run.CompletedAt = &completedAt
	api := &guidedWorkflowAPIMock{
		snapshotRuns: []*guidedworkflows.WorkflowRun{cloneWorkflowRun(run)},
		snapshotTimelines: [][]guidedworkflows.RunTimelineEvent{
			{
				{At: now, Type: "run_started", RunID: run.ID},
				{At: completedAt, Type: "run_completed", RunID: run.ID},
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
	if !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected completed workflow row to be selectable")
	}
	_ = m.onSelectionChangedImmediate()
	handled, cmd := m.activateSelectionFromSidebar()
	if !handled {
		t.Fatalf("expected workflow selection activation to open selected workflow")
	}
	msg := workflowRunSnapshotMsgFromCmd(t, cmd)
	updated, _ := m.Update(msg)
	m = asModel(t, updated)
	if strings.Contains(strings.ToLower(m.status), "guided workflow completed") {
		t.Fatalf("expected completed snapshot to avoid duplicate completion toast, got %q", m.status)
	}
}

func TestWorkflowRunSnapshotPreservesExistingWorkflowContext(t *testing.T) {
	now := time.Date(2026, 2, 21, 8, 15, 0, 0, time.UTC)
	existing := newWorkflowRunFixture("gwf-context-preserve", guidedworkflows.WorkflowRunStatusRunning, now)
	existing.WorkspaceID = "ws1"
	existing.WorktreeID = "wt-missing"
	existing.SessionID = "s1"
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"g1"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace", GroupIDs: []string{"g1"}},
	}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {},
	}
	m.workflowRuns = []*guidedworkflows.WorkflowRun{existing}
	m.applySidebarItems()
	if !m.sidebar.SelectByWorkflowID(existing.ID) {
		t.Fatalf("expected workflow row before snapshot update")
	}
	snapshot := &guidedworkflows.WorkflowRun{
		ID:        existing.ID,
		Status:    guidedworkflows.WorkflowRunStatusCompleted,
		CreatedAt: existing.CreatedAt,
		// Intentionally missing workspace/worktree/session to simulate sparse snapshot payloads.
	}
	updated, _ := m.Update(workflowRunSnapshotMsg{
		run: snapshot,
		timeline: []guidedworkflows.RunTimelineEvent{
			{At: now.Add(time.Minute), Type: "run_completed", RunID: existing.ID},
		},
	})
	m = asModel(t, updated)
	var stored *guidedworkflows.WorkflowRun
	for _, run := range m.workflowRuns {
		if run != nil && strings.TrimSpace(run.ID) == existing.ID {
			stored = run
			break
		}
	}
	if stored == nil {
		t.Fatalf("expected workflow run to remain stored")
	}
	if stored.WorkspaceID != "ws1" || stored.WorktreeID != "wt-missing" || stored.SessionID != "s1" {
		t.Fatalf("expected workflow context to be preserved, got workspace=%q worktree=%q session=%q", stored.WorkspaceID, stored.WorktreeID, stored.SessionID)
	}
	if !m.sidebar.SelectByWorkflowID(existing.ID) {
		t.Fatalf("expected workflow row to remain visible after sparse snapshot update")
	}
}

func TestWorkflowRunsRefreshPreservesExistingWorkflowContext(t *testing.T) {
	now := time.Date(2026, 2, 21, 9, 0, 0, 0, time.UTC)
	existing := newWorkflowRunFixture("gwf-list-context", guidedworkflows.WorkflowRunStatusRunning, now)
	existing.WorkspaceID = "ws1"
	existing.WorktreeID = "wt-missing"
	existing.SessionID = "s1"
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"g1"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace", GroupIDs: []string{"g1"}},
	}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {},
	}
	m.workflowRuns = []*guidedworkflows.WorkflowRun{existing}
	m.applySidebarItems()
	if !m.sidebar.SelectByWorkflowID(existing.ID) {
		t.Fatalf("expected workflow row before workflow list refresh")
	}

	updated, _ := m.Update(workflowRunsMsg{
		runs: []*guidedworkflows.WorkflowRun{
			{
				ID:        existing.ID,
				Status:    guidedworkflows.WorkflowRunStatusCompleted,
				CreatedAt: existing.CreatedAt,
				// Sparse list payload: missing workspace/worktree/session should not drop context.
			},
		},
	})
	m = asModel(t, updated)
	var stored *guidedworkflows.WorkflowRun
	for _, run := range m.workflowRuns {
		if run != nil && strings.TrimSpace(run.ID) == existing.ID {
			stored = run
			break
		}
	}
	if stored == nil {
		t.Fatalf("expected workflow run to remain stored")
	}
	if stored.WorkspaceID != "ws1" || stored.WorktreeID != "wt-missing" || stored.SessionID != "s1" {
		t.Fatalf("expected workflow context to be preserved from existing run, got workspace=%q worktree=%q session=%q", stored.WorkspaceID, stored.WorktreeID, stored.SessionID)
	}
	if !m.sidebar.SelectByWorkflowID(existing.ID) {
		t.Fatalf("expected workflow row to remain visible after sparse list refresh")
	}
}

func TestWorkflowRunsRefreshKeepsSelectedWorkflowWhenListTransientlyDrops(t *testing.T) {
	now := time.Date(2026, 2, 21, 9, 5, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-refresh-sticky", guidedworkflows.WorkflowRunStatusCompleted, now)
	m := newPhase0ModelWithSession("codex")
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}
	m.sessionMeta["s1"] = &types.SessionMeta{
		SessionID:     "s1",
		WorkspaceID:   "ws1",
		WorkflowRunID: run.ID,
	}
	m.applySidebarItems()
	if !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected workflow to be selected")
	}

	updated, _ := m.Update(workflowRunsMsg{runs: []*guidedworkflows.WorkflowRun{}})
	m = asModel(t, updated)
	if !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected selected workflow to survive transient empty workflow list")
	}
}

func TestWorkflowRunSnapshotUsesStatusIndexToAvoidDuplicateCompletedToast(t *testing.T) {
	now := time.Date(2026, 2, 21, 9, 10, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-status-index", guidedworkflows.WorkflowRunStatusCompleted, now)
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	m.recordWorkflowRunState(run)
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	m.guidedWorkflow.SetRun(cloneWorkflowRun(run))
	m.status = ""

	updated, _ := m.Update(workflowRunSnapshotMsg{
		run: cloneWorkflowRun(run),
		timeline: []guidedworkflows.RunTimelineEvent{
			{At: now, Type: "run_completed", RunID: run.ID},
		},
	})
	m = asModel(t, updated)
	if strings.Contains(strings.ToLower(m.status), "guided workflow completed") {
		t.Fatalf("expected status index to suppress duplicate completion toast, got %q", m.status)
	}
}

func TestGuidedWorkflowModeDismissHotkeyUsesActiveRunWhenSessionSelected(t *testing.T) {
	now := time.Date(2026, 2, 17, 13, 40, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-dismiss-active", guidedworkflows.WorkflowRunStatusRunning, now)
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

	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{workspaceID: "ws1"})
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	m.guidedWorkflow.SetRun(run)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected no async command before confirm choice")
	}
	action, ok := m.pendingSelectionAction.(dismissWorkflowSelectionAction)
	if !ok {
		t.Fatalf("expected workflow dismiss selection action, got %T", m.pendingSelectionAction)
	}
	if action.runID != run.ID {
		t.Fatalf("expected workflow run id %q, got %q", run.ID, action.runID)
	}
}

func TestGuidedWorkflowLiveStopHotkeyStopsRun(t *testing.T) {
	now := time.Date(2026, 2, 22, 9, 0, 0, 0, time.UTC)
	running := newWorkflowRunFixture("gwf-stop-hotkey", guidedworkflows.WorkflowRunStatusRunning, now)
	stopped := cloneWorkflowRun(running)
	stopped.Status = guidedworkflows.WorkflowRunStatusStopped
	completedAt := now.Add(15 * time.Second)
	stopped.CompletedAt = &completedAt
	stopped.LastError = "workflow run stopped by user"
	api := &guidedWorkflowAPIMock{
		stopRun: stopped,
		snapshotRuns: []*guidedworkflows.WorkflowRun{
			cloneWorkflowRun(stopped),
		},
		snapshotTimelines: [][]guidedworkflows.RunTimelineEvent{
			{
				{At: now, Type: "run_started", RunID: running.ID},
				{At: completedAt, Type: "run_stopped", RunID: running.ID, Message: "workflow run stopped by user"},
			},
		},
	}
	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	m.guidedWorkflow.SetRun(cloneWorkflowRun(running))
	m.renderGuidedWorkflowContent()
	if m.guidedWorkflow.Stage() != guidedWorkflowStageLive {
		t.Fatalf("expected live stage before stop, got %v", m.guidedWorkflow.Stage())
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected stop command from guided workflow hotkey")
	}
	stopMsg, ok := cmd().(workflowRunStoppedMsg)
	if !ok {
		t.Fatalf("expected workflowRunStoppedMsg, got %T", cmd())
	}
	updated, cmd = m.Update(stopMsg)
	m = asModel(t, updated)
	if m.status != "guided workflow stopped" {
		t.Fatalf("expected guided workflow stopped status, got %q", m.status)
	}
	if len(api.stopRunIDs) != 1 || api.stopRunIDs[0] != running.ID {
		t.Fatalf("expected one stop request for run %q, got %#v", running.ID, api.stopRunIDs)
	}
	if cmd == nil {
		t.Fatalf("expected snapshot refresh command after stop")
	}
	snapshotMsg := workflowRunSnapshotMsgFromCmd(t, cmd)
	updated, _ = m.Update(snapshotMsg)
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageLive {
		t.Fatalf("expected guided workflow to remain in live stage after stop snapshot")
	}
	if got := strings.TrimSpace(m.guidedWorkflow.RunID()); got != running.ID {
		t.Fatalf("expected run id %q after stop snapshot, got %q", running.ID, got)
	}
}

func TestStopGuidedWorkflowRunRequiresRunID(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	m.guidedWorkflow.SetRun(nil)

	cmd := m.stopGuidedWorkflowRun()
	if cmd != nil {
		t.Fatalf("expected no stop command when run id is missing")
	}
	if m.status != "guided run id is missing" {
		t.Fatalf("expected missing run id validation status, got %q", m.status)
	}
}

func TestConfirmDismissWorkflowReturnsVisibilityCommand(t *testing.T) {
	now := time.Date(2026, 2, 17, 14, 0, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-confirm-dismiss", guidedworkflows.WorkflowRunStatusRunning, now)
	m := NewModel(nil)
	m.guidedWorkflowAPI = &guidedWorkflowAPIMock{
		startRun: run,
	}

	m.confirmDismissWorkflow(run.ID)
	if m.confirm == nil || !m.confirm.IsOpen() {
		t.Fatalf("expected confirm modal to be open")
	}
	cmd := m.handleConfirmChoice(confirmChoiceConfirm)
	if cmd == nil {
		t.Fatalf("expected dismiss command")
	}
	msg, ok := cmd().(workflowRunVisibilityMsg)
	if !ok {
		t.Fatalf("expected workflowRunVisibilityMsg, got %T", cmd())
	}
	if !msg.dismissed {
		t.Fatalf("expected dismissed visibility message")
	}
	if msg.runID != run.ID {
		t.Fatalf("expected run id %q, got %q", run.ID, msg.runID)
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

func TestWorkflowSnapshotDoesNotReclaimFocusAfterSelectingChildSession(t *testing.T) {
	now := time.Date(2026, 2, 17, 13, 37, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-focus-sticky", guidedworkflows.WorkflowRunStatusRunning, now)
	m := newPhase0ModelWithSession("codex")
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}
	m.sessionMeta["s1"] = &types.SessionMeta{
		SessionID:     "s1",
		WorkspaceID:   "ws1",
		WorkflowRunID: run.ID,
	}
	m.applySidebarItems()

	if !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected workflow row to be selectable")
	}
	_ = m.onSelectionChangedImmediate()
	handled, cmd := m.activateSelectionFromSidebar()
	if !handled || cmd == nil {
		t.Fatalf("expected workflow activation command")
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode before switching to session")
	}

	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected workflow child session row to be selectable")
	}
	if cmd := m.onSelectionChangedImmediate(); cmd == nil {
		t.Fatalf("expected session load command")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected normal mode after selecting child session, got %v", m.mode)
	}
	if got := m.selectedSessionID(); got != "s1" {
		t.Fatalf("expected selected session s1 before snapshot, got %q", got)
	}

	updated, _ := m.Update(workflowRunSnapshotMsg{
		run: cloneWorkflowRun(run),
		timeline: []guidedworkflows.RunTimelineEvent{
			{At: now, Type: "run_started", RunID: run.ID},
			{At: now.Add(2 * time.Second), Type: "step_completed", RunID: run.ID, Message: "phase plan complete"},
		},
	})
	m = asModel(t, updated)
	if m.mode != uiModeNormal {
		t.Fatalf("expected workflow snapshot to keep normal mode, got %v", m.mode)
	}
	if got := m.selectedSessionID(); got != "s1" {
		t.Fatalf("expected selected session s1 after snapshot, got %q", got)
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
					{
						ID:     "phase_plan",
						Name:   "phase plan",
						Status: guidedworkflows.StepRunStatusCompleted,
						Execution: &guidedworkflows.StepExecutionRef{
							SessionID: "s1",
							TurnID:    "turn-1",
						},
					},
					{
						ID:     "implementation",
						Name:   "implementation",
						Status: guidedworkflows.StepRunStatusCompleted,
						Execution: &guidedworkflows.StepExecutionRef{
							SessionID: "s1",
							TurnID:    "turn-2",
						},
					},
				},
			},
		},
	})
	content := controller.Render()
	promptIdx := strings.Index(content, "### Original Prompt")
	outcomeIdx := strings.Index(content, "### Outcome")
	if promptIdx < 0 || outcomeIdx < 0 || promptIdx > outcomeIdx {
		t.Fatalf("expected original prompt section ahead of outcome in summary, got %q", content)
	}
	if !strings.Contains(content, "- Final status: completed  \n- Completed steps: 2/2") {
		t.Fatalf("expected summary fields to render on separate lines, got %q", content)
	}
	if !strings.Contains(content, "- Completed steps: 2/2  \n- Decisions requested: 0") {
		t.Fatalf("expected decision count on a separate line, got %q", content)
	}
	if !strings.Contains(content, "### Step Links") {
		t.Fatalf("expected step links section in summary, got %q", content)
	}
	if !strings.Contains(content, "- Phase Delivery / phase plan: user turn turn-1") {
		t.Fatalf("expected first step turn label in summary")
	}
	if !strings.Contains(content, "- Phase Delivery / implementation: user turn turn-2") {
		t.Fatalf("expected second step turn label in summary")
	}
	if strings.Contains(content, "archon://session/") {
		t.Fatalf("expected summary step links to omit raw deep-link urls, got %q", content)
	}
}

func TestGuidedWorkflowLauncherTemplateLoadErrorAndRetry(t *testing.T) {
	api := &guidedWorkflowAPIMock{
		listTemplatesErr: errors.New("template backend unavailable"),
	}
	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	m.guidedWorkflowTemplateAPI = api
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	updated, _ := m.Update(fetchWorkflowTemplatesCmd(m.guidedWorkflowTemplateAPI)())
	m = asModel(t, updated)
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	if m.guidedWorkflow.TemplateLoadError() == "" {
		t.Fatalf("expected template load error")
	}
	if !strings.Contains(m.contentRaw, "Template load failed") {
		t.Fatalf("expected launcher error content, got %q", m.contentRaw)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected no command while template load has failed")
	}
	if !strings.Contains(strings.ToLower(m.status), "failed to load") {
		t.Fatalf("expected load-failed status, got %q", m.status)
	}

	api.listTemplatesErr = nil
	api.listTemplates = []guidedworkflows.WorkflowTemplate{
		{ID: guidedworkflows.TemplateIDSolidPhaseDelivery, Name: "SOLID Phase Delivery"},
	}

	updated, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = asModel(t, updated)
	if cmd == nil {
		t.Fatalf("expected template refresh command")
	}
	msg, ok := cmd().(workflowTemplatesMsg)
	if !ok {
		t.Fatalf("expected workflowTemplatesMsg, got %T", cmd())
	}

	updated, _ = m.Update(msg)
	m = asModel(t, updated)
	if m.guidedWorkflow.TemplateLoadError() != "" {
		t.Fatalf("expected template load error to clear after retry")
	}
	if !m.guidedWorkflow.HasTemplateSelection() {
		t.Fatalf("expected template selection after successful retry")
	}
}

func TestGuidedWorkflowLauncherBlocksSetupWhenNoTemplatesAvailable(t *testing.T) {
	api := &guidedWorkflowAPIMock{}
	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	m.guidedWorkflowTemplateAPI = api
	m.enterGuidedWorkflow(guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	updated, _ := m.Update(fetchWorkflowTemplatesCmd(m.guidedWorkflowTemplateAPI)())
	m = asModel(t, updated)
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	if m.guidedWorkflow.HasTemplateSelection() {
		t.Fatalf("expected no template selection when list is empty")
	}
	if !strings.Contains(strings.ToLower(m.status), "no workflow templates available") {
		t.Fatalf("expected no-template status, got %q", m.status)
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected no command without a selected template")
	}
	if m.guidedWorkflow.Stage() != guidedWorkflowStageLauncher {
		t.Fatalf("expected launcher stage when templates are unavailable")
	}
	if !strings.Contains(strings.ToLower(m.status), "select a workflow template") {
		t.Fatalf("expected select-template status, got %q", m.status)
	}
}

func TestGuidedWorkflowSetupEscReturnsToLauncher(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})

	advanceGuidedWorkflowToComposerForTest(t, &m)
	if m.guidedWorkflowPromptInput == nil || !m.guidedWorkflowPromptInput.Focused() {
		t.Fatalf("expected focused setup prompt input")
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = asModel(t, updated)
	if cmd != nil {
		t.Fatalf("expected no command when returning to launcher")
	}
	if m.guidedWorkflow.Stage() != guidedWorkflowStagePolicy {
		t.Fatalf("expected policy stage after esc")
	}
	if m.guidedWorkflowPromptInput.Focused() {
		t.Fatalf("expected setup prompt input to blur when returning to launcher")
	}
	if !strings.Contains(strings.ToLower(m.status), "policy sensitivity") {
		t.Fatalf("expected policy status, got %q", m.status)
	}
}

func TestGuidedWorkflowControllerTemplateAndRefreshGuards(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{workspaceID: "ws1"})

	controller.SetTemplateLoadError(errors.New("no templates"))
	if controller.TemplateLoadError() == "" {
		t.Fatalf("expected template load error text")
	}
	if controller.TemplatesLoading() {
		t.Fatalf("expected loading=false after template load error")
	}
	if controller.HasTemplateSelection() {
		t.Fatalf("expected no template selection before templates are set")
	}

	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: guidedworkflows.TemplateIDSolidPhaseDelivery, Name: "SOLID Phase Delivery"},
	})
	if !controller.HasTemplateSelection() {
		t.Fatalf("expected template selection after templates are set")
	}
	if !controller.OpenProvider() {
		t.Fatalf("expected launcher to open provider when template is selected")
	}
	if !controller.OpenPolicy() {
		t.Fatalf("expected provider to open policy when provider is selected")
	}
	if !controller.OpenSetup() {
		t.Fatalf("expected policy to open setup")
	}
	controller.OpenLauncher()
	if controller.Stage() != guidedWorkflowStageLauncher {
		t.Fatalf("expected launcher stage after OpenLauncher")
	}

	now := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	controller.SetRun(newWorkflowRunFixture("gwf-refresh", guidedworkflows.WorkflowRunStatusRunning, now))
	if !controller.CanRefresh(now, time.Second) {
		t.Fatalf("expected refresh to be allowed for active run")
	}
	controller.MarkRefreshQueued(now)
	if controller.CanRefresh(now, time.Second) {
		t.Fatalf("expected queued refresh to block additional refreshes")
	}

	// Exercise interval and terminal-state guards.
	controller.refreshQueued = false
	controller.lastRefreshAt = now
	if controller.CanRefresh(now, 2*time.Second) {
		t.Fatalf("expected refresh interval guard to block immediate refresh")
	}
	controller.lastRefreshAt = now.Add(-3 * time.Second)
	if !controller.CanRefresh(now, 2*time.Second) {
		t.Fatalf("expected refresh interval guard to allow elapsed interval")
	}
	controller.SetRun(newWorkflowRunFixture("gwf-refresh", guidedworkflows.WorkflowRunStatusCompleted, now))
	if controller.CanRefresh(now, time.Second) {
		t.Fatalf("expected completed run to disable refresh")
	}
}

func TestGuidedWorkflowProviderStageUsesDirectPickerBodyView(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	if !m.guidedWorkflow.OpenProvider() {
		t.Fatalf("expected provider stage to open")
	}
	m.renderGuidedWorkflowContent()

	header, body := m.modeViewContent()
	if header != "Guided Workflow" {
		t.Fatalf("expected guided workflow header, got %q", header)
	}
	expected := strings.TrimRight(m.guidedWorkflow.Render(), "\n")
	if body != expected {
		t.Fatalf("expected direct provider picker body view, got %q", body)
	}
}

func TestGuidedWorkflowRefreshNowValidatesRunID(t *testing.T) {
	now := time.Date(2026, 2, 20, 12, 30, 0, 0, time.UTC)
	api := &guidedWorkflowAPIMock{
		snapshotRuns: []*guidedworkflows.WorkflowRun{
			newWorkflowRunFixture("gwf-refresh-now", guidedworkflows.WorkflowRunStatusRunning, now),
		},
		snapshotTimelines: [][]guidedworkflows.RunTimelineEvent{
			{{At: now, Type: "run_started", RunID: "gwf-refresh-now"}},
		},
	}
	m := newPhase0ModelWithSession("codex")
	m.guidedWorkflowAPI = api
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
	})

	cmd := m.refreshGuidedWorkflowNow("refreshing guided workflow timeline")
	if cmd != nil {
		t.Fatalf("expected no command without an active run id")
	}
	if !strings.Contains(strings.ToLower(m.status), "no guided run to refresh") {
		t.Fatalf("expected missing-run status, got %q", m.status)
	}

	m.guidedWorkflow.SetRun(newWorkflowRunFixture("gwf-refresh-now", guidedworkflows.WorkflowRunStatusRunning, now))
	m.clockNow = now
	cmd = m.refreshGuidedWorkflowNow("refreshing guided workflow timeline")
	if cmd == nil {
		t.Fatalf("expected refresh command when run id is present")
	}
	_ = workflowRunSnapshotMsgFromCmd(t, cmd)
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

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
