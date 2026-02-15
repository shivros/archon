package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

type GroupPicker struct {
	width   int
	height  int
	cursor  int
	offset  int
	query   string
	options []groupOption
	visible []int
}

type groupOption struct {
	id       string
	label    string
	search   string
	selected bool
}

func NewGroupPicker(width, height int) *GroupPicker {
	return &GroupPicker{width: width, height: height}
}

func (p *GroupPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.clampOffset()
}

func (p *GroupPicker) SetGroups(groups []*types.WorkspaceGroup, selectedIDs map[string]bool) {
	options := make([]groupOption, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		label := strings.TrimSpace(group.Name)
		if label == "" {
			continue
		}
		options = append(options, groupOption{
			id:       group.ID,
			label:    label,
			selected: selectedIDs[group.ID],
		})
	}
	p.options = options
	p.rebuildVisible()
}

func (p *GroupPicker) Move(delta int) bool {
	if len(p.visible) == 0 || delta == 0 {
		return false
	}
	next := clamp(p.cursor+delta, 0, len(p.visible)-1)
	if next == p.cursor {
		return false
	}
	p.cursor = next
	p.ensureVisible()
	return true
}

func (p *GroupPicker) Toggle() bool {
	optionIndex := p.selectedOptionIndex()
	if optionIndex < 0 || optionIndex >= len(p.options) {
		return false
	}
	p.options[optionIndex].selected = !p.options[optionIndex].selected
	return true
}

func (p *GroupPicker) HandleClick(row int) bool {
	row -= p.queryRows()
	if row < 0 || row >= p.visibleHeight() {
		return false
	}
	index := p.offset + row
	if index < 0 || index >= len(p.visible) {
		return false
	}
	p.cursor = index
	optionIndex := p.selectedOptionIndex()
	if optionIndex < 0 || optionIndex >= len(p.options) {
		return false
	}
	p.options[optionIndex].selected = !p.options[optionIndex].selected
	p.ensureVisible()
	return true
}

func (p *GroupPicker) SelectedIDs() []string {
	out := make([]string, 0, len(p.options))
	for _, opt := range p.options {
		if opt.selected {
			out = append(out, opt.id)
		}
	}
	return out
}

func (p *GroupPicker) Query() string {
	return p.query
}

func (p *GroupPicker) SetQuery(query string) bool {
	if query == p.query {
		return false
	}
	selected := p.selectedOptionIndex()
	p.query = query
	p.rebuildVisible()
	if selected >= 0 {
		if pos := p.visiblePosition(selected); pos >= 0 {
			p.cursor = pos
			p.ensureVisible()
		}
	}
	return true
}

func (p *GroupPicker) AppendQuery(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	return p.SetQuery(p.query + text)
}

func (p *GroupPicker) BackspaceQuery() bool {
	if p.query == "" {
		return false
	}
	runes := []rune(p.query)
	return p.SetQuery(string(runes[:len(runes)-1]))
}

func (p *GroupPicker) ClearQuery() bool {
	return p.SetQuery("")
}

func (p *GroupPicker) View() string {
	if p.height <= 0 {
		return ""
	}
	lines := make([]string, 0, p.height)
	lines = append(lines, renderPickerQueryLine(p.query))
	if p.visibleHeight() <= 0 {
		return padBlock(lines, p.width)
	}
	if len(p.options) == 0 {
		lines = append(lines, " (no groups)")
		for len(lines) < p.height {
			lines = append(lines, "")
		}
		return padBlock(lines, p.width)
	}
	if len(p.visible) == 0 {
		lines = append(lines, " (no matches)")
		for len(lines) < p.height {
			lines = append(lines, "")
		}
		return padBlock(lines, p.width)
	}
	for i := 0; i < p.visibleHeight(); i++ {
		idx := p.offset + i
		if idx >= len(p.visible) {
			lines = append(lines, "")
			continue
		}
		optionIndex := p.visible[idx]
		if optionIndex < 0 || optionIndex >= len(p.options) {
			lines = append(lines, "")
			continue
		}
		opt := p.options[optionIndex]
		checkbox := "[ ]"
		if opt.selected {
			checkbox = "[x]"
		}
		line := " " + checkbox + " " + opt.label
		if idx == p.cursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return padBlock(lines, p.width)
}

func (p *GroupPicker) visibleHeight() int {
	height := p.height - p.queryRows()
	if height <= 0 {
		return 0
	}
	return height
}

func (p *GroupPicker) ensureVisible() {
	if p.visibleHeight() <= 0 {
		p.offset = 0
		return
	}
	if len(p.visible) == 0 {
		p.cursor = 0
		p.offset = 0
		return
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.visible) {
		p.cursor = len(p.visible) - 1
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.visibleHeight() {
		p.offset = p.cursor - p.visibleHeight() + 1
	}
	p.clampOffset()
}

func (p *GroupPicker) clampOffset() {
	if p.visibleHeight() <= 0 {
		p.offset = 0
		return
	}
	if p.offset < 0 {
		p.offset = 0
	}
	maxOffset := max(0, len(p.visible)-p.visibleHeight())
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
}

func (p *GroupPicker) queryRows() int {
	if p.height <= 0 {
		return 0
	}
	return 1
}

func (p *GroupPicker) selectedOptionIndex() int {
	if p.cursor < 0 || p.cursor >= len(p.visible) {
		return -1
	}
	return p.visible[p.cursor]
}

func (p *GroupPicker) visiblePosition(optionIndex int) int {
	for i, idx := range p.visible {
		if idx == optionIndex {
			return i
		}
	}
	return -1
}

func (p *GroupPicker) rebuildVisible() {
	p.visible = pickerFilterIndices(p.query, len(p.options), func(index int) (label, id, search string) {
		opt := p.options[index]
		return opt.label, opt.id, opt.search
	})
	if len(p.visible) == 0 {
		p.cursor = 0
		p.offset = 0
		return
	}
	if p.cursor >= len(p.visible) {
		p.cursor = len(p.visible) - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
	p.ensureVisible()
}

func padBlock(lines []string, width int) string {
	if width <= 0 {
		return strings.Join(lines, "\n")
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		lineWidth := xansi.StringWidth(line)
		if lineWidth < width {
			line = line + strings.Repeat(" ", width-lineWidth)
		}
		out[i] = line
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(out, "\n"))
}
