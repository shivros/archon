package app

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
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
	rightWidth := m.width - frame.rightStart
	if rightWidth < 1 {
		rightWidth = max(1, blockWidth(strings.Split(rightView, "\n")))
	}
	body := rightView
	if frame.sidebarWidth <= 0 {
		return fillBlockWithBackground(body, rightWidth, lipgloss.Height(body), mainPaneStyle)
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
	rightView = fillBlockWithBackground(rightView, rightWidth, height, mainPaneStyle)
	listView = fillBlockWithBackground(listView, frame.sidebarWidth, height, sidebarPaneStyle)
	divider := strings.Repeat("│\n", height-1) + "│"
	return lipgloss.JoinHorizontal(lipgloss.Top, listView, dividerStyle.Render(divider), rightView)
}

func fillBlockWithBackground(block string, width, height int, style lipgloss.Style) string {
	if width <= 0 {
		return block
	}
	if height < 1 {
		height = max(1, lipgloss.Height(block))
	}
	prefix, suffix, hasBackgroundEnvelope := backgroundEnvelope(style)
	lines := strings.Split(block, "\n")
	rendered := make([]string, 0, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		rendered = append(rendered, applyBackgroundAcrossLine(line, width, prefix, suffix, hasBackgroundEnvelope))
	}
	return strings.Join(rendered, "\n")
}

func backgroundEnvelope(style lipgloss.Style) (prefix, suffix string, ok bool) {
	marker := "X"
	styledMarker := style.Render(marker)
	idx := strings.Index(styledMarker, marker)
	if idx < 0 {
		return "", "", false
	}
	prefix = styledMarker[:idx]
	suffix = styledMarker[idx+len(marker):]
	if prefix == "" && suffix == "" {
		return "", "", false
	}
	return prefix, suffix, true
}

func applyBackgroundAcrossLine(line string, width int, prefix, suffix string, hasBackgroundEnvelope bool) string {
	lineWidth := xansi.StringWidth(line)
	if width < 0 {
		width = 0
	}
	padding := ""
	if lineWidth < width {
		padding = strings.Repeat(" ", width-lineWidth)
	}
	if !hasBackgroundEnvelope {
		return line + padding
	}
	// Re-apply pane background after background-reset SGR sequences so existing
	// styled segments don't leak back to the terminal default background.
	line = reapplyBackgroundAfterResetSGR(line, prefix)
	return prefix + line + padding + suffix
}

func reapplyBackgroundAfterResetSGR(line, prefix string) string {
	if line == "" || prefix == "" {
		return line
	}
	var b strings.Builder
	for i := 0; i < len(line); {
		if line[i] != '\x1b' || i+1 >= len(line) || line[i+1] != '[' {
			b.WriteByte(line[i])
			i++
			continue
		}
		seqEnd := i + 2
		for seqEnd < len(line) && line[seqEnd] != 'm' {
			seqEnd++
		}
		if seqEnd >= len(line) {
			b.WriteString(line[i:])
			break
		}
		seq := line[i : seqEnd+1]
		b.WriteString(seq)
		params := line[i+2 : seqEnd]
		if sgrResetsBackground(params) {
			b.WriteString(prefix)
		}
		i = seqEnd + 1
	}
	return b.String()
}

func sgrResetsBackground(params string) bool {
	if params == "" {
		return true
	}
	parts := strings.Split(params, ";")
	resetsBg := false
	setsBg := false
	for _, part := range parts {
		if part == "" {
			resetsBg = true
			continue
		}
		code, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		switch {
		case code == 0:
			resetsBg = true
		case code == 49:
			resetsBg = true
		case code == 48:
			setsBg = true
		case code >= 40 && code <= 47:
			setsBg = true
		case code >= 100 && code <= 107:
			setsBg = true
		}
	}
	return resetsBg && !setsBg
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
	_, row, ok := m.rightPaneOverlayPlacement(bodyHeight, xansi.StringWidth(line), rightPaneOverlayAlignBottom)
	if !ok {
		return "", 0, false
	}
	return line, row, true
}

func (m *Model) loadingOverlay(bodyHeight int) (string, int, int, bool) {
	if m == nil || !m.loading || bodyHeight < 1 {
		return "", 0, 0, false
	}
	line := toastInfoStyle.Render(" " + m.loader.View() + " Loading... ")
	x, row, ok := m.rightPaneOverlayPlacement(bodyHeight, xansi.StringWidth(line), rightPaneOverlayAlignViewportCenter)
	if !ok {
		return "", 0, 0, false
	}
	return line, x, row, true
}

type rightPaneOverlayAlign int

const (
	rightPaneOverlayAlignBottom rightPaneOverlayAlign = iota
	rightPaneOverlayAlignViewportCenter
)

func (m *Model) rightPaneOverlayPlacement(bodyHeight, overlayWidth int, align rightPaneOverlayAlign) (int, int, bool) {
	if m == nil || bodyHeight < 1 {
		return 0, 0, false
	}
	frame := m.layoutFrame()
	paneWidth := m.width - frame.rightStart
	if frame.panelVisible && frame.panelMain > 0 {
		paneWidth = frame.panelMain
	}
	if paneWidth <= 0 {
		paneWidth = m.viewport.Width()
	}
	if paneWidth <= 0 {
		paneWidth = overlayWidth
	}
	x := frame.rightStart
	if overlayWidth > 0 {
		if extra := paneWidth - overlayWidth; extra > 0 {
			x += extra / 2
		}
	}
	row := bodyHeight - 1
	switch align {
	case rightPaneOverlayAlignViewportCenter:
		row = min(bodyHeight-1, max(1, bodyHeight/2))
		if m.viewport.Height() > 0 {
			row = min(bodyHeight-1, 1+max(0, m.viewport.Height()/2))
		}
	}
	if row < 0 {
		return 0, 0, false
	}
	return x, row, true
}
