package app

type layoutFrame struct {
	sidebarWidth int
	rightStart   int

	panelVisible bool
	panelMain    int
	panelWidth   int
	panelStart   int
}

type resizeLayout struct {
	contentHeight int
	sidebarWidth  int
	viewportWidth int
	contentWidth  int
	panelVisible  bool
	panelMain     int
	panelWidth    int
}

type sidePanelMode int

const (
	sidePanelModeNone sidePanelMode = iota
	sidePanelModeNotes
	sidePanelModeDebug
)

const (
	sidePanelMinWidth = 28
	sidePanelMaxWidth = 56
)

func computeSidebarWidth(totalWidth int, collapsed bool) int {
	if collapsed {
		return 0
	}
	listWidth := clamp(totalWidth/3, minListWidth, maxListWidth)
	if totalWidth-listWidth-1 < minViewportWidth {
		listWidth = max(minListWidth, totalWidth/2)
	}
	return listWidth
}

func resolveResizeLayout(width, height int, sidebarCollapsed bool, panelMode sidePanelMode, usesViewport bool) resizeLayout {
	layout := resizeLayout{
		contentHeight: max(minContentHeight, height-2),
		sidebarWidth:  computeSidebarWidth(width, sidebarCollapsed),
		viewportWidth: width,
	}
	if layout.sidebarWidth > 0 {
		layout.viewportWidth = max(minViewportWidth, width-layout.sidebarWidth-1)
	}
	layout.contentWidth = layout.viewportWidth
	layout.panelMain = layout.viewportWidth
	if panelMode != sidePanelModeNone {
		layout.panelWidth = clamp(layout.viewportWidth/3, sidePanelMinWidth, sidePanelMaxWidth)
		if layout.viewportWidth-layout.panelWidth-1 >= minViewportWidth {
			layout.panelVisible = true
			layout.panelMain = layout.viewportWidth - layout.panelWidth - 1
			layout.contentWidth = layout.panelMain
		}
	}
	if usesViewport && layout.panelMain > minViewportWidth+viewportScrollbarWidth {
		layout.contentWidth = layout.panelMain - viewportScrollbarWidth
	}
	return layout
}

func (m *Model) layoutFrame() layoutFrame {
	sidebarWidth := m.sidebarWidth()
	panelVisible, panelMain, panelWidth := m.activePanelDimensions()
	frame := layoutFrame{
		sidebarWidth: sidebarWidth,
		panelVisible: panelVisible,
		panelMain:    panelMain,
		panelWidth:   panelWidth,
	}
	if sidebarWidth > 0 {
		frame.rightStart = sidebarWidth + 1
	}
	if frame.panelVisible {
		frame.panelStart = frame.rightStart + frame.panelMain + 1
	}
	return frame
}

func (m *Model) activePanelDimensions() (visible bool, main int, width int) {
	if m != nil && m.appState.DebugStreamsEnabled {
		return m.debugPanelVisible && m.debugPanelWidth > 0, m.debugPanelMainWidth, m.debugPanelWidth
	}
	return m.notesPanelVisible && m.notesPanelWidth > 0, m.notesPanelMainWidth, m.notesPanelWidth
}
