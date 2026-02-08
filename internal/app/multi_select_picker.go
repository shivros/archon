package app

type MultiSelectPicker struct {
	width   int
	height  int
	cursor  int
	offset  int
	options []multiSelectOption
}

type multiSelectOption struct {
	id       string
	label    string
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
	p.options = options
	if p.cursor >= len(p.options) {
		p.cursor = max(0, len(p.options)-1)
	}
	p.ensureVisible()
}

func (p *MultiSelectPicker) Move(delta int) bool {
	if len(p.options) == 0 || delta == 0 {
		return false
	}
	next := clamp(p.cursor+delta, 0, len(p.options)-1)
	if next == p.cursor {
		return false
	}
	p.cursor = next
	p.ensureVisible()
	return true
}

func (p *MultiSelectPicker) Toggle() bool {
	if p.cursor < 0 || p.cursor >= len(p.options) {
		return false
	}
	p.options[p.cursor].selected = !p.options[p.cursor].selected
	return true
}

func (p *MultiSelectPicker) HandleClick(row int) bool {
	if row < 0 || row >= p.visibleHeight() {
		return false
	}
	index := p.offset + row
	if index < 0 || index >= len(p.options) {
		return false
	}
	p.cursor = index
	p.options[p.cursor].selected = !p.options[p.cursor].selected
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

func (p *MultiSelectPicker) View() string {
	if p.height <= 0 {
		return ""
	}
	lines := make([]string, 0, p.visibleHeight())
	if len(p.options) == 0 {
		lines = append(lines, " (none)")
		return padLines(lines, p.width)
	}
	for i := 0; i < p.visibleHeight(); i++ {
		idx := p.offset + i
		if idx >= len(p.options) {
			lines = append(lines, "")
			continue
		}
		opt := p.options[idx]
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
	if p.height <= 0 {
		return 0
	}
	return p.height
}

func (p *MultiSelectPicker) ensureVisible() {
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.visibleHeight() {
		p.offset = p.cursor - p.visibleHeight() + 1
	}
	p.clampOffset()
}

func (p *MultiSelectPicker) clampOffset() {
	if p.offset < 0 {
		p.offset = 0
	}
	maxOffset := max(0, len(p.options)-p.visibleHeight())
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
}
