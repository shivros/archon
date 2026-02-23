package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type SelectionKind int

const (
	SelectionKindUnknown SelectionKind = iota
	SelectionKindWorkspace
	SelectionKindWorktree
	SelectionKindWorkflow
	SelectionKindSession
)

type SelectionTarget struct {
	Kind          SelectionKind
	WorkspaceID   string
	WorktreeID    string
	WorkflowRunID string
	SessionID     string
	Collapsible   bool
}

func selectionTargetFromSidebarItem(item *sidebarItem) SelectionTarget {
	if item == nil {
		return SelectionTarget{}
	}
	target := SelectionTarget{
		Collapsible: item.collapsible,
		WorkspaceID: strings.TrimSpace(item.workspaceID()),
	}
	switch item.kind {
	case sidebarWorkspace:
		target.Kind = SelectionKindWorkspace
	case sidebarWorktree:
		target.Kind = SelectionKindWorktree
		if item.worktree != nil {
			target.WorktreeID = strings.TrimSpace(item.worktree.ID)
		}
	case sidebarWorkflow:
		target.Kind = SelectionKindWorkflow
		target.WorkflowRunID = strings.TrimSpace(item.workflowRunID())
		if item.workflow != nil {
			if target.WorkspaceID == "" {
				target.WorkspaceID = strings.TrimSpace(item.workflow.WorkspaceID)
			}
			target.WorktreeID = strings.TrimSpace(item.workflow.WorktreeID)
			target.SessionID = strings.TrimSpace(item.workflow.SessionID)
		}
	case sidebarSession:
		target.Kind = SelectionKindSession
		if item.session != nil {
			target.SessionID = strings.TrimSpace(item.session.ID)
		}
		if item.meta != nil {
			target.WorktreeID = strings.TrimSpace(item.meta.WorktreeID)
		}
	default:
		target.Kind = SelectionKindUnknown
	}
	return target
}

func (t SelectionTarget) containerToggleEligible() bool {
	if !t.Collapsible {
		return false
	}
	return t.Kind == SelectionKindWorkspace || t.Kind == SelectionKindWorktree
}

type SelectionActivationContext interface {
	OpenWorkflow(runID string) tea.Cmd
	OpenSessionCompose(sessionID string)
}

type SelectionActivator interface {
	Kind() SelectionKind
	Activate(target SelectionTarget, context SelectionActivationContext) (handled bool, cmd tea.Cmd)
}

type SelectionActivationService interface {
	ActivateSelection(target SelectionTarget, context SelectionActivationContext) (handled bool, cmd tea.Cmd)
}

type registrySelectionActivationService struct {
	activators map[SelectionKind]SelectionActivator
}

type workflowSelectionActivator struct{}

func (workflowSelectionActivator) Kind() SelectionKind {
	return SelectionKindWorkflow
}

func (workflowSelectionActivator) Activate(target SelectionTarget, context SelectionActivationContext) (bool, tea.Cmd) {
	if context == nil {
		return false, nil
	}
	return true, context.OpenWorkflow(strings.TrimSpace(target.WorkflowRunID))
}

type sessionSelectionActivator struct{}

func (sessionSelectionActivator) Kind() SelectionKind {
	return SelectionKindSession
}

func (sessionSelectionActivator) Activate(target SelectionTarget, context SelectionActivationContext) (bool, tea.Cmd) {
	if context == nil {
		return false, nil
	}
	sessionID := strings.TrimSpace(target.SessionID)
	if sessionID == "" {
		return true, nil
	}
	context.OpenSessionCompose(sessionID)
	return true, nil
}

func defaultSelectionActivators() []SelectionActivator {
	return []SelectionActivator{
		workflowSelectionActivator{},
		sessionSelectionActivator{},
	}
}

func NewSelectionActivationService(activators ...SelectionActivator) SelectionActivationService {
	registry := map[SelectionKind]SelectionActivator{}
	for _, activator := range activators {
		if activator == nil {
			continue
		}
		registry[activator.Kind()] = activator
	}
	return registrySelectionActivationService{activators: registry}
}

func NewDefaultSelectionActivationService() SelectionActivationService {
	return NewSelectionActivationService(defaultSelectionActivators()...)
}

func (s registrySelectionActivationService) ActivateSelection(target SelectionTarget, context SelectionActivationContext) (bool, tea.Cmd) {
	if context == nil {
		return false, nil
	}
	activator, ok := s.activators[target.Kind]
	if !ok || activator == nil {
		return false, nil
	}
	return activator.Activate(target, context)
}

type SelectionEnterActionContext interface {
	ToggleSelectedContainer() bool
	SyncSidebarExpansionChange() tea.Cmd
	ActivateSelection(target SelectionTarget) (handled bool, cmd tea.Cmd)
	SetValidationStatus(message string)
}

type SelectionEnterActionService interface {
	HandleEnter(target SelectionTarget, context SelectionEnterActionContext) (handled bool, cmd tea.Cmd)
}

type defaultSelectionEnterActionService struct {
	missingSelectionMessage string
}

func NewDefaultSelectionEnterActionService() SelectionEnterActionService {
	return defaultSelectionEnterActionService{
		missingSelectionMessage: "select a session to chat",
	}
}

func (s defaultSelectionEnterActionService) HandleEnter(target SelectionTarget, context SelectionEnterActionContext) (bool, tea.Cmd) {
	if context == nil {
		return false, nil
	}
	if target.containerToggleEligible() {
		if context.ToggleSelectedContainer() {
			return true, context.SyncSidebarExpansionChange()
		}
		return true, nil
	}
	if handled, cmd := context.ActivateSelection(target); handled {
		return true, cmd
	}
	message := strings.TrimSpace(s.missingSelectionMessage)
	if message != "" {
		context.SetValidationStatus(message)
	}
	return true, nil
}

func WithSelectionActivationService(service SelectionActivationService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if service == nil {
			m.selectionActivationService = NewDefaultSelectionActivationService()
			return
		}
		m.selectionActivationService = service
	}
}

func (m *Model) selectionActivationServiceOrDefault() SelectionActivationService {
	if m == nil || m.selectionActivationService == nil {
		return NewDefaultSelectionActivationService()
	}
	return m.selectionActivationService
}

func WithSelectionEnterActionService(service SelectionEnterActionService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if service == nil {
			m.selectionEnterActionService = NewDefaultSelectionEnterActionService()
			return
		}
		m.selectionEnterActionService = service
	}
}

func (m *Model) selectionEnterActionServiceOrDefault() SelectionEnterActionService {
	if m == nil || m.selectionEnterActionService == nil {
		return NewDefaultSelectionEnterActionService()
	}
	return m.selectionEnterActionService
}
