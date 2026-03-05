package app

import tea "charm.land/bubbletea/v2"

func (m *Model) contextPanelEnabled() bool {
	if m == nil {
		return false
	}
	return !m.appState.ContextPanelHidden
}

func (m *Model) setContextPanelHidden(hidden bool) bool {
	if m == nil {
		return false
	}
	if m.appState.ContextPanelHidden == hidden {
		return false
	}
	m.appState.ContextPanelHidden = hidden
	m.hasAppState = true
	return true
}

func contextPanelToggleStatus(hidden bool) string {
	if hidden {
		return "context panel hidden"
	}
	return "context panel shown"
}

func (m *Model) toggleContextPanel() tea.Cmd {
	if m == nil {
		return nil
	}
	nextHidden := !m.appState.ContextPanelHidden
	if !m.setContextPanelHidden(nextHidden) {
		return nil
	}
	m.setStatusMessage(contextPanelToggleStatus(nextHidden))
	m.resize(m.width, m.height)
	return m.requestAppStateSaveCmd()
}
