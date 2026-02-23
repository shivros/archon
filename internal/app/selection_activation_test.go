package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type stubSelectionActivator struct {
	kind    SelectionKind
	handled bool
	cmd     tea.Cmd
	calls   int
}

func (s *stubSelectionActivator) Kind() SelectionKind {
	return s.kind
}

func (s *stubSelectionActivator) Activate(_ SelectionTarget, _ SelectionActivationContext) (bool, tea.Cmd) {
	s.calls++
	return s.handled, s.cmd
}

type stubSelectionEnterActionContext struct {
	toggleResult    bool
	toggleCalls     int
	syncCmd         tea.Cmd
	activateHandled bool
	activateCmd     tea.Cmd
	activateCalls   int
	validation      string
}

func (s *stubSelectionEnterActionContext) ToggleSelectedContainer() bool {
	s.toggleCalls++
	return s.toggleResult
}

func (s *stubSelectionEnterActionContext) SyncSidebarExpansionChange() tea.Cmd {
	return s.syncCmd
}

func (s *stubSelectionEnterActionContext) ActivateSelection(_ SelectionTarget) (bool, tea.Cmd) {
	s.activateCalls++
	return s.activateHandled, s.activateCmd
}

func (s *stubSelectionEnterActionContext) SetValidationStatus(message string) {
	s.validation = message
}

type stubSelectionActivationContext struct {
	openWorkflowCalls int
	openWorkflowRunID string
	openWorkflowCmd   tea.Cmd
	openSessionCalls  int
	openSessionID     string
}

func (s *stubSelectionActivationContext) OpenWorkflow(runID string) tea.Cmd {
	s.openWorkflowCalls++
	s.openWorkflowRunID = runID
	return s.openWorkflowCmd
}

func (s *stubSelectionActivationContext) OpenSessionCompose(sessionID string) {
	s.openSessionCalls++
	s.openSessionID = sessionID
}

type stubSelectionActivationService struct{}

func (stubSelectionActivationService) ActivateSelection(_ SelectionTarget, _ SelectionActivationContext) (bool, tea.Cmd) {
	return false, nil
}

type stubSelectionEnterActionService struct{}

func (stubSelectionEnterActionService) HandleEnter(_ SelectionTarget, _ SelectionEnterActionContext) (bool, tea.Cmd) {
	return false, nil
}

func TestSelectionTargetFromSidebarItemWorkflow(t *testing.T) {
	item := &sidebarItem{
		kind:        sidebarWorkflow,
		workflowID:  "gwf-1",
		collapsible: true,
		workflow: &guidedworkflows.WorkflowRun{
			ID:          "gwf-1",
			WorkspaceID: "ws1",
			WorktreeID:  "wt1",
			SessionID:   "s1",
		},
	}

	target := selectionTargetFromSidebarItem(item)
	if target.Kind != SelectionKindWorkflow {
		t.Fatalf("expected workflow target kind, got %v", target.Kind)
	}
	if target.WorkflowRunID != "gwf-1" {
		t.Fatalf("expected run id gwf-1, got %q", target.WorkflowRunID)
	}
	if target.WorkspaceID != "ws1" || target.WorktreeID != "wt1" || target.SessionID != "s1" {
		t.Fatalf("unexpected workflow target context: %#v", target)
	}
	if !target.Collapsible {
		t.Fatalf("expected collapsible workflow target")
	}
}

func TestSelectionTargetFromSidebarItemSession(t *testing.T) {
	item := &sidebarItem{
		kind:    sidebarSession,
		session: &types.Session{ID: "s1"},
		meta: &types.SessionMeta{
			WorkspaceID: "ws1",
			WorktreeID:  "wt1",
		},
	}

	target := selectionTargetFromSidebarItem(item)
	if target.Kind != SelectionKindSession {
		t.Fatalf("expected session target kind, got %v", target.Kind)
	}
	if target.SessionID != "s1" {
		t.Fatalf("expected session id s1, got %q", target.SessionID)
	}
	if target.WorkspaceID != "ws1" || target.WorktreeID != "wt1" {
		t.Fatalf("unexpected session target context: %#v", target)
	}
}

