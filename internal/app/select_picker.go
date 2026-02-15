package app

import "strings"

type SelectPicker struct {
	width   int
	height  int
	cursor  int
	offset  int
	query   string
	options []selectOption
	visible []int
}

type selectOption struct {
	id     string
	label  string
	search string
}

func NewSelectPicker(width, height int) *SelectPicker {
	return &SelectPicker{width: width, height: height}
}

func (p *SelectPicker) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.clampOffset()
}

func (p *SelectPicker) SetOptions(options []selectOption) {
	p.options = append(p.options[:0], options...)
	p.rebuildVisible()
}

func (p *SelectPicker) Move(delta int) bool {
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

func (p *SelectPicker) SelectedID() string {
	optionIndex := p.selectedOptionIndex()
	if optionIndex < 0 || optionIndex >= len(p.options) {
		return ""
	}
	return p.options[optionIndex].id
}

func (p *SelectPicker) HandleClick(row int) bool {
	row -= p.queryRows()
	if row < 0 || row >= p.visibleHeight() {
		return false
	}
	index := p.offset + row
	if index < 0 || index >= len(p.visible) {
		return false
	}
	p.cursor = index
	p.ensureVisible()
	return true
}

func (p *SelectPicker) SelectID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	target := -1
	for i, option := range p.options {
		if strings.EqualFold(strings.TrimSpace(option.id), id) {
			target = i
			break
		}
	}
	if target < 0 {
		return false
	}
	if pos := p.visiblePosition(target); pos >= 0 {
		p.cursor = pos
		p.ensureVisible()
		return true
	}
	if p.query != "" {
		p.query = ""
		p.rebuildVisible()
		if pos := p.visiblePosition(target); pos >= 0 {
			p.cursor = pos
			p.ensureVisible()
			return true
		}
	}
	return false
}

func (p *SelectPicker) Query() string {
	return p.query
}

func (p *SelectPicker) SetQuery(query string) bool {
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

func (p *SelectPicker) AppendQuery(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	return p.SetQuery(p.query + text)
}

func (p *SelectPicker) BackspaceQuery() bool {
	if p.query == "" {
		return false
	}
	runes := []rune(p.query)
	return p.SetQuery(string(runes[:len(runes)-1]))
}

func (p *SelectPicker) ClearQuery() bool {
	return p.SetQuery("")
}

func (p *SelectPicker) View() string {
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
		line := " " + opt.label
		if idx == p.cursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return padLines(lines, p.width)
}

func (p *SelectPicker) visibleHeight() int {
	height := p.height - p.queryRows()
	if height <= 0 {
		return 0
	}
	return height
}

func (p *SelectPicker) ensureVisible() {
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

func (p *SelectPicker) clampOffset() {
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

func (p *SelectPicker) queryRows() int {
	if p.height <= 0 {
		return 0
	}
	return 1
}

func (p *SelectPicker) selectedOptionIndex() int {
	if p.cursor < 0 || p.cursor >= len(p.visible) {
		return -1
	}
	return p.visible[p.cursor]
}

func (p *SelectPicker) visiblePosition(optionIndex int) int {
	for i, idx := range p.visible {
		if idx == optionIndex {
			return i
		}
	}
	return -1
}

func (p *SelectPicker) rebuildVisible() {
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
