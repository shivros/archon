package app

import (
	"errors"
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
	if m.status != "select an item to dismiss or delete" {
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
