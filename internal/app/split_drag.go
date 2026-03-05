package app

import tea "charm.land/bubbletea/v2"

type splitDragTarget int

const (
	splitDragTargetNone splitDragTarget = iota
	splitDragTargetSidebar
	splitDragTargetPanel
)

func (m *Model) reduceSplitDragMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	mouse := msg.Mouse()
	switch msg.(type) {
	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft {
			return false
		}
		return m.beginSplitDrag(mouse, layout)
	case tea.MouseMotionMsg:
		if m.splitDraggingTarget == splitDragTargetNone {
			return false
		}
		changed := false
		switch m.splitDraggingTarget {
		case splitDragTargetSidebar:
			changed = m.applySidebarSplitWidth(mouse.X)
		case splitDragTargetPanel:
			viewportWidth := m.width
			if sidebarWidth := m.sidebarWidth(); sidebarWidth > 0 {
				viewportWidth = max(minViewportWidth, m.width-sidebarWidth-1)
			}
			panelWidth := viewportWidth - (mouse.X - layout.rightStart) - 1
			changed = m.applyPanelSplitWidth(m.splitDraggingPanelMode, viewportWidth, panelWidth)
		}
		if changed {
			m.splitDraggingChanged = true
			m.resize(m.width, m.height)
		}
		return true
	case tea.MouseReleaseMsg:
		if m.splitDraggingTarget == splitDragTargetNone {
			return false
		}
		m.splitDraggingTarget = splitDragTargetNone
		m.splitDraggingPanelMode = sidePanelModeNone
		if m.splitDraggingChanged {
			m.splitDraggingChanged = false
			if cmd := m.requestAppStateSaveCmd(); cmd != nil {
				m.pendingMouseCmd = cmd
			}
		}
		return true
	}
	return false
}

func (m *Model) beginSplitDrag(mouse tea.Mouse, layout mouseLayout) bool {
	if layout.listWidth > 0 && mouse.X == layout.listWidth {
		m.splitDraggingTarget = splitDragTargetSidebar
		m.splitDraggingPanelMode = sidePanelModeNone
		m.splitDraggingChanged = false
		return true
	}
	if layout.panelVisible && layout.panelStart > 0 && mouse.X == layout.panelStart-1 {
		mode := m.activeSidePanelMode()
		if mode == sidePanelModeNone {
			return false
		}
		m.splitDraggingTarget = splitDragTargetPanel
		m.splitDraggingPanelMode = mode
		m.splitDraggingChanged = false
		return true
	}
	return false
}

func (m *Model) applySidebarSplitWidth(width int) bool {
	if m == nil || m.appState.SidebarCollapsed || m.width <= 0 {
		return false
	}
	nextWidth := clampSidebarWidthForTerminal(m.width, width)
	if nextWidth <= 0 {
		return false
	}
	next := captureSplitPreference(m.width, nextWidth, fromAppStateSplit(m.appState.SidebarSplit))
	if splitPreferenceEqual(fromAppStateSplit(m.appState.SidebarSplit), next) {
		return false
	}
	m.appState.SidebarSplit = toAppStateSplit(next)
	m.hasAppState = true
	return true
}

func (m *Model) applyPanelSplitWidth(mode sidePanelMode, viewportWidth, panelWidth int) bool {
	if m == nil || mode == sidePanelModeNone || viewportWidth <= 0 {
		return false
	}
	nextWidth := clampSidePanelWidthForViewport(viewportWidth, panelWidth)
	if nextWidth <= 0 {
		return false
	}
	changed := false
	nextSplit := captureSplitPreference(viewportWidth, nextWidth, fromAppStateSplit(m.appState.MainSideSplit))
	if !splitPreferenceEqual(fromAppStateSplit(m.appState.MainSideSplit), nextSplit) {
		m.appState.MainSideSplit = toAppStateSplit(nextSplit)
		changed = true
	}
	if policy := panelLayoutPersistencePolicy(mode); policy.SetWidth(&m.appState, nextWidth) {
		changed = true
	}
	if changed {
		m.hasAppState = true
	}
	return changed
}
