package app

import "strings"

type MultiSelectPicker struct {
	width   int
	height  int
	cursor  int
	offset  int
	query   string
	options []multiSelectOption
	visible []int
}

type multiSelectOption struct {
	id       string
	label    string
	search   string
	selected bool
}

func NewMultiSelectPicker(width, height int) *MultiSelectPicker {
	return &MultiSelectPicker{width: width, height: height}
}

func (p *MultiSelectPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.clampOffset()
}

func (p *MultiSelectPicker) SetOptions(options []multiSelectOption) {
	p.options = append(p.options[:0], options...)
	p.rebuildVisible()
}

func (p *MultiSelectPicker) Move(delta int) bool {
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

func (p *MultiSelectPicker) Toggle() bool {
	optionIndex := p.selectedOptionIndex()
	if optionIndex < 0 || optionIndex >= len(p.options) {
		return false
	}
	p.options[optionIndex].selected = !p.options[optionIndex].selected
	return true
}

func (p *MultiSelectPicker) HandleClick(row int) bool {
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

func (p *MultiSelectPicker) SelectedIDs() []string {
	out := make([]string, 0, len(p.options))
	for _, opt := range p.options {
		if opt.selected {
			out = append(out, opt.id)
		}
	}
	return out
}

func (p *MultiSelectPicker) Query() string {
	return p.query
}

func (p *MultiSelectPicker) SetQuery(query string) bool {
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

func (p *MultiSelectPicker) AppendQuery(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	return p.SetQuery(p.query + text)
}

func (p *MultiSelectPicker) BackspaceQuery() bool {
	if p.query == "" {
		return false
	}
	runes := []rune(p.query)
	return p.SetQuery(string(runes[:len(runes)-1]))
}

func (p *MultiSelectPicker) ClearQuery() bool {
	return p.SetQuery("")
}

func (p *MultiSelectPicker) View() string {
	if p.height <= 0 {
		return ""
	}
	lines := make([]string, 0, p.height)
	lines = append(lines, renderPickerQueryLine(p.query))
	if p.visibleHeight() <= 0 {
		return padLines(lines, p.width)
	}
	if len(p.options) == 0 {
		lines = append(lines, " (none)")
		for len(lines) < p.height {
			lines = append(lines, "")
		}
		return padLines(lines, p.width)
	}
	if len(p.visible) == 0 {
		lines = append(lines, " (no matches)")
		for len(lines) < p.height {
			lines = append(lines, "")
		}
		return padLines(lines, p.width)
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
	return padLines(lines, p.width)
}

func (p *MultiSelectPicker) visibleHeight() int {
	height := p.height - p.queryRows()
	if height <= 0 {
		return 0
	}
	return height
}

func (p *MultiSelectPicker) ensureVisible() {
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

func (p *MultiSelectPicker) clampOffset() {
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

func (p *MultiSelectPicker) queryRows() int {
	if p.height <= 0 {
		return 0
	}
	return 1
}

func (p *MultiSelectPicker) selectedOptionIndex() int {
	if p.cursor < 0 || p.cursor >= len(p.visible) {
		return -1
	}
	return p.visible[p.cursor]
}

func (p *MultiSelectPicker) visiblePosition(optionIndex int) int {
	for i, idx := range p.visible {
		if idx == optionIndex {
			return i
		}
	}
	return -1
}

func (p *MultiSelectPicker) rebuildVisible() {
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
