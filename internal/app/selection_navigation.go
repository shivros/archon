package app

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type selectionChangeSource int

const (
	selectionChangeSourceUser selectionChangeSource = iota
	selectionChangeSourceSystem
	selectionChangeSourceHistory
)

type SelectionHistoryAction int

const (
	SelectionHistoryActionIgnore SelectionHistoryAction = iota
	SelectionHistoryActionVisit
	SelectionHistoryActionSyncCurrent
)

type SelectionOriginPolicy interface {
	HistoryActionForSource(selectionChangeSource) SelectionHistoryAction
}

type selectionOriginPolicyMap struct {
	bySource map[selectionChangeSource]SelectionHistoryAction
	fallback SelectionHistoryAction
}

func NewSelectionOriginPolicy(actions map[selectionChangeSource]SelectionHistoryAction, fallback SelectionHistoryAction) SelectionOriginPolicy {
	bySource := map[selectionChangeSource]SelectionHistoryAction{}
	for source, action := range actions {
		bySource[source] = action
	}
	return selectionOriginPolicyMap{
		bySource: bySource,
		fallback: fallback,
	}
}

func DefaultSelectionOriginPolicy() SelectionOriginPolicy {
	return NewSelectionOriginPolicy(map[selectionChangeSource]SelectionHistoryAction{
		selectionChangeSourceUser:    SelectionHistoryActionVisit,
		selectionChangeSourceSystem:  SelectionHistoryActionSyncCurrent,
		selectionChangeSourceHistory: SelectionHistoryActionSyncCurrent,
	}, SelectionHistoryActionSyncCurrent)
}

func (p selectionOriginPolicyMap) HistoryActionForSource(source selectionChangeSource) SelectionHistoryAction {
	if action, ok := p.bySource[source]; ok {
		return action
	}
	return p.fallback
}

type SelectionTransitionService interface {
	SelectionChanged(m *Model, delay time.Duration, source selectionChangeSource) tea.Cmd
}

type defaultSelectionTransitionService struct{}

type selectionTransitionOutcome struct {
	command      tea.Cmd
	stateChanged bool
	draftChanged bool
}

func NewDefaultSelectionTransitionService() SelectionTransitionService {
	return defaultSelectionTransitionService{}
}

func (defaultSelectionTransitionService) SelectionChanged(m *Model, delay time.Duration, source selectionChangeSource) tea.Cmd {
	if m == nil {
		return nil
	}
	service := defaultSelectionTransitionService{}
	m.trackSelectionHistory(source)
	item := m.selectedItem()
	focusPolicy := m.selectionFocusPolicyOrDefault()
	service.applySelectionFocusTransition(m, item, source, focusPolicy)
	outcome := service.resolveSelectionTransitionOutcome(m, item, delay, source, focusPolicy)
	cmd := service.withSelectionStatePersistence(m, outcome)
	return m.batchWithNotesPanelSync(cmd)
}

func (defaultSelectionTransitionService) applySelectionFocusTransition(m *Model, item *sidebarItem, source selectionChangeSource, focusPolicy SelectionFocusPolicy) {
	if m == nil || focusPolicy == nil {
		return
	}
	if focusPolicy.ShouldExitGuidedWorkflowForSessionSelection(m.mode, item, source) {
		m.exitGuidedWorkflow("")
	}
}

func (s defaultSelectionTransitionService) resolveSelectionTransitionOutcome(m *Model, item *sidebarItem, delay time.Duration, source selectionChangeSource, focusPolicy SelectionFocusPolicy) selectionTransitionOutcome {
	handled, stateChanged, draftChanged := m.applySelectionState(item)
	cmd := s.resolveSelectionCommand(m, handled, item, delay, source, focusPolicy)
	return selectionTransitionOutcome{
		command:      cmd,
		stateChanged: stateChanged,
		draftChanged: draftChanged,
	}
}

func (defaultSelectionTransitionService) resolveSelectionCommand(m *Model, handled bool, item *sidebarItem, delay time.Duration, source selectionChangeSource, focusPolicy SelectionFocusPolicy) tea.Cmd {
	switch {
	case handled && focusPolicy != nil && focusPolicy.ShouldOpenWorkflowSelection(item, source):
		return m.openGuidedWorkflowFromSidebar(item)
	case !handled:
		return m.scheduleSessionLoad(item, delay)
	case m.mode == uiModeRecents:
		return m.ensureRecentsPreviewForSelection()
	default:
		return nil
	}
}

func (defaultSelectionTransitionService) withSelectionStatePersistence(m *Model, outcome selectionTransitionOutcome) tea.Cmd {
	if m == nil {
		return outcome.command
	}
	if !outcome.stateChanged && !outcome.draftChanged {
		return outcome.command
	}
	save := m.requestAppStateSaveCmd()
	if outcome.command != nil && save != nil {
		return tea.Batch(outcome.command, save)
	}
	if save != nil {
		return save
	}
	return outcome.command
}

func WithSelectionHistory(history SelectionHistory) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if history == nil {
			m.selectionHistory = NewSelectionHistory(selectionHistoryMaxEntries)
			return
		}
		m.selectionHistory = history
	}
}

func WithSelectionOriginPolicy(policy SelectionOriginPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.selectionOriginPolicy = DefaultSelectionOriginPolicy()
			return
		}
		m.selectionOriginPolicy = policy
	}
}

func WithSelectionTransitionService(service SelectionTransitionService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if service == nil {
			m.selectionTransitionService = NewDefaultSelectionTransitionService()
			return
		}
		m.selectionTransitionService = service
	}
}

func (m *Model) selectionOriginPolicyOrDefault() SelectionOriginPolicy {
	if m == nil || m.selectionOriginPolicy == nil {
		return DefaultSelectionOriginPolicy()
	}
	return m.selectionOriginPolicy
}

func (m *Model) selectionTransitionServiceOrDefault() SelectionTransitionService {
	if m == nil || m.selectionTransitionService == nil {
		return NewDefaultSelectionTransitionService()
	}
	return m.selectionTransitionService
}

func (m *Model) trackSelectionHistory(source selectionChangeSource) {
	if m == nil || m.selectionHistory == nil {
		return
	}
	key := strings.TrimSpace(m.selectedKey())
	if key == "" {
		return
	}
	action := m.selectionOriginPolicyOrDefault().HistoryActionForSource(source)
	switch action {
	case SelectionHistoryActionVisit:
		m.selectionHistory.Visit(key)
	case SelectionHistoryActionSyncCurrent:
		m.selectionHistory.SyncCurrent(key)
	case SelectionHistoryActionIgnore:
		return
	}
}
