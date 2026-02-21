package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type stubSelectionTransitionService struct {
	calls  int
	delay  time.Duration
	source selectionChangeSource
	cmd    tea.Cmd
}

type stubSelectionFocusPolicy struct {
	openWorkflow bool
	exitGuided   bool
}

type selectionTransitionTestMsg struct{}

func (s stubSelectionFocusPolicy) ShouldOpenWorkflowSelection(item *sidebarItem, _ selectionChangeSource) bool {
	return s.openWorkflow && item != nil && item.kind == sidebarWorkflow
}

func (s stubSelectionFocusPolicy) ShouldExitGuidedWorkflowForSessionSelection(_ uiMode, _ *sidebarItem, _ selectionChangeSource) bool {
	return s.exitGuided
}

func (s *stubSelectionTransitionService) SelectionChanged(_ *Model, delay time.Duration, source selectionChangeSource) tea.Cmd {
	s.calls++
	s.delay = delay
	s.source = source
	return s.cmd
}

func TestOnSelectionChangedDelegatesToSelectionTransitionService(t *testing.T) {
	stub := &stubSelectionTransitionService{}
	m := NewModel(nil, WithSelectionTransitionService(stub))

	cmd := m.onSelectionChangedWithDelayAndSource(25*time.Millisecond, selectionChangeSourceSystem)
	if stub.calls != 1 {
		t.Fatalf("expected transition service call count 1, got %d", stub.calls)
	}
	if stub.delay != 25*time.Millisecond {
		t.Fatalf("expected delay to be propagated, got %s", stub.delay)
	}
	if stub.source != selectionChangeSourceSystem {
		t.Fatalf("expected source %v, got %v", selectionChangeSourceSystem, stub.source)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd from stub transition service")
	}
}

func TestSelectionOriginPolicyDefaults(t *testing.T) {
	policy := DefaultSelectionOriginPolicy()
	if got := policy.HistoryActionForSource(selectionChangeSourceUser); got != SelectionHistoryActionVisit {
		t.Fatalf("expected user source to visit history, got %v", got)
	}
	if got := policy.HistoryActionForSource(selectionChangeSourceSystem); got != SelectionHistoryActionSyncCurrent {
		t.Fatalf("expected system source to sync current history, got %v", got)
	}
	if got := policy.HistoryActionForSource(selectionChangeSourceHistory); got != SelectionHistoryActionSyncCurrent {
		t.Fatalf("expected history source to sync current history, got %v", got)
	}
}

func TestSelectionOriginPolicySupportsFallback(t *testing.T) {
	policy := NewSelectionOriginPolicy(map[selectionChangeSource]SelectionHistoryAction{
		selectionChangeSourceUser: SelectionHistoryActionIgnore,
	}, SelectionHistoryActionVisit)
	if got := policy.HistoryActionForSource(selectionChangeSourceUser); got != SelectionHistoryActionIgnore {
		t.Fatalf("expected user source override, got %v", got)
	}
	if got := policy.HistoryActionForSource(selectionChangeSourceSystem); got != SelectionHistoryActionVisit {
		t.Fatalf("expected fallback action for system source, got %v", got)
	}
}

func TestSystemSelectionOnWorkflowDoesNotOpenGuidedWorkflow(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	now := time.Now().UTC()
	m.workflowRuns = []*guidedworkflows.WorkflowRun{
		{
			ID:          "gwf-1",
			WorkspaceID: "ws1",
			Status:      guidedworkflows.WorkflowRunStatusRunning,
			CreatedAt:   now,
		},
	}
	m.sessionMeta["s1"] = &types.SessionMeta{
		SessionID:     "s1",
		WorkspaceID:   "ws1",
		WorkflowRunID: "gwf-1",
	}
	m.applySidebarItems()
	if m.sidebar == nil || !m.sidebar.SelectByWorkflowID("gwf-1") {
		t.Fatalf("expected workflow to be selectable")
	}

	_ = m.onSystemSelectionChangedImmediate()
	if m.mode == uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode to stay closed for system-origin selection")
	}
}

func TestSessionSelectionExitsGuidedWorkflowMode(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	now := time.Now().UTC()
	run := &guidedworkflows.WorkflowRun{
		ID:          "gwf-1",
		WorkspaceID: "ws1",
		Status:      guidedworkflows.WorkflowRunStatusRunning,
		CreatedAt:   now,
	}
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}
	m.sessionMeta["s1"] = &types.SessionMeta{
		SessionID:     "s1",
		WorkspaceID:   "ws1",
		WorkflowRunID: run.ID,
	}
	m.applySidebarItems()
	if m.sidebar == nil || !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected workflow row to be selectable")
	}
	if cmd := m.onSelectionChangedImmediate(); cmd == nil {
		t.Fatalf("expected guided workflow open command")
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode before session selection")
	}

	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected workflow child session row to be selectable")
	}
	cmd := m.onSelectionChangedImmediate()
	if cmd == nil {
		t.Fatalf("expected session load command after selecting workflow child session")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected guided workflow mode to exit after session selection, got %v", m.mode)
	}
}