func TestSelectionTargetFromSidebarItemNil(t *testing.T) {
	target := selectionTargetFromSidebarItem(nil)
	if target.Kind != SelectionKindUnknown {
		t.Fatalf("expected unknown target kind for nil item, got %v", target.Kind)
	}
}

func TestSelectionTargetFromSidebarItemWorkspace(t *testing.T) {
	item := &sidebarItem{
		kind:        sidebarWorkspace,
		workspace:   &types.Workspace{ID: "ws1"},
		collapsible: true,
	}
	target := selectionTargetFromSidebarItem(item)
	if target.Kind != SelectionKindWorkspace {
		t.Fatalf("expected workspace kind, got %v", target.Kind)
	}
	if target.WorkspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", target.WorkspaceID)
	}
	if !target.containerToggleEligible() {
		t.Fatalf("expected workspace target to be toggle-eligible")
	}
}

func TestSelectionTargetFromSidebarItemWorktreeWithoutPointer(t *testing.T) {
	item := &sidebarItem{
		kind: sidebarWorktree,
	}
	target := selectionTargetFromSidebarItem(item)
	if target.Kind != SelectionKindWorktree {
		t.Fatalf("expected worktree kind, got %v", target.Kind)
	}
	if target.WorktreeID != "" {
		t.Fatalf("expected empty worktree id when pointer is missing, got %q", target.WorktreeID)
	}
}

func TestSelectionTargetFromSidebarItemRecentsFallsBackToUnknown(t *testing.T) {
	item := &sidebarItem{kind: sidebarRecentsAll}
	target := selectionTargetFromSidebarItem(item)
	if target.Kind != SelectionKindUnknown {
		t.Fatalf("expected unknown kind for recents item, got %v", target.Kind)
	}
}

func TestSelectionActivationServiceDispatchesByKind(t *testing.T) {
	workflow := &stubSelectionActivator{kind: SelectionKindWorkflow, handled: true}
	session := &stubSelectionActivator{kind: SelectionKindSession, handled: true}
	service := NewSelectionActivationService(workflow, session)

	handled, cmd := service.ActivateSelection(SelectionTarget{Kind: SelectionKindWorkflow}, newModelSelectionActivationContext(nil))
	if !handled {
		t.Fatalf("expected workflow activator to handle target")
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd from stub activator")
	}
	if workflow.calls != 1 {
		t.Fatalf("expected workflow activator call count 1, got %d", workflow.calls)
	}
	if session.calls != 0 {
		t.Fatalf("expected session activator call count 0, got %d", session.calls)
	}
}

func TestSelectionActivationServiceSkipsNilActivator(t *testing.T) {
	var nilActivator SelectionActivator
	workflow := &stubSelectionActivator{kind: SelectionKindWorkflow, handled: true}
	service := NewSelectionActivationService(nilActivator, workflow)

	context := &stubSelectionActivationContext{}
	handled, _ := service.ActivateSelection(SelectionTarget{Kind: SelectionKindWorkflow}, context)
	if !handled {
		t.Fatalf("expected workflow activator to handle target")
	}
	if workflow.calls != 1 {
		t.Fatalf("expected workflow activator call count 1, got %d", workflow.calls)
	}
}

func TestSelectionActivationServiceReturnsUnhandledForNilContext(t *testing.T) {
	service := NewDefaultSelectionActivationService()
	handled, cmd := service.ActivateSelection(SelectionTarget{Kind: SelectionKindWorkflow, WorkflowRunID: "gwf-1"}, nil)
	if handled {
		t.Fatalf("expected nil context to return unhandled")
	}
	if cmd != nil {
		t.Fatalf("expected nil command for nil context")
	}
}

