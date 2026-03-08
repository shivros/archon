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
	sidePanelModeContext
)

const (
	sidePanelMinWidth = 28
	sidePanelMaxWidth = 56
)

func computeSidebarWidth(totalWidth int, collapsed bool, pref *SplitPreference) int {
	return resolveSidebarWidth(totalWidth, collapsed, pref)
}

func resolveResizeLayout(
	width, height int,
	sidebarCollapsed bool,
	sidebarSplit, mainSideSplit *SplitPreference,
	panelWidth int,
	panelMode sidePanelMode,
	usesViewport bool,
) resizeLayout {
	layout := resizeLayout{
		contentHeight: max(minContentHeight, height-1),
		sidebarWidth:  computeSidebarWidth(width, sidebarCollapsed, sidebarSplit),
		viewportWidth: width,
	}
	if layout.sidebarWidth > 0 {
		layout.viewportWidth = max(minViewportWidth, width-layout.sidebarWidth-1)
	}
	layout.contentWidth = layout.viewportWidth
	layout.panelMain = layout.viewportWidth
	if panelMode != sidePanelModeNone {
		layout.panelWidth = resolveSidePanelWidth(layout.viewportWidth, mainSideSplit, panelWidth)
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
	switch m.activeSidePanelMode() {
	case sidePanelModeDebug:
		return m.debugPanelVisible && m.debugPanelWidth > 0, m.debugPanelMainWidth, m.debugPanelWidth
	case sidePanelModeContext:
		return m.contextPanelVisible && m.contextPanelWidth > 0, m.contextPanelMainWidth, m.contextPanelWidth
	case sidePanelModeNotes:
		return m.notesPanelVisible && m.notesPanelWidth > 0, m.notesPanelMainWidth, m.notesPanelWidth
	default:
		return false, 0, 0
	}
}
