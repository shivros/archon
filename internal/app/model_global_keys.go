package app

import tea "charm.land/bubbletea/v2"

type globalKeyOptions struct {
	AllowMenu          bool
	AllowSettings      bool
	AllowToggleSidebar bool
	AllowToggleNotes   bool
	AllowToggleContext bool
	AllowToggleDebug   bool
}

func (m *Model) reduceGlobalKey(msg tea.KeyMsg, opts globalKeyOptions) (bool, tea.Cmd) {
	if opts.AllowMenu && m.keyMatchesCommand(msg, KeyCommandMenu, "ctrl+m") {
		if m.menu != nil {
			if m.contextMenu != nil {
				m.contextMenu.Close()
			}
			m.menu.Toggle()
		}
		return true, nil
	}
	if opts.AllowToggleSidebar && m.keyMatchesCommand(msg, KeyCommandToggleSidebar, "ctrl+b") {
		m.toggleSidebar()
		return true, m.requestAppStateSaveCmd()
	}
	if opts.AllowToggleNotes && m.keyMatchesCommand(msg, KeyCommandToggleNotesPanel, "ctrl+o") {
		return true, m.toggleNotesPanel()
	}
	if opts.AllowToggleContext && m.keyMatchesCommand(msg, KeyCommandToggleContextPanel, "ctrl+l") {
		return true, m.toggleContextPanel()
	}
	if opts.AllowToggleDebug && m.keyMatchesCommand(msg, KeyCommandToggleDebugStreams, "ctrl+d") {
		return true, m.toggleDebugStreams()
	}
	if opts.AllowSettings && m.keyMatchesCommandStrict(msg, KeyCommandOpenSettings, "esc") {
		m.openSettingsMenu()
		return true, nil
	}
	return false, nil
}
