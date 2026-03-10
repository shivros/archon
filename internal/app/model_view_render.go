package app

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

func (m *Model) renderRightPaneView() string {
	frame := m.layoutFrame()
	headerText, bodyText := m.modeViewContent()
	rightHeader := headerStyle.Render(headerText)
	rightBody := bodyText
	if m.usesViewport() {
		scrollbar := m.viewportScrollbarView()
		if scrollbar != "" {
			rightBody = lipgloss.JoinHorizontal(lipgloss.Top, bodyText, scrollbar)
		}
	}
	inputLine, inputScrollable := m.modeInputView()
	rightLines := []string{rightHeader, rightBody}
	if activity := m.composeActivityLine(time.Now()); activity != "" {
		rightLines = append(rightLines, activityStyle.Render(activity))
	}
	if inputLine != "" {
		dividerWidth := m.viewport.Width()
		if dividerWidth <= 0 {
			dividerWidth = max(1, m.width)
		}
		dividerLine := renderInputDivider(dividerWidth, inputScrollable)
		if dividerLine != "" {
			rightLines = append(rightLines, dividerLine)
		}
		rightLines = append(rightLines, inputLine)
	}
	mainView := lipgloss.JoinVertical(lipgloss.Left, rightLines...)
	if !frame.panelVisible || frame.panelWidth <= 0 {
		return mainView
	}
	panelView := ""
	panelHeight := 0
	switch m.activeSidePanelMode() {
	case sidePanelModeDebug:
		panelView, panelHeight = m.renderDebugPanelView()
	case sidePanelModeContext:
		panelView = m.renderContextPanelView()
		panelHeight = lipgloss.Height(panelView)
	default:
		panelView = m.renderNotesPanelView()
		panelHeight = lipgloss.Height(panelView)
	}
	height := max(lipgloss.Height(mainView), panelHeight)
	if height < 1 {
		height = 1
	}
	divider := strings.Repeat("│\n", height-1) + "│"
	return lipgloss.JoinHorizontal(lipgloss.Top, mainView, dividerStyle.Render(divider), panelView)
}

func (m *Model) renderBodyWithSidebar(rightView string) string {
	frame := m.layoutFrame()
	body := rightView
	if frame.sidebarWidth <= 0 {
		return body
	}
	listView := ""
	if m.sidebar != nil {
		m.sidebar.SetSidebarFocused(m.input != nil && m.input.IsSidebarFocused())
		listView = m.sidebar.View()
		listView = normalizeBlockWidth(listView, frame.sidebarWidth)
	}
	height := max(lipgloss.Height(listView), lipgloss.Height(rightView))
	if height < 1 {
		height = 1
	}
	divider := strings.Repeat("│\n", height-1) + "│"
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, dividerStyle.Render(divider), rightView)
}

func normalizeBlockWidth(block string, width int) string {
	if width <= 0 || block == "" {
		return block
	}
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = lipgloss.PlaceHorizontal(width, lipgloss.Left, line)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderDebugPanelView() (string, int) {
	if m.debugPanel == nil {
		m.debugPanel = NewDebugPanelController(max(1, m.debugPanelWidth), max(1, m.height-1), nil)
	}
	return m.debugPanel.View()
}

func (m *Model) renderStatusLineView() string {
	help, status := m.statusLineParts()
	return renderStatusLine(m.width, help, status)
}

func (m *Model) statusLineParts() (string, string) {
	help := ""
	status := statusStyle.Render(m.status)
	return help, status
}

func (m *Model) statusLineStatusHitbox() (int, int, bool) {
	help, status := m.statusLineParts()
	return statusLineStatusBounds(m.width, help, status)
}

func (m *Model) renderedBodyHeight() int {
	rightView := m.renderRightPaneView()
	body := m.renderBodyWithSidebar(rightView)
	if m.height > 0 && m.width > 0 {
		body = m.overlayTransientViews(body)
	}
	return lipgloss.Height(body)
}

func (m *Model) overlayTransientViews(body string) string {
	if body == "" {
		return body
	}
	m.statusHistoryLastViewValid = false
	bodyHeight := len(strings.Split(body, "\n"))
	ctx := TransientOverlayContext{
		Body:       body,
		BodyHeight: bodyHeight,
	}
	providers := m.transientOverlayProvidersOrDefault()
	overlays := make([]LayerOverlay, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		overlay, ok := provider.Build(m, ctx)
		if ok {
			overlays = append(overlays, overlay)
		}
	}
	composer := m.overlayComposerOrDefault()
	return composer.Compose(body, overlays)
}

func (m *Model) toastOverlay(bodyHeight int) (string, int, bool) {
	if bodyHeight < 1 {
		return "", 0, false
	}
	if m.statusHistoryOverlayOpen() {
		return "", 0, false
	}
	line := m.toastLine(m.width)
	if line == "" {
		return "", 0, false
	}
	row := bodyHeight - 1
	if row < 0 {
		return "", 0, false
	}
	return line, row, true
}
