package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) sidebarSortStripAvailable() bool {
	if m == nil || m.sidebar == nil {
		return false
	}
	return m.sortStripVisibilityPolicyOrDefault().ShowStrip(m.mode, m.appState.SidebarCollapsed)
}

func (m *Model) reduceSidebarSortStripKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.sidebarSortStripAvailable() {
		return false, nil
	}
	if m.input != nil && !m.input.IsSidebarFocused() {
		return false, nil
	}
	if m.keyMatchesCommand(msg, KeyCommandSidebarFilter, "ctrl+f") {
		return true, m.toggleSidebarFilter()
	}
	if m.keyMatchesCommand(msg, KeyCommandSidebarSortReverse, "alt+r") {
		return true, m.applySidebarSortStripAction(sidebarSortStripActionReverse)
	}
	switch msg.String() {
	case "left":
		if item := m.selectedItem(); item != nil && item.collapsible {
			return false, nil
		}
		return true, m.applySidebarSortStripAction(sidebarSortStripActionSortPrev)
	case "right":
		if item := m.selectedItem(); item != nil && item.collapsible {
			return false, nil
		}
		return true, m.applySidebarSortStripAction(sidebarSortStripActionSortNext)
	}
	return false, nil
}

func (m *Model) reduceSidebarFilterInput(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.sidebarSortStripAvailable() || !m.sidebarFilterActive {
		return false, nil
	}
	if m.input != nil && !m.input.IsSidebarFocused() {
		return false, nil
	}
	key := msg.String()
	changed := false
	switch key {
	case "esc":
		m.sidebarFilterActive = false
		m.sidebarFilterQuery = ""
		changed = true
	case "enter":
		m.sidebarFilterActive = false
		changed = true
	case "backspace", "ctrl+h":
		if len(m.sidebarFilterQuery) > 0 {
			m.sidebarFilterQuery = m.sidebarFilterQuery[:len(m.sidebarFilterQuery)-1]
			changed = true
		}
	case "space":
		m.sidebarFilterQuery += " "
		changed = true
	default:
		if text := msg.String(); len([]rune(text)) == 1 {
			m.sidebarFilterQuery += text
			changed = true
		}
	}
	if !changed {
		return true, nil
	}
	if m.sidebar != nil {
		m.sidebar.MarkSortStripFocus(sidebarSortStripSegmentFilter)
	}
	m.updateDelegate()
	return true, nil
}

func (m *Model) toggleSidebarFilter() tea.Cmd {
	if m == nil {
		return nil
	}
	if m.sidebarFilterActive {
		m.sidebarFilterActive = false
		m.sidebarFilterQuery = ""
	} else {
		m.sidebarFilterActive = true
		m.sidebarFilterQuery = strings.TrimSpace(m.sidebarFilterQuery)
	}
	if m.sidebar != nil {
		m.sidebar.MarkSortStripFocus(sidebarSortStripSegmentFilter)
	}
	m.updateDelegate()
	return nil
}

func (m *Model) applySidebarSortStripAction(action sidebarSortStripAction) tea.Cmd {
	if m == nil || !m.sidebarSortStripAvailable() {
		return nil
	}
	switch action {
	case sidebarSortStripActionFilter:
		return m.toggleSidebarFilter()
	case sidebarSortStripActionReverse:
		m.sidebarSort = m.sidebarSortPolicyOrDefault().ToggleReverse(m.sidebarSort)
		if m.sidebar != nil {
			m.sidebar.MarkSortStripFocus(sidebarSortStripSegmentReverse)
		}
		m.updateDelegate()
		m.syncAppStateSidebarSort()
		return m.requestAppStateSaveCmd()
	case sidebarSortStripActionSortPrev:
		m.sidebarSort = m.sidebarSortPolicyOrDefault().Cycle(m.sidebarSort, -1)
		if m.sidebar != nil {
			m.sidebar.MarkSortStripFocus(sidebarSortStripSegmentSortKey)
		}
		m.updateDelegate()
		m.syncAppStateSidebarSort()
		return m.requestAppStateSaveCmd()
	case sidebarSortStripActionSortNext:
		m.sidebarSort = m.sidebarSortPolicyOrDefault().Cycle(m.sidebarSort, 1)
		if m.sidebar != nil {
			m.sidebar.MarkSortStripFocus(sidebarSortStripSegmentSortKey)
		}
		m.updateDelegate()
		m.syncAppStateSidebarSort()
		return m.requestAppStateSaveCmd()
	default:
		return nil
	}
}
