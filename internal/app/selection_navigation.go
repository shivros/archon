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

func NewDefaultSelectionTransitionService() SelectionTransitionService {
	return defaultSelectionTransitionService{}
}

func (defaultSelectionTransitionService) SelectionChanged(m *Model, delay time.Duration, source selectionChangeSource) tea.Cmd {
	if m == nil {
		return nil
	}
	m.trackSelectionHistory(source)
	item := m.selectedItem()
	handled, stateChanged, draftChanged := m.applySelectionState(item)
	var cmd tea.Cmd
	if !handled {
		cmd = m.scheduleSessionLoad(item, delay)
	} else if m.mode == uiModeRecents {
		cmd = m.ensureRecentsPreviewForSelection()
	}
	if stateChanged || draftChanged {
		save := m.requestAppStateSaveCmd()
		if cmd != nil && save != nil {
			cmd = tea.Batch(cmd, save)
		} else if save != nil {
			cmd = save
		}
	}
	return m.batchWithNotesPanelSync(cmd)
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
