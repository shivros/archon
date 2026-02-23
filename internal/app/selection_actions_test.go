package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type invalidSelectionAction struct{}

func (invalidSelectionAction) Validate(*Model) error { return errors.New("invalid action") }

func (invalidSelectionAction) ConfirmSpec(*Model) selectionActionConfirmSpec {
	return selectionActionConfirmSpec{title: "Invalid", message: "Invalid?"}
}

func (invalidSelectionAction) Execute(*Model) tea.Cmd { return nil }

type blankConfirmSelectionAction struct{}

func (blankConfirmSelectionAction) Validate(*Model) error { return nil }

func (blankConfirmSelectionAction) ConfirmSpec(*Model) selectionActionConfirmSpec {
	return selectionActionConfirmSpec{}
}

func (blankConfirmSelectionAction) Execute(*Model) tea.Cmd { return nil }

func TestResolveDismissOrDeleteSelectionActionNilItem(t *testing.T) {
	action, err := resolveDismissOrDeleteSelectionAction(nil)
	if err == nil {
		t.Fatalf("expected validation error for nil item")
	}
	if action != nil {
		t.Fatalf("expected nil action on validation failure")
	}
}

func TestResolveDismissOrDeleteSelectionActionWorkflow(t *testing.T) {
	item := &sidebarItem{
		kind:       sidebarWorkflow,
		workflowID: "gwf-1",
		workflow:   &guidedworkflows.WorkflowRun{ID: "gwf-1"},
	}
	action, err := resolveDismissOrDeleteSelectionAction(item)
	if err != nil {
		t.Fatalf("unexpected resolve error: %v", err)
	}
	workflowAction, ok := action.(dismissWorkflowSelectionAction)
	if !ok {
		t.Fatalf("expected dismissWorkflowSelectionAction, got %T", action)
	}
	if workflowAction.runID != "gwf-1" {
		t.Fatalf("expected workflow run id gwf-1, got %q", workflowAction.runID)
	}
}

func TestResolveDismissOrDeleteSelectionActionInvalidItemShapes(t *testing.T) {
	cases := []struct {
		name string
		item *sidebarItem
		want string
	}{
		{
			name: "workspace missing payload",
			item: &sidebarItem{kind: sidebarWorkspace},
			want: "select a workspace to delete",
		},
		{
			name: "worktree missing payload",
			item: &sidebarItem{kind: sidebarWorktree},
			want: "select a worktree to delete",
		},
		{
			name: "session missing payload",
			item: &sidebarItem{kind: sidebarSession},
			want: "select a session to dismiss",
		},
		{
			name: "unsupported kind",
			item: &sidebarItem{kind: sidebarRecentsAll},
			want: "select an item to dismiss or delete",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action, err := resolveDismissOrDeleteSelectionAction(tc.item)
			if err == nil {
				t.Fatalf("expected resolve error")
			}
			if action != nil {
				t.Fatalf("expected nil action on resolve error")
			}
			if err.Error() != tc.want {
				t.Fatalf("expected error %q, got %q", tc.want, err.Error())
			}
		})
	}
}

func TestSelectionActionsValidateFailures(t *testing.T) {
	cases := []struct {
		name   string
		action selectionAction
		want   string
	}{
		{
			name:   "workspace empty id",
			action: deleteWorkspaceSelectionAction{},
			want:   "select a workspace to delete",
		},
		{
			name:   "worktree empty id",
			action: deleteWorktreeSelectionAction{},
			want:   "select a worktree to delete",
		},
		{
			name:   "session empty id",
			action: dismissSessionSelectionAction{},
			want:   "select a session to dismiss",
		},
		{
			name:   "workflow empty id",
			action: dismissWorkflowSelectionAction{},
			want:   "select a workflow to dismiss",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.action.Validate(nil)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if err.Error() != tc.want {
				t.Fatalf("expected error %q, got %q", tc.want, err.Error())
			}
		})
	}
}

func TestOpenSelectionActionConfirmRejectsInvalidAction(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	m.openSelectionActionConfirm(invalidSelectionAction{})
	if m.status != "invalid action" {
		t.Fatalf("expected validation status from action, got %q", m.status)
	}
	if m.confirm != nil && m.confirm.IsOpen() {
		t.Fatalf("expected confirm modal to remain closed")
	}
}

