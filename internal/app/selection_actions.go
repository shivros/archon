package app

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type selectionActionConfirmSpec struct {
	title       string
	message     string
	confirmText string
	cancelText  string
}

type selectionAction interface {
	Validate(*Model) error
	ConfirmSpec(*Model) selectionActionConfirmSpec
	Execute(*Model) tea.Cmd
}

type deleteWorkspaceSelectionAction struct {
	workspaceID string
}

func (a deleteWorkspaceSelectionAction) Validate(_ *Model) error {
	if strings.TrimSpace(a.workspaceID) == "" || strings.TrimSpace(a.workspaceID) == unassignedWorkspaceID {
		return errors.New("select a workspace to delete")
	}
	return nil
}

func (a deleteWorkspaceSelectionAction) ConfirmSpec(m *Model) selectionActionConfirmSpec {
	message := "Delete workspace?"
	if m != nil {
		if ws := m.workspaceByID(a.workspaceID); ws != nil && strings.TrimSpace(ws.Name) != "" {
			message = fmt.Sprintf("Delete workspace %q?", ws.Name)
		}
	}
	return selectionActionConfirmSpec{
		title:       "Delete Workspace",
		message:     message,
		confirmText: "Delete",
		cancelText:  "Cancel",
	}
}

func (a deleteWorkspaceSelectionAction) Execute(m *Model) tea.Cmd {
	if m == nil {
		return nil
	}
	m.setStatusMessage("deleting workspace")
	return deleteWorkspaceCmd(m.workspaceAPI, a.workspaceID)
}

type deleteWorktreeSelectionAction struct {
	workspaceID string
	worktreeID  string
}

func (a deleteWorktreeSelectionAction) Validate(_ *Model) error {
	if strings.TrimSpace(a.workspaceID) == "" || strings.TrimSpace(a.worktreeID) == "" {
		return errors.New("select a worktree to delete")
	}
	return nil
}

func (a deleteWorktreeSelectionAction) ConfirmSpec(m *Model) selectionActionConfirmSpec {
	message := "Delete worktree?"
	if m != nil {
		if wt := m.worktreeByID(a.worktreeID); wt != nil && strings.TrimSpace(wt.Name) != "" {
			message = fmt.Sprintf("Delete worktree %q?", wt.Name)
		}
	}
	return selectionActionConfirmSpec{
		title:       "Delete Worktree",
		message:     message,
		confirmText: "Delete",
		cancelText:  "Cancel",
	}
}

func (a deleteWorktreeSelectionAction) Execute(m *Model) tea.Cmd {
	if m == nil {
		return nil
	}
	m.setStatusMessage("deleting worktree")
	return deleteWorktreeCmd(m.workspaceAPI, a.workspaceID, a.worktreeID)
}

type dismissSessionSelectionAction struct {
	sessionID string
}

func (a dismissSessionSelectionAction) Validate(_ *Model) error {
	if strings.TrimSpace(a.sessionID) == "" {
		return errors.New("select a session to dismiss")
	}
	return nil
}

func (a dismissSessionSelectionAction) ConfirmSpec(_ *Model) selectionActionConfirmSpec {
	return selectionActionConfirmSpec{
		title:       "Dismiss Sessions",
		message:     "Dismiss session?",
		confirmText: "Dismiss",
		cancelText:  "Cancel",
	}
}

func (a dismissSessionSelectionAction) Execute(m *Model) tea.Cmd {
	if m == nil {
		return nil
	}
	m.setStatusMessage("dismissing " + a.sessionID)
	return dismissSessionCmd(m.sessionAPI, a.sessionID)
}

type dismissWorkflowSelectionAction struct {
	runID string
}

func (a dismissWorkflowSelectionAction) Validate(_ *Model) error {
	if strings.TrimSpace(a.runID) == "" {
		return errors.New("select a workflow to dismiss")
	}
	return nil
}

func (a dismissWorkflowSelectionAction) ConfirmSpec(_ *Model) selectionActionConfirmSpec {
	return selectionActionConfirmSpec{
		title:       "Dismiss Workflow",
		message:     "Dismiss workflow?",
		confirmText: "Dismiss",
		cancelText:  "Cancel",
	}
}

func (a dismissWorkflowSelectionAction) Execute(m *Model) tea.Cmd {
	if m == nil {
		return nil
	}
	m.setStatusMessage("dismissing workflow " + a.runID)
	return dismissWorkflowRunCmd(m.guidedWorkflowAPI, a.runID)
}

func resolveDismissOrDeleteSelectionAction(item *sidebarItem) (selectionAction, error) {
	if item == nil {
		return nil, errors.New("select an item to dismiss or delete")
	}
	switch item.kind {
	case sidebarWorkspace:
		if item.workspace == nil {
			return nil, errors.New("select a workspace to delete")
		}
		return deleteWorkspaceSelectionAction{workspaceID: item.workspace.ID}, nil
	case sidebarWorktree:
		if item.worktree == nil {
			return nil, errors.New("select a worktree to delete")
		}
		return deleteWorktreeSelectionAction{
			workspaceID: item.worktree.WorkspaceID,
			worktreeID:  item.worktree.ID,
		}, nil
	case sidebarSession:
		if item.session == nil {
			return nil, errors.New("select a session to dismiss")
		}
		return dismissSessionSelectionAction{sessionID: item.session.ID}, nil
	case sidebarWorkflow:
		return dismissWorkflowSelectionAction{runID: item.workflowRunID()}, nil
	default:
		return nil, errors.New("select an item to dismiss or delete")
	}
}

func (m *Model) openSelectionActionConfirm(action selectionAction) {
	if m == nil {
		return
	}
	if action == nil {
		m.setValidationStatus("select an item to dismiss or delete")
		return
	}
	if m.confirm == nil {
		return
	}
	if err := action.Validate(m); err != nil {
		m.setValidationStatus(err.Error())
		return
	}
	spec := action.ConfirmSpec(m)
	if strings.TrimSpace(spec.title) == "" {
		spec.title = "Confirm"
	}
	if strings.TrimSpace(spec.message) == "" {
		spec.message = "Are you sure?"
	}
	if strings.TrimSpace(spec.confirmText) == "" {
		spec.confirmText = "Confirm"
	}
	if strings.TrimSpace(spec.cancelText) == "" {
		spec.cancelText = "Cancel"
	}
	m.pendingSelectionAction = action
	m.pendingConfirm = confirmAction{}
	if m.menu != nil {
		m.menu.CloseAll()
	}
	if m.contextMenu != nil {
		m.contextMenu.Close()
	}
	m.confirm.Open(spec.title, spec.message, spec.confirmText, spec.cancelText)
}
