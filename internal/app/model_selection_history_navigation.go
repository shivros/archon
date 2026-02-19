package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) reduceMouseBackPress() bool {
	cmd := m.navigateSelectionHistoryBack()
	if cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return true
}

func (m *Model) reduceMouseForwardPress() bool {
	cmd := m.navigateSelectionHistoryForward()
	if cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return true
}

func (m *Model) navigateSelectionHistoryBack() tea.Cmd {
	return m.navigateSelectionHistory(-1)
}

func (m *Model) navigateSelectionHistoryForward() tea.Cmd {
	return m.navigateSelectionHistory(1)
}

func (m *Model) navigateSelectionHistory(direction int) tea.Cmd {
	if m == nil || m.sidebar == nil || m.selectionHistory == nil {
		return nil
	}
	valid := func(key string) bool {
		return m.sidebar.CanSelectKey(key)
	}
	var target string
	var ok bool
	if direction < 0 {
		target, ok = m.selectionHistory.Back(valid)
	} else {
		target, ok = m.selectionHistory.Forward(valid)
	}
	if !ok {
		if direction < 0 {
			m.setStatusMessage("history: at oldest selection")
		} else {
			m.setStatusMessage("history: at newest selection")
		}
		return nil
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	if !m.sidebar.SelectByKey(target) {
		if direction < 0 {
			m.setStatusMessage("history: no previous selection")
		} else {
			m.setStatusMessage("history: no forward selection")
		}
		return nil
	}
	return m.onHistorySelectionChangedImmediate()
}