func TestOpenSelectionActionConfirmUsesDefaultSpecValues(t *testing.T) {
	m := NewModel(nil)
	m.openSelectionActionConfirm(blankConfirmSelectionAction{})
	if m.confirm == nil || !m.confirm.IsOpen() {
		t.Fatalf("expected confirm modal to be open")
	}
	if m.confirm.title != "Confirm" {
		t.Fatalf("expected default title, got %q", m.confirm.title)
	}
	if m.confirm.message != "Are you sure?" {
		t.Fatalf("expected default message, got %q", m.confirm.message)
	}
	if m.confirm.confirmLabel != "Confirm" {
		t.Fatalf("expected default confirm label, got %q", m.confirm.confirmLabel)
	}
	if m.confirm.cancelLabel != "Cancel" {
		t.Fatalf("expected default cancel label, got %q", m.confirm.cancelLabel)
	}
}

func TestOpenSelectionActionConfirmNilGuards(t *testing.T) {
	var nilModel *Model
	nilModel.openSelectionActionConfirm(blankConfirmSelectionAction{})

	m := NewModel(nil)
	m.confirm = nil
	m.openSelectionActionConfirm(blankConfirmSelectionAction{})
	if m.pendingSelectionAction != nil {
		t.Fatalf("expected no pending selection action when confirm controller is absent")
	}
}

func TestOpenSelectionActionConfirmNilActionSetsValidation(t *testing.T) {
	m := NewModel(nil)
	m.openSelectionActionConfirm(nil)
	if m.status != "select an item to act on" {
		t.Fatalf("expected nil-action validation status, got %q", m.status)
	}
}

func TestHandleConfirmChoiceExecutesSelectionActions(t *testing.T) {
	cases := []struct {
		name       string
		action     selectionAction
		wantStatus string
	}{
		{
			name:       "delete workspace",
			action:     deleteWorkspaceSelectionAction{workspaceID: "ws1"},
			wantStatus: "deleting workspace",
		},
		{
			name:       "delete worktree",
			action:     deleteWorktreeSelectionAction{workspaceID: "ws1", worktreeID: "wt1"},
			wantStatus: "deleting worktree",
		},
		{
			name:       "dismiss session",
			action:     dismissSessionSelectionAction{sessionID: "s1"},
			wantStatus: "dismissing s1",
		},
		{
			name:       "dismiss workflow",
			action:     dismissWorkflowSelectionAction{runID: "gwf-1"},
			wantStatus: "dismissing workflow gwf-1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(nil)
			m.openSelectionActionConfirm(tc.action)
			if m.confirm == nil || !m.confirm.IsOpen() {
				t.Fatalf("expected confirm modal to be open")
			}
			cmd := m.handleConfirmChoice(confirmChoiceConfirm)
			if cmd == nil {
				t.Fatalf("expected command from confirm execution")
			}
			if m.status != tc.wantStatus {
				t.Fatalf("expected status %q, got %q", tc.wantStatus, m.status)
			}
			if m.pendingSelectionAction != nil {
				t.Fatalf("expected pending selection action to clear after confirm")
			}
		})
	}
}

func TestHandleConfirmChoiceCancelSelectionAction(t *testing.T) {
	m := NewModel(nil)
	m.openSelectionActionConfirm(dismissSessionSelectionAction{sessionID: "s1"})
	cmd := m.handleConfirmChoice(confirmChoiceCancel)
	if cmd != nil {
		t.Fatalf("expected no command on cancel")
	}
	if m.status != "canceled" {
		t.Fatalf("expected canceled status, got %q", m.status)
	}
	if m.pendingSelectionAction != nil {
		t.Fatalf("expected pending selection action to clear on cancel")
	}
}

func TestSelectionActionExecuteNilModelReturnsNil(t *testing.T) {
	actions := []selectionAction{
		deleteWorkspaceSelectionAction{workspaceID: "ws1"},
		deleteWorktreeSelectionAction{workspaceID: "ws1", worktreeID: "wt1"},
		dismissSessionSelectionAction{sessionID: "s1"},
		dismissWorkflowSelectionAction{runID: "gwf-1"},
	}
	for _, action := range actions {
		if cmd := action.Execute(nil); cmd != nil {
			t.Fatalf("expected nil command for nil model, got %T", cmd)
		}
	}
}

