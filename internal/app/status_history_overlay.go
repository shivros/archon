package app

import (
	"fmt"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

type statusHistoryOverlayHitbox struct {
	panelLeftX    int
	panelRightX   int
	panelTopY     int
	panelBottomY  int
	listIndexByY  map[int]int
	copyRowY      int
	copyStartX    int
	copyEndX      int
	copyAvailable bool
}

func (h statusHistoryOverlayHitbox) contains(x, y int) bool {
	return statusHistoryMouseColInRange(x, h.panelLeftX, h.panelRightX) && statusHistoryMouseRowInRange(y, h.panelTopY, h.panelBottomY)
}

func (h statusHistoryOverlayHitbox) listIndexAt(y int) (int, bool) {
	if len(h.listIndexByY) == 0 {
		return 0, false
	}
	index, ok := h.listIndexByY[y]
	if ok {
		return index, true
	}
	index, ok = h.listIndexByY[y-1]
	if ok {
		return index, true
	}
	index, ok = h.listIndexByY[y+1]
	return index, ok
}

func (h statusHistoryOverlayHitbox) copyContains(x, y int) bool {
	if !h.copyAvailable || h.copyRowY <= 0 {
		return false
	}
	if y != h.copyRowY && y != h.copyRowY-1 && y != h.copyRowY+1 {
		return false
	}
	return statusHistoryMouseColInRange(x, h.copyStartX, h.copyEndX)
}

type statusHistoryOverlayView struct {
	block       string
	x           int
	row         int
	visibleRows int
	entries     []string
	hitbox      statusHistoryOverlayHitbox
}

type statusHistoryOverlayRenderInput struct {
	entries       []string
	selectedIndex int
	scrollOffset  int
	width         int
	rightStart    int
	bodyHeight    int
}

type StatusHistoryOverlayPresenter interface {
	Render(input statusHistoryOverlayRenderInput) statusHistoryOverlayView
}

type statusHistoryOverlayLayoutEngine interface {
	Layout(input statusHistoryOverlayRenderInput) (statusHistoryOverlayLayout, bool)
}

type statusHistoryOverlayRenderer interface {
	Render(layout statusHistoryOverlayLayout) string
}

type statusHistoryOverlayHitTester interface {
	Build(layout statusHistoryOverlayLayout) statusHistoryOverlayHitbox
}

type defaultStatusHistoryOverlayPresenter struct {
	layoutEngine statusHistoryOverlayLayoutEngine
	renderer     statusHistoryOverlayRenderer
	hitTester    statusHistoryOverlayHitTester
}

func newDefaultStatusHistoryOverlayPresenter(config StatusHistoryOverlayConfig) StatusHistoryOverlayPresenter {
	return defaultStatusHistoryOverlayPresenter{
		layoutEngine: defaultStatusHistoryOverlayLayoutEngine{config: config},
		renderer:     defaultStatusHistoryOverlayRenderer{},
		hitTester:    defaultStatusHistoryOverlayHitTester{},
	}
}

func (p defaultStatusHistoryOverlayPresenter) Render(input statusHistoryOverlayRenderInput) statusHistoryOverlayView {
	layout, ok := p.layoutEngine.Layout(input)
	if !ok {
		return statusHistoryOverlayView{}
	}
	block := p.renderer.Render(layout)
	if strings.TrimSpace(block) == "" {
		return statusHistoryOverlayView{}
	}
	return statusHistoryOverlayView{
		block:       block,
		x:           layout.panelLeft,
		row:         layout.row,
		visibleRows: layout.visibleRows,
		entries:     input.entries,
		hitbox:      p.hitTester.Build(layout),
	}
}

type statusHistoryOverlayLayoutLine struct {
	text     string
	selected bool
}

type statusHistoryOverlayLayout struct {
	entries           []string
	row               int
	panelLeft         int
	panelWidth        int
	contentWidth      int
	visibleRows       int
	visibleLineStart  int
	lines             []statusHistoryOverlayLayoutLine
	listLocalRowIndex map[int]int
	copyLocalRow      int
}

type defaultStatusHistoryOverlayLayoutEngine struct {
	config StatusHistoryOverlayConfig
}

func (e defaultStatusHistoryOverlayLayoutEngine) Layout(input statusHistoryOverlayRenderInput) (statusHistoryOverlayLayout, bool) {
	cfg := e.config
	if cfg.RowTruncateWidth <= 0 {
		cfg.RowTruncateWidth = 64
	}
	if cfg.PanelMinWidth <= 0 {
		cfg.PanelMinWidth = 38
	}
	if cfg.PanelMaxWidth < cfg.PanelMinWidth {
		cfg.PanelMaxWidth = cfg.PanelMinWidth
	}
	total := len(input.entries)
	if total == 0 || input.width <= 0 || input.bodyHeight <= 0 {
		return statusHistoryOverlayLayout{}, false
	}
	rightStart := max(0, input.rightStart)
	availableWidth := input.width - rightStart
	if availableWidth < cfg.PanelMinWidth {
		return statusHistoryOverlayLayout{}, false
	}
	panelWidth := min(cfg.PanelMaxWidth, max(cfg.PanelMinWidth, availableWidth))
	panelLeft := input.width - panelWidth
	if panelLeft < rightStart {
		panelLeft = rightStart
		panelWidth = input.width - panelLeft
		if panelWidth < cfg.PanelMinWidth {
			return statusHistoryOverlayLayout{}, false
		}
	}
	contentWidth := panelWidth - 4
	if contentWidth < 8 {
		return statusHistoryOverlayLayout{}, false
	}

	visibleRows := min(total, statusHistoryListVisibleRows)
	if visibleRows <= 0 {
		visibleRows = 1
	}
	maxOffset := max(0, total-visibleRows)
	scrollOffset := clamp(input.scrollOffset, 0, maxOffset)
	selected := clamp(input.selectedIndex, -1, total-1)

	lines := make([]statusHistoryOverlayLayoutLine, 0, visibleRows+6)
	listLocal := make(map[int]int, visibleRows)
	lines = append(lines, statusHistoryOverlayLayoutLine{text: fmt.Sprintf("status history (%d)", total)})
	for i := 0; i < visibleRows; i++ {
		entryIndex := scrollOffset + i
		entry := input.entries[entryIndex]
		text := truncateToWidth(entry, cfg.RowTruncateWidth)
		text = truncateToWidth(text, contentWidth-2)
		prefix := " "
		if entryIndex == selected {
			prefix = ">"
		}
		lines = append(lines, statusHistoryOverlayLayoutLine{
			text:     fmt.Sprintf("%s %s", prefix, text),
			selected: entryIndex == selected,
		})
		listLocal[len(lines)] = entryIndex
	}
	copyLocalRow := -1
	if selected >= 0 {
		lines = append(lines, statusHistoryOverlayLayoutLine{text: strings.Repeat("─", min(12, contentWidth))})
		lines = append(lines, statusHistoryOverlayLayoutLine{text: "full status"})
		for _, wrapped := range wrapPlainToWidth(input.entries[selected], contentWidth) {
			lines = append(lines, statusHistoryOverlayLayoutLine{text: wrapped})
		}
		lines = append(lines, statusHistoryOverlayLayoutLine{text: "  [ copy ]"})
		copyLocalRow = len(lines)
	}
	panelHeight := len(lines)
	visibleLineStart := 0
	if panelHeight > input.bodyHeight {
		visibleLineStart = panelHeight - input.bodyHeight
	}
	visiblePanelHeight := panelHeight - visibleLineStart
	if visiblePanelHeight <= 0 {
		return statusHistoryOverlayLayout{}, false
	}
	row := input.bodyHeight - visiblePanelHeight
	if row < 0 {
		row = 0
	}

	return statusHistoryOverlayLayout{
		entries:           input.entries,
		row:               row,
		panelLeft:         panelLeft,
		panelWidth:        panelWidth,
		contentWidth:      contentWidth,
		visibleRows:       visibleRows,
		visibleLineStart:  visibleLineStart,
		lines:             lines,
		listLocalRowIndex: listLocal,
		copyLocalRow:      copyLocalRow,
	}, true
}

type defaultStatusHistoryOverlayRenderer struct{}

func (defaultStatusHistoryOverlayRenderer) Render(layout statusHistoryOverlayLayout) string {
	if layout.panelWidth <= 0 || layout.contentWidth <= 0 || len(layout.lines) == 0 {
		return ""
	}
	rendered := make([]string, 0, len(layout.lines)-layout.visibleLineStart)
	for i := layout.visibleLineStart; i < len(layout.lines); i++ {
		line := layout.lines[i]
		text := padToWidth(truncateToWidth(strings.TrimSpace(line.text), layout.contentWidth), layout.contentWidth)
		if line.selected {
			text = statusHistorySelectedStyle.Render(text)
		}
		rendered = append(rendered, "  "+text+"  ")
	}
	panel := strings.Join(rendered, "\n")
	return menuDropStyle.Width(layout.panelWidth).Render(panel)
}

type defaultStatusHistoryOverlayHitTester struct{}

func (defaultStatusHistoryOverlayHitTester) Build(layout statusHistoryOverlayLayout) statusHistoryOverlayHitbox {
	hit := statusHistoryOverlayHitbox{
		listIndexByY: make(map[int]int, len(layout.listLocalRowIndex)),
	}
	visibleHeight := len(layout.lines) - layout.visibleLineStart
	if visibleHeight <= 0 {
		return hit
	}
	hit.panelLeftX = layout.panelLeft
	hit.panelRightX = layout.panelLeft + layout.panelWidth - 1
	hit.panelTopY = layout.row + 1
	hit.panelBottomY = hit.panelTopY + visibleHeight - 1

	for localRow, entryIndex := range layout.listLocalRowIndex {
		if localRow <= layout.visibleLineStart {
			continue
		}
		globalY := layout.row + (localRow - layout.visibleLineStart)
		hit.listIndexByY[globalY] = entryIndex
	}
	if layout.copyLocalRow > layout.visibleLineStart {
		hit.copyAvailable = true
		hit.copyRowY = layout.row + (layout.copyLocalRow - layout.visibleLineStart)
		hit.copyStartX = layout.panelLeft + 4
		hit.copyEndX = hit.copyStartX + len("[ copy ]") - 1
	}
	return hit
}

func wrapPlainToWidth(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{"(empty)"}
	}
	if width <= 1 {
		return []string{text}
	}
	totalWidth := xansi.StringWidth(text)
	if totalWidth <= width {
		return []string{text}
	}
	out := make([]string, 0, totalWidth/width+1)
	for start := 0; start < totalWidth; start += width {
		end := start + width
		if end > totalWidth {
			end = totalWidth
		}
		out = append(out, xansi.Cut(text, start, end))
	}
	return out
}