func TestSelectionActivationServiceWorkflowSelectionOpensGuidedWorkflow(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, workflows, map[string]*types.SessionMeta{}, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkflow)

	service := NewDefaultSelectionActivationService()
	target := selectionTargetFromSidebarItem(m.selectedItem())
	handled, cmd := service.ActivateSelection(target, newModelSelectionActivationContext(&m))
	if !handled {
		t.Fatalf("expected workflow selection to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected workflow activation command")
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.RunID() != "gwf-1" {
		t.Fatalf("expected guided workflow run gwf-1 to be active")
	}
}

func TestSelectionActivationServiceSessionSelectionEntersCompose(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	service := NewDefaultSelectionActivationService()
	target := selectionTargetFromSidebarItem(m.selectedItem())
	handled, cmd := service.ActivateSelection(target, newModelSelectionActivationContext(&m))
	if !handled {
		t.Fatalf("expected session selection to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected session activation to avoid async command")
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode, got %v", m.mode)
	}
}

func TestSelectionActivationServiceIgnoresUnknownSelection(t *testing.T) {
	service := NewDefaultSelectionActivationService()
	handled, cmd := service.ActivateSelection(SelectionTarget{Kind: SelectionKindWorkspace}, newModelSelectionActivationContext(nil))
	if handled {
		t.Fatalf("expected workspace selection to remain non-activatable")
	}
	if cmd != nil {
		t.Fatalf("expected no command for non-activatable selection")
	}
}

func TestWorkflowSelectionActivatorNilContext(t *testing.T) {
	activator := workflowSelectionActivator{}
	handled, cmd := activator.Activate(SelectionTarget{Kind: SelectionKindWorkflow, WorkflowRunID: "gwf-1"}, nil)
	if handled {
		t.Fatalf("expected nil context to return unhandled")
	}
	if cmd != nil {
		t.Fatalf("expected nil command for nil context")
	}
}

func TestSessionSelectionActivatorBranches(t *testing.T) {
	activator := sessionSelectionActivator{}
	context := &stubSelectionActivationContext{}

	handled, cmd := activator.Activate(SelectionTarget{Kind: SelectionKindSession, SessionID: " "}, context)
	if !handled {
		t.Fatalf("expected empty session id to be handled as no-op")
	}
	if cmd != nil {
		t.Fatalf("expected nil command for empty session id")
	}
	if context.openSessionCalls != 0 {
		t.Fatalf("expected no compose open calls for empty session id, got %d", context.openSessionCalls)
	}

	handled, cmd = activator.Activate(SelectionTarget{Kind: SelectionKindSession, SessionID: "s1"}, context)
	if !handled {
		t.Fatalf("expected session activation to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected nil command for compose activation")
	}
	if context.openSessionCalls != 1 || context.openSessionID != "s1" {
		t.Fatalf("unexpected compose open call state: calls=%d id=%q", context.openSessionCalls, context.openSessionID)
	}

	handled, cmd = activator.Activate(SelectionTarget{Kind: SelectionKindSession, SessionID: "s1"}, nil)
	if handled {
		t.Fatalf("expected nil context to return unhandled")
	}
	if cmd != nil {
		t.Fatalf("expected nil command for nil context")
	}
}

func TestSelectionEnterActionServicePrefersContainerToggle(t *testing.T) {
	context := &stubSelectionEnterActionContext{
		toggleResult:    true,
		activateHandled: true,
	}
	service := NewDefaultSelectionEnterActionService()
	target := SelectionTarget{Kind: SelectionKindWorkspace, Collapsible: true}

	handled, _ := service.HandleEnter(target, context)
	if !handled {
		t.Fatalf("expected enter handling for collapsible workspace")
	}
	if context.toggleCalls != 1 {
		t.Fatalf("expected one toggle attempt, got %d", context.toggleCalls)
	}
	if context.activateCalls != 0 {
		t.Fatalf("expected activation to be skipped when toggle succeeds, got %d calls", context.activateCalls)
	}
}