func TestResolveDismissOrDeleteSelectionActionForItemsMixedSelection(t *testing.T) {
	items := []*sidebarItem{
		{
			kind:      sidebarWorkspace,
			workspace: &types.Workspace{ID: "ws1", Name: "Workspace"},
		},
		{
			kind:     sidebarSession,
			session:  &types.Session{ID: "s1"},
			meta:     &types.SessionMeta{SessionID: "s1", Title: "Session"},
			workflow: nil,
		},
	}

	action, err := resolveDismissOrDeleteSelectionActionForItems(items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	batch, ok := action.(selectionBatchAction)
	if !ok {
		t.Fatalf("expected selectionBatchAction, got %T", action)
	}
	if len(batch.operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(batch.operations))
	}
	spec := batch.ConfirmSpec(nil)
	if spec.message == "" || !containsAll(spec.message, []string{"Delete:", "Dismiss:", "Workspace", "Session"}) {
		t.Fatalf("expected grouped confirmation message, got %q", spec.message)
	}
}

func TestResolveInterruptOrStopSelectionActionSkipsStoppedWorkflow(t *testing.T) {
	items := []*sidebarItem{
		{
			kind:    sidebarSession,
			session: &types.Session{ID: "s1", Status: types.SessionStatusRunning},
			meta:    &types.SessionMeta{SessionID: "s1", Title: "Session"},
		},
		{
			kind:     sidebarWorkflow,
			workflow: &guidedworkflows.WorkflowRun{ID: "gwf-1", Status: guidedworkflows.WorkflowRunStatusStopped, TemplateName: "SOLID"},
		},
	}

	action, err := resolveInterruptOrStopSelectionAction(nil, items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	batch, ok := action.(selectionBatchAction)
	if !ok {
		t.Fatalf("expected selectionBatchAction, got %T", action)
	}
	if len(batch.operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(batch.operations))
	}
	if batch.operations[0].kind != selectionOperationInterruptSession {
		t.Fatalf("expected interrupt session operation, got %v", batch.operations[0].kind)
	}
	if batch.skippedCount != 1 {
		t.Fatalf("expected skipped count 1, got %d", batch.skippedCount)
	}
}

func TestResolveKillSelectionActionFiltersTerminalSessions(t *testing.T) {
	items := []*sidebarItem{
		{
			kind:    sidebarSession,
			session: &types.Session{ID: "s1", Status: types.SessionStatusRunning},
		},
		{
			kind:    sidebarSession,
			session: &types.Session{ID: "s2", Status: types.SessionStatusExited},
		},
	}

	action, err := resolveKillSelectionAction(nil, items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	batch, ok := action.(selectionBatchAction)
	if !ok {
		t.Fatalf("expected selectionBatchAction, got %T", action)
	}
	if len(batch.operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(batch.operations))
	}
	if batch.operations[0].kind != selectionOperationKillSession {
		t.Fatalf("expected kill session operation, got %v", batch.operations[0].kind)
	}
}

