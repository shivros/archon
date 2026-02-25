package app

import tea "charm.land/bubbletea/v2"

func (m *Model) debugPanelNavigable() bool {
	return m != nil &&
		m.appState.DebugStreamsEnabled &&
		m.debugPanelVisible &&
		m.debugPanel != nil
}

func (m *Model) handleDebugPanelScrollKey(msg tea.KeyMsg) bool {
	if !m.debugPanelNavigable() {
		return false
	}
	switch {
	case m.keyMatchesCommand(msg, KeyCommandDebugPanelDown, "J") || m.keyString(msg) == "shift+down":
		return m.debugPanel.ScrollDown(1)
	case m.keyMatchesCommand(msg, KeyCommandDebugPanelUp, "K") || m.keyString(msg) == "shift+up":
		return m.debugPanel.ScrollUp(1)
	case m.keyMatchesCommand(msg, KeyCommandDebugPanelLeft, "H") || m.keyString(msg) == "shift+left":
		return m.debugPanel.ScrollLeft(4)
	case m.keyMatchesCommand(msg, KeyCommandDebugPanelRight, "L") || m.keyString(msg) == "shift+right":
		return m.debugPanel.ScrollRight(4)
	case m.keyMatchesCommand(msg, KeyCommandDebugPanelPageDown, "shift+pgdown"):
		return m.debugPanel.PageDown()
	case m.keyMatchesCommand(msg, KeyCommandDebugPanelPageUp, "shift+pgup"):
		return m.debugPanel.PageUp()
	case m.keyMatchesCommand(msg, KeyCommandDebugPanelTop, "shift+home"):
		return m.debugPanel.GotoTop()
	case m.keyMatchesCommand(msg, KeyCommandDebugPanelBottom, "shift+end"):
		return m.debugPanel.GotoBottom()
	default:
		return false
	}
}

func (m *Model) reduceDebugPanelWheelMouse(msg tea.MouseMsg, layout mouseLayout, delta int) bool {
	if !m.debugPanelNavigable() {
		return false
	}
	if !layout.panelVisible || layout.panelWidth <= 0 {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.panelStart || mouse.X >= layout.panelStart+layout.panelWidth {
		return false
	}
	if mouse.Y < 0 || mouse.Y > m.debugPanelHeight() {
		return false
	}
	if delta < 0 {
		return m.debugPanel.ScrollUp(3)
	}
	return m.debugPanel.ScrollDown(3)
}

func (m *Model) debugPanelHeight() int {
	if m == nil || m.debugPanel == nil {
		return 0
	}
	return m.debugPanel.Height()
}
