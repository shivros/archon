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

func (m *Model) reduceDebugPanelLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	if !m.debugPanelNavigable() {
		return false
	}
	if !layout.panelVisible || layout.panelWidth <= 0 || m.debugPanelHeight() <= 0 {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.panelStart || mouse.X >= layout.panelStart+layout.panelWidth {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.debugPanelHeight() {
		return false
	}
	interaction := m.debugPanelInteractionService
	if interaction == nil {
		interaction = NewDefaultDebugPanelInteractionService()
		m.debugPanelInteractionService = interaction
	}
	hit, ok := interaction.HitTest(m.debugPanelSpans, m.debugPanel.YOffset(), mouse.X-layout.panelStart, mouse.Y-1)
	if !ok {
		return false
	}
	if cmd := m.applyDebugPanelControl(hit); cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return true
}

func (m *Model) debugPanelHeight() int {
	if m == nil || m.debugPanel == nil {
		return 0
	}
	return m.debugPanel.Height()
}