func TestSelectionEnterActionServiceSkipsActivationWhenToggleFails(t *testing.T) {
	context := &stubSelectionEnterActionContext{
		toggleResult:    false,
		activateHandled: true,
	}
	service := NewDefaultSelectionEnterActionService()
	target := SelectionTarget{Kind: SelectionKindWorkspace, Collapsible: true}

	handled, cmd := service.HandleEnter(target, context)
	if !handled {
		t.Fatalf("expected enter handling for collapsible workspace")
	}
	if cmd != nil {
		t.Fatalf("expected nil command when toggle does not change state")
	}
	if context.activateCalls != 0 {
		t.Fatalf("expected activation to be skipped when target is toggle-eligible")
	}
}

func TestSelectionEnterActionServiceFallsBackToActivation(t *testing.T) {
	context := &stubSelectionEnterActionContext{
		activateHandled: true,
	}
	service := NewDefaultSelectionEnterActionService()
	target := SelectionTarget{Kind: SelectionKindWorkflow}

	handled, _ := service.HandleEnter(target, context)
	if !handled {
		t.Fatalf("expected enter handling when activation succeeds")
	}
	if context.activateCalls != 1 {
		t.Fatalf("expected one activation attempt, got %d", context.activateCalls)
	}
	if context.validation != "" {
		t.Fatalf("did not expect validation status when activation succeeds, got %q", context.validation)
	}
}

func TestSelectionEnterActionServiceSetsValidationWhenUnhandled(t *testing.T) {
	context := &stubSelectionEnterActionContext{}
	service := NewDefaultSelectionEnterActionService()
	target := SelectionTarget{Kind: SelectionKindUnknown}

	handled, _ := service.HandleEnter(target, context)
	if !handled {
		t.Fatalf("expected enter handling even when selection is invalid")
	}
	if context.validation != "select a session to chat" {
		t.Fatalf("unexpected validation status %q", context.validation)
	}
}

func TestSelectionEnterActionServiceWithNilContext(t *testing.T) {
	service := NewDefaultSelectionEnterActionService()
	handled, cmd := service.HandleEnter(SelectionTarget{Kind: SelectionKindSession}, nil)
	if handled {
		t.Fatalf("expected nil context to be unhandled")
	}
	if cmd != nil {
		t.Fatalf("expected nil command for nil context")
	}
}

func TestSelectionActivationModelAdapterBranches(t *testing.T) {
	m := NewModel(nil)
	ctx := newModelSelectionActivationContext(&m)

	if cmd := ctx.OpenWorkflow(" "); cmd != nil {
		t.Fatalf("expected nil command for empty workflow id")
	}
	if m.status != "select a workflow" {
		t.Fatalf("unexpected validation status %q", m.status)
	}

	if cmd := ctx.OpenWorkflow("gwf-missing"); cmd != nil {
		t.Fatalf("expected nil command for missing workflow")
	}
	if m.status != "workflow not found in sidebar" {
		t.Fatalf("unexpected missing workflow status %q", m.status)
	}

	m2 := NewModel(nil)
	m2.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m2.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m2.worktrees = map[string][]*types.Worktree{}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m2.sidebar.Apply(m2.workspaces, m2.worktrees, nil, workflows, map[string]*types.SessionMeta{}, "", "", false)
	m2.sidebar.Select(0)
	ctx2 := newModelSelectionActivationContext(&m2)
	cmd := ctx2.OpenWorkflow("gwf-1")
	if cmd == nil {
		t.Fatalf("expected workflow open command")
	}
	if m2.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode after open, got %v", m2.mode)
	}

	var nilModel *Model
	nilCtx := newModelSelectionActivationContext(nilModel)
	if cmd := nilCtx.OpenWorkflow("gwf-1"); cmd != nil {
		t.Fatalf("expected nil command for nil model")
	}
	nilCtx.OpenSessionCompose("s1")
}