func containsAll(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

type recordingSelectionOperationExecutionContext struct {
	operations []selectionOperation
	statuses   []string
	returnNil  bool
}

func (c *recordingSelectionOperationExecutionContext) CommandForSelectionOperation(operation selectionOperation) tea.Cmd {
	c.operations = append(c.operations, operation)
	if c.returnNil {
		return nil
	}
	return func() tea.Msg { return operation.kind }
}

func (c *recordingSelectionOperationExecutionContext) SetSelectionOperationStatus(message string) {
	c.statuses = append(c.statuses, message)
}

type recordingSelectionOperationExecutor struct {
	plan        SelectionOperationPlan
	called      bool
	statusCalls int
}

func (e *recordingSelectionOperationExecutor) Execute(plan SelectionOperationPlan, context SelectionOperationExecutionContext) tea.Cmd {
	e.called = true
	e.plan = plan
	if context != nil {
		context.SetSelectionOperationStatus("stub-executor")
		e.statusCalls++
	}
	return func() tea.Msg { return "executed-selection-batch" }
}

type noopSelectionOperationPlanner struct {
	plan SelectionOperationPlan
	err  error
}

func (p noopSelectionOperationPlanner) Plan(selectionCommandKind, []*sidebarItem, SelectionOperationPlanningContext) (SelectionOperationPlan, error) {
	return p.plan, p.err
}

type noopSelectionConfirmationPresenter struct {
	spec selectionActionConfirmSpec
}

func (p noopSelectionConfirmationPresenter) ConfirmSpec(SelectionOperationPlan) selectionActionConfirmSpec {
	return p.spec
}

type fixedWorkflowPlanningContext struct {
	status guidedworkflows.WorkflowRunStatus
	ok     bool
}

func (c fixedWorkflowPlanningContext) WorkflowRunStatus(string) (guidedworkflows.WorkflowRunStatus, bool) {
	return c.status, c.ok
}

func TestDefaultSelectionOperationExecutorExecuteDispatchesAndSetsStatus(t *testing.T) {
	executor := NewDefaultSelectionOperationExecutor()
	plan := SelectionOperationPlan{
		Command: selectionCommandInterruptStop,
		Operations: []selectionOperation{
			{kind: selectionOperationInterruptSession, sessionID: "s1"},
			{kind: selectionOperationStopWorkflow, runID: "gwf-1"},
		},
	}
	ctx := &recordingSelectionOperationExecutionContext{}

	cmd := executor.Execute(plan, ctx)
	if cmd == nil {
		t.Fatalf("expected batch command")
	}
	if len(ctx.operations) != 2 {
		t.Fatalf("expected 2 operations dispatched, got %d", len(ctx.operations))
	}
	if len(ctx.statuses) != 1 || ctx.statuses[0] != "processing interrupt/stop for 2 item(s)" {
		t.Fatalf("unexpected statuses: %#v", ctx.statuses)
	}
}

func TestDefaultSelectionOperationExecutorExecuteHandlesNilAndNoopCases(t *testing.T) {
	executor := NewDefaultSelectionOperationExecutor()
	plan := SelectionOperationPlan{Command: selectionCommandKill}
	if cmd := executor.Execute(plan, nil); cmd != nil {
		t.Fatalf("expected nil command when context is nil")
	}

	ctx := &recordingSelectionOperationExecutionContext{returnNil: true}
	plan.Operations = []selectionOperation{{kind: selectionOperationKillSession, sessionID: "s1"}}
	if cmd := executor.Execute(plan, ctx); cmd != nil {
		t.Fatalf("expected nil command when all operation commands are nil")
	}
	if len(ctx.statuses) != 1 || ctx.statuses[0] != "processing kill for 1 item(s)" {
		t.Fatalf("unexpected status updates: %#v", ctx.statuses)
	}
}

func TestSelectionBatchActionExecuteUsesInjectedExecutor(t *testing.T) {
	executor := &recordingSelectionOperationExecutor{}
	action := selectionBatchAction{
		command:    selectionCommandDismissDelete,
		operations: []selectionOperation{{kind: selectionOperationDismissSession, sessionID: "s1"}},
		executor:   executor,
	}
	m := NewModel(nil)
	cmd := action.Execute(&m)
	if cmd == nil {
		t.Fatalf("expected command from injected executor")
	}
	msg := cmd()
	if msg != "executed-selection-batch" {
		t.Fatalf("expected injected executor message, got %#v", msg)
	}
	if !executor.called {
		t.Fatalf("expected injected executor to be called")
	}
	if len(executor.plan.Operations) != 1 {
		t.Fatalf("expected one operation in recorded plan, got %d", len(executor.plan.Operations))
	}
}

func TestSelectionBatchActionExecuteFallsBackToDefaultExecutor(t *testing.T) {
	action := selectionBatchAction{
		command:    selectionCommandDismissDelete,
		operations: []selectionOperation{{kind: selectionOperationDismissSession, sessionID: "s1"}},
	}
	if cmd := action.Execute(nil); cmd != nil {
		t.Fatalf("expected nil command for nil model execution context")
	}
}

func TestSelectionOperationExecuteBranches(t *testing.T) {
	m := NewModel(nil)
	ops := []selectionOperation{
		{kind: selectionOperationDeleteWorkspace, workspaceID: "ws1"},
		{kind: selectionOperationDeleteWorktree, workspaceID: "ws1", worktreeID: "wt1"},
		{kind: selectionOperationDismissSession, sessionID: "s1"},
		{kind: selectionOperationDismissWorkflow, runID: "gwf-1"},
		{kind: selectionOperationInterruptSession, sessionID: "s1"},
		{kind: selectionOperationStopWorkflow, runID: "gwf-1"},
		{kind: selectionOperationKillSession, sessionID: "s1"},
	}
	for _, op := range ops {
		if cmd := op.execute(&m); cmd == nil {
			t.Fatalf("expected non-nil cmd for operation kind %v", op.kind)
		}
	}
	if cmd := (selectionOperation{kind: selectionOperationKind(99)}).execute(&m); cmd != nil {
		t.Fatalf("expected nil cmd for unknown operation kind")
	}
	if cmd := (selectionOperation{kind: selectionOperationDismissSession}).execute(nil); cmd != nil {
		t.Fatalf("expected nil cmd for nil model")
	}
}

func TestSelectionCommandProfileMethodsCoverFallbackBranches(t *testing.T) {
	profile := selectionCommandProfile{
		kind:              selectionCommandKill,
		noActionableErr:   " no-actionable ",
		emptySelectionErr: " empty ",
	}
	if profile.Kind() != selectionCommandKill {
		t.Fatalf("unexpected kind: %v", profile.Kind())
	}
	if got := profile.NoActionableError(); got != "no-actionable" {
		t.Fatalf("unexpected no-actionable error: %q", got)
	}
	if got := profile.EmptySelectionError(); got != "empty" {
		t.Fatalf("unexpected empty-selection error: %q", got)
	}
	if got := profile.ExecutionStatus(3); got != "processing 3 item(s)" {
		t.Fatalf("unexpected fallback status: %q", got)
	}
}

func TestDefaultSelectionOperationPlannerNoActionableAndDeduped(t *testing.T) {
	planner := NewDefaultSelectionOperationPlanner()
	noopItems := []*sidebarItem{
		{kind: sidebarWorkspace, workspace: &types.Workspace{ID: "ws1", Name: "Workspace"}},
	}
	_, err := planner.Plan(selectionCommandKill, noopItems, nil)
	if err == nil || err.Error() != "selection has no killable items" {
		t.Fatalf("expected no-actionable kill error, got %v", err)
	}

	dupItems := []*sidebarItem{
		{kind: sidebarSession, session: &types.Session{ID: "s1", Status: types.SessionStatusRunning}},
		{kind: sidebarSession, session: &types.Session{ID: "s1", Status: types.SessionStatusRunning}},
	}
	plan, err := planner.Plan(selectionCommandDismissDelete, dupItems, nil)
	if err != nil {
		t.Fatalf("unexpected planner error for duplicate items: %v", err)
	}
	if len(plan.Operations) != 1 {
		t.Fatalf("expected duplicate operations to be deduped, got %d", len(plan.Operations))
	}
}

func TestDefaultSelectionConfirmationPresenterIncludesSkippedAndTruncation(t *testing.T) {
	presenter := NewDefaultSelectionConfirmationPresenter()
	ops := make([]selectionOperation, 0, 22)
	for i := 0; i < 22; i++ {
		ops = append(ops, selectionOperation{
			kind:  selectionOperationDismissSession,
			label: fmt.Sprintf("Session %02d", i+1),
		})
	}
	spec := presenter.ConfirmSpec(SelectionOperationPlan{
		Command:      selectionCommandDismissDelete,
		Operations:   ops,
		SkippedCount: 2,
	})
	if !strings.Contains(spec.message, "...and 2 more") {
		t.Fatalf("expected truncation suffix in message, got %q", spec.message)
	}
	if !strings.Contains(spec.message, "Skipped 2 selected item(s) that are not applicable.") {
		t.Fatalf("expected skipped-count note in message, got %q", spec.message)
	}
}

func TestWorkflowStatusForPlanningUsesItemAndContextFallback(t *testing.T) {
	if got := workflowStatusForPlanning(nil, nil); got != "" {
		t.Fatalf("expected empty status for nil item, got %q", got)
	}
	itemWithStatus := &sidebarItem{
		kind:     sidebarWorkflow,
		workflow: &guidedworkflows.WorkflowRun{ID: "gwf-1", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	if got := workflowStatusForPlanning(itemWithStatus, fixedWorkflowPlanningContext{status: guidedworkflows.WorkflowRunStatusStopped, ok: true}); got != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected item status precedence, got %q", got)
	}
	itemWithoutStatus := &sidebarItem{
		kind:       sidebarWorkflow,
		workflowID: "gwf-2",
	}
	if got := workflowStatusForPlanning(itemWithoutStatus, fixedWorkflowPlanningContext{status: guidedworkflows.WorkflowRunStatusStopped, ok: true}); got != guidedworkflows.WorkflowRunStatusStopped {
		t.Fatalf("expected context fallback status, got %q", got)
	}
	if got := workflowStatusForPlanning(itemWithoutStatus, fixedWorkflowPlanningContext{ok: false}); got != "" {
		t.Fatalf("expected empty status for missing context status, got %q", got)
	}
}

func TestSelectionModelPlanningAndExecutionContexts(t *testing.T) {
	emptyCtx := emptySelectionOperationPlanningContext{}
	if status, ok := emptyCtx.WorkflowRunStatus("gwf-1"); ok || status != "" {
		t.Fatalf("expected empty context to return no status, got %q / %v", status, ok)
	}

	m := NewModel(nil)
	m.workflowRunStatusIndex["gwf-1"] = guidedworkflows.WorkflowRunStatusRunning
	planningCtx := modelSelectionOperationPlanningContext{model: &m}
	if status, ok := planningCtx.WorkflowRunStatus(" gwf-1 "); !ok || status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected model planning context status, got %q / %v", status, ok)
	}
	if status, ok := (modelSelectionOperationPlanningContext{}).WorkflowRunStatus("gwf-1"); ok || status != "" {
		t.Fatalf("expected nil-model planning context to return no status, got %q / %v", status, ok)
	}

	execCtx := modelSelectionOperationExecutionContext{model: &m}
	ops := []selectionOperation{
		{kind: selectionOperationDeleteWorkspace, workspaceID: "ws1"},
		{kind: selectionOperationDeleteWorktree, workspaceID: "ws1", worktreeID: "wt1"},
		{kind: selectionOperationDismissSession, sessionID: "s1"},
		{kind: selectionOperationDismissWorkflow, runID: "gwf-1"},
		{kind: selectionOperationInterruptSession, sessionID: "s1"},
		{kind: selectionOperationStopWorkflow, runID: "gwf-1"},
		{kind: selectionOperationKillSession, sessionID: "s1"},
	}
	for _, op := range ops {
		if cmd := execCtx.CommandForSelectionOperation(op); cmd == nil {
			t.Fatalf("expected non-nil command for op %v", op.kind)
		}
	}
	if cmd := execCtx.CommandForSelectionOperation(selectionOperation{kind: selectionOperationKind(999)}); cmd != nil {
		t.Fatalf("expected nil command for unknown op kind")
	}
	if cmd := (modelSelectionOperationExecutionContext{}).CommandForSelectionOperation(selectionOperation{kind: selectionOperationKillSession}); cmd != nil {
		t.Fatalf("expected nil command when model is nil")
	}
	execCtx.SetSelectionOperationStatus("")
	execCtx.SetSelectionOperationStatus("status-from-exec-context")
	if m.status != "status-from-exec-context" {
		t.Fatalf("expected status update from execution context, got %q", m.status)
	}
}

func TestSelectionOperationModelOptionsInstallAndResetDefaults(t *testing.T) {
	customPlanner := noopSelectionOperationPlanner{}
	customPresenter := noopSelectionConfirmationPresenter{spec: selectionActionConfirmSpec{title: "Custom"}}
	customExecutor := &recordingSelectionOperationExecutor{}
	m := NewModel(nil,
		WithSelectionOperationPlanner(customPlanner),
		WithSelectionConfirmationPresenter(customPresenter),
		WithSelectionOperationExecutor(customExecutor),
	)
	if _, ok := m.selectionOperationPlanner.(noopSelectionOperationPlanner); !ok {
		t.Fatalf("expected custom planner installed, got %T", m.selectionOperationPlanner)
	}
	if _, ok := m.selectionConfirmationPresenter.(noopSelectionConfirmationPresenter); !ok {
		t.Fatalf("expected custom presenter installed, got %T", m.selectionConfirmationPresenter)
	}
	if m.selectionOperationExecutor != customExecutor {
		t.Fatalf("expected custom executor installed")
	}

	m2 := NewModel(nil,
		WithSelectionOperationPlanner(nil),
		WithSelectionConfirmationPresenter(nil),
		WithSelectionOperationExecutor(nil),
	)
	if m2.selectionOperationPlanner == nil || m2.selectionConfirmationPresenter == nil || m2.selectionOperationExecutor == nil {
		t.Fatalf("expected nil options to reset to defaults")
	}

	WithSelectionOperationPlanner(customPlanner)(nil)
	WithSelectionConfirmationPresenter(customPresenter)(nil)
	WithSelectionOperationExecutor(customExecutor)(nil)
}
