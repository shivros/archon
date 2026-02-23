package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type modelSelectionActivationContext struct {
	model *Model
}

func newModelSelectionActivationContext(model *Model) SelectionActivationContext {
	return modelSelectionActivationContext{model: model}
}

func (c modelSelectionActivationContext) OpenWorkflow(runID string) tea.Cmd {
	if c.model == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		c.model.setValidationStatus("select a workflow")
		return nil
	}
	item := c.model.selectedItem()
	if item == nil || item.kind != sidebarWorkflow || strings.TrimSpace(item.workflowRunID()) != runID {
		if c.model.sidebar != nil {
			c.model.sidebar.SelectByWorkflowID(runID)
		}
		item = c.model.selectedItem()
	}
	if item == nil || item.kind != sidebarWorkflow {
		c.model.setValidationStatus("workflow not found in sidebar")
		return nil
	}
	return c.model.openGuidedWorkflowFromSidebar(item)
}

func (c modelSelectionActivationContext) OpenSessionCompose(sessionID string) {
	if c.model == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.model.enterCompose(sessionID)
}

type modelSelectionEnterActionContext struct {
	model *Model
}

func newModelSelectionEnterActionContext(model *Model) SelectionEnterActionContext {
	return modelSelectionEnterActionContext{model: model}
}

func (c modelSelectionEnterActionContext) ToggleSelectedContainer() bool {
	if c.model == nil || c.model.sidebar == nil {
		return false
	}
	return c.model.sidebar.ToggleSelectedContainer()
}

func (c modelSelectionEnterActionContext) SyncSidebarExpansionChange() tea.Cmd {
	if c.model == nil {
		return nil
	}
	return c.model.syncSidebarExpansionChange()
}

func (c modelSelectionEnterActionContext) ActivateSelection(target SelectionTarget) (bool, tea.Cmd) {
	if c.model == nil {
		return false, nil
	}
	return c.model.activateSelectionTarget(target)
}

func (c modelSelectionEnterActionContext) SetValidationStatus(message string) {
	if c.model == nil {
		return
	}
	c.model.setValidationStatus(message)
}