func TestSelectionActivationModelAdapterSessionComposeEmptyNoop(t *testing.T) {
	m := NewModel(nil)
	ctx := newModelSelectionActivationContext(&m)
	ctx.OpenSessionCompose(" ")
	if m.mode != uiModeNormal {
		t.Fatalf("expected empty session id to keep normal mode, got %v", m.mode)
	}
}

func TestSelectionEnterModelAdapterBranches(t *testing.T) {
	var nilModel *Model
	nilCtx := newModelSelectionEnterActionContext(nilModel)
	if nilCtx.ToggleSelectedContainer() {
		t.Fatalf("expected nil model toggle to return false")
	}
	if cmd := nilCtx.SyncSidebarExpansionChange(); cmd != nil {
		t.Fatalf("expected nil model sync command to be nil")
	}
	if handled, cmd := nilCtx.ActivateSelection(SelectionTarget{Kind: SelectionKindSession, SessionID: "s1"}); handled || cmd != nil {
		t.Fatalf("expected nil model activation to be unhandled with nil command")
	}
	nilCtx.SetValidationStatus("ignored")

	m := NewModel(nil)
	ctx := newModelSelectionEnterActionContext(&m)
	ctx.SetValidationStatus("selection invalid")
	if m.status != "selection invalid" {
		t.Fatalf("expected status update, got %q", m.status)
	}
}

func TestSelectionActivationAndEnterServiceOptionsAndFallbacks(t *testing.T) {
	customActivation := stubSelectionActivationService{}
	customEnter := stubSelectionEnterActionService{}

	m := NewModel(nil, WithSelectionActivationService(customActivation), WithSelectionEnterActionService(customEnter))
	if _, ok := m.selectionActivationService.(stubSelectionActivationService); !ok {
		t.Fatalf("expected custom selection activation service, got %T", m.selectionActivationService)
	}
	if _, ok := m.selectionEnterActionService.(stubSelectionEnterActionService); !ok {
		t.Fatalf("expected custom selection enter service, got %T", m.selectionEnterActionService)
	}

	mDefault := NewModel(nil, WithSelectionActivationService(nil), WithSelectionEnterActionService(nil))
	if mDefault.selectionActivationService == nil {
		t.Fatalf("expected default selection activation service")
	}
	if mDefault.selectionEnterActionService == nil {
		t.Fatalf("expected default selection enter service")
	}

	optActivation := WithSelectionActivationService(customActivation)
	optEnter := WithSelectionEnterActionService(customEnter)
	optActivation(nil)
	optEnter(nil)
}

func TestSelectionServiceOrDefaultHelpers(t *testing.T) {
	var nilModel *Model
	if nilModel.selectionActivationServiceOrDefault() == nil {
		t.Fatalf("expected default activation service for nil model")
	}
	if nilModel.selectionEnterActionServiceOrDefault() == nil {
		t.Fatalf("expected default enter service for nil model")
	}

	m := NewModel(nil)
	m.selectionActivationService = nil
	m.selectionEnterActionService = nil
	if m.selectionActivationServiceOrDefault() == nil {
		t.Fatalf("expected default activation service for nil field")
	}
	if m.selectionEnterActionServiceOrDefault() == nil {
		t.Fatalf("expected default enter service for nil field")
	}
}

func TestModelKeyActionServiceFallbackPaths(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session fixture to be selectable")
	}
	m.selectionActivationService = nil
	m.selectionEnterActionService = nil

	handled, cmd := m.activateSelectionTarget(SelectionTarget{Kind: SelectionKindSession, SessionID: "s1"})
	if !handled {
		t.Fatalf("expected activation fallback to default service")
	}
	if cmd != nil {
		t.Fatalf("expected session compose activation to return nil cmd")
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode after activation, got %v", m.mode)
	}
}