func TestSelectionTransitionResolveSelectionCommandHonorsFocusPolicy(t *testing.T) {
	service := defaultSelectionTransitionService{}
	m := newPhase0ModelWithSession("codex")
	now := time.Now().UTC()
	run := &guidedworkflows.WorkflowRun{
		ID:          "gwf-1",
		WorkspaceID: "ws1",
		Status:      guidedworkflows.WorkflowRunStatusRunning,
		CreatedAt:   now,
	}
	m.workflowRuns = []*guidedworkflows.WorkflowRun{run}
	m.applySidebarItems()
	if m.sidebar == nil || !m.sidebar.SelectByWorkflowID(run.ID) {
		t.Fatalf("expected workflow row to be selectable")
	}
	item := m.selectedItem()
	if item == nil || item.kind != sidebarWorkflow {
		t.Fatalf("expected selected workflow item")
	}
	if cmd := service.resolveSelectionCommand(&m, true, item, 0, selectionChangeSourceUser, stubSelectionFocusPolicy{openWorkflow: false}); cmd != nil {
		t.Fatalf("expected nil command when focus policy denies workflow open")
	}
	if cmd := service.resolveSelectionCommand(&m, true, item, 0, selectionChangeSourceUser, stubSelectionFocusPolicy{openWorkflow: true}); cmd == nil {
		t.Fatalf("expected workflow command when focus policy allows workflow open")
	}
}

func TestSelectionTransitionWithSelectionStatePersistenceBehavior(t *testing.T) {
	service := defaultSelectionTransitionService{}
	m := newPhase0ModelWithSession("codex")

	if cmd := service.withSelectionStatePersistence(&m, selectionTransitionOutcome{}); cmd != nil {
		t.Fatalf("expected no command when transition has no work")
	}

	m.stateAPI = &phase1AppStateSyncStub{}
	m.hasAppState = true
	cmd := service.withSelectionStatePersistence(&m, selectionTransitionOutcome{
		stateChanged: true,
	})
	if cmd == nil {
		t.Fatalf("expected app state save command when state changed")
	}
	if !m.appStateSaveScheduled {
		t.Fatalf("expected app state save scheduling flag to be set")
	}
}

func TestSelectionTransitionWithSelectionStatePersistenceBatchesCommandAndSave(t *testing.T) {
	service := defaultSelectionTransitionService{}
	m := newPhase0ModelWithSession("codex")
	m.stateAPI = &phase1AppStateSyncStub{}
	m.hasAppState = true
	outcomeCmd := func() tea.Msg { return selectionTransitionTestMsg{} }

	cmd := service.withSelectionStatePersistence(&m, selectionTransitionOutcome{
		command:      outcomeCmd,
		stateChanged: true,
	})
	if cmd == nil {
		t.Fatalf("expected batched command when outcome command and save command are both present")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 batched commands, got %d", len(batch))
	}
}

func TestSelectionTransitionWithSelectionStatePersistenceNilModelPassthrough(t *testing.T) {
	service := defaultSelectionTransitionService{}
	outcomeCmd := func() tea.Msg { return selectionTransitionTestMsg{} }

	cmd := service.withSelectionStatePersistence(nil, selectionTransitionOutcome{command: outcomeCmd, stateChanged: true})
	if cmd == nil {
		t.Fatalf("expected passthrough command for nil model")
	}
	if _, ok := cmd().(selectionTransitionTestMsg); !ok {
		t.Fatalf("expected passthrough outcome command message")
	}
}

func TestSelectionTransitionWithSelectionStatePersistenceReturnsOutcomeWhenSaveUnavailable(t *testing.T) {
	service := defaultSelectionTransitionService{}
	m := newPhase0ModelWithSession("codex")
	outcomeCmd := func() tea.Msg { return selectionTransitionTestMsg{} }

	cmd := service.withSelectionStatePersistence(&m, selectionTransitionOutcome{
		command:      outcomeCmd,
		stateChanged: true,
	})
	if cmd == nil {
		t.Fatalf("expected outcome command when save command is unavailable")
	}
	if _, ok := cmd().(selectionTransitionTestMsg); !ok {
		t.Fatalf("expected outcome command message when save is unavailable")
	}
}

func TestApplySelectionFocusTransitionIgnoresNilFocusPolicy(t *testing.T) {
	service := defaultSelectionTransitionService{}
	m := newPhase0ModelWithSession("codex")
	m.mode = uiModeGuidedWorkflow
	item := &sidebarItem{
		kind:    sidebarSession,
		session: &types.Session{ID: "s1"},
	}
	service.applySelectionFocusTransition(&m, item, selectionChangeSourceUser, nil)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected nil focus policy to leave mode unchanged")
	}
}

func TestApplySelectionFocusTransitionNilModelNoop(t *testing.T) {
	service := defaultSelectionTransitionService{}
	service.applySelectionFocusTransition(nil, nil, selectionChangeSourceUser, stubSelectionFocusPolicy{exitGuided: true})
}

func TestSelectionChangedNilModelReturnsNil(t *testing.T) {
	service := defaultSelectionTransitionService{}
	if cmd := service.SelectionChanged(nil, 0, selectionChangeSourceUser); cmd != nil {
		t.Fatalf("expected nil command for nil model")
	}
}