func statusHistoryMouseColInRange(x, start, end int) bool {
	if x >= start && x <= end {
		return true
	}
	col := x - 1
	return col >= start && col <= end
}

func statusHistoryMouseRowInRange(y, start, end int) bool {
	if y >= start && y <= end {
		return true
	}
	row := y - 1
	return row >= start && row <= end
}

func (m *Model) statusHistoryOverlayOpen() bool {
	return m != nil && m.statusHistoryOverlay.IsOpen()
}

func (m *Model) statusHistoryOverlayView(bodyHeight int) (string, int, int, bool) {
	view, ok := m.computeStatusHistoryOverlayView(bodyHeight)
	if !ok {
		return "", 0, 0, false
	}
	m.statusHistoryLastView = view
	m.statusHistoryLastViewValid = true
	return view.block, view.x, view.row, true
}

func (m *Model) computeStatusHistoryOverlayView(bodyHeight int) (statusHistoryOverlayView, bool) {
	if m == nil || !m.statusHistoryOverlayOpen() {
		return statusHistoryOverlayView{}, false
	}
	entries := m.statusHistory.SnapshotNewestFirst()
	if len(entries) == 0 {
		return statusHistoryOverlayView{}, false
	}
	visibleRows := min(len(entries), statusHistoryListVisibleRows)
	m.statusHistoryOverlay.Reconcile(len(entries), visibleRows)
	presenter := m.statusHistoryPresenter
	if presenter == nil {
		presenter = newDefaultStatusHistoryOverlayPresenter(defaultStatusHistoryOverlayConfig())
	}
	view := presenter.Render(statusHistoryOverlayRenderInput{
		entries:       entries,
		selectedIndex: m.statusHistoryOverlay.SelectedIndex(),
		scrollOffset:  m.statusHistoryOverlay.ScrollOffset(),
		width:         m.width,
		rightStart:    m.resolveMouseLayout().rightStart,
		bodyHeight:    bodyHeight,
	})
	if strings.TrimSpace(view.block) == "" {
		return statusHistoryOverlayView{}, false
	}
	return view, true
}

func (m *Model) currentStatusHistoryOverlayView() (statusHistoryOverlayView, bool) {
	if m == nil || !m.statusHistoryOverlayOpen() {
		return statusHistoryOverlayView{}, false
	}
	if m.statusHistoryLastViewValid && strings.TrimSpace(m.statusHistoryLastView.block) != "" {
		return m.statusHistoryLastView, true
	}
	return m.computeStatusHistoryOverlayView(m.renderedBodyHeight())
}

func (m *Model) closeStatusHistoryOverlay() {
	if m == nil {
		return
	}
	m.statusHistoryOverlay.Close()
	m.statusHistoryLastViewValid = false
}

func (m *Model) toggleStatusHistoryOverlay() {
	if m == nil {
		return
	}
	m.statusHistoryOverlay.Toggle()
	m.statusHistoryLastViewValid = false
}
