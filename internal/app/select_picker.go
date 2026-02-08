package app

type SelectPicker struct {
	width   int
	height  int
	cursor  int
	offset  int
	options []selectOption
}

type selectOption struct {
	id    string
	label string
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
	p.options = options
	if p.cursor >= len(p.options) {
		p.cursor = max(0, len(p.options)-1)
	}
	p.ensureVisible()
}

func (p *SelectPicker) Move(delta int) bool {
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

func (p *SelectPicker) SelectedID() string {
	if p.cursor < 0 || p.cursor >= len(p.options) {
		return ""
	}
	return p.options[p.cursor].id
}

func (p *SelectPicker) HandleClick(row int) bool {
	if row < 0 || row >= p.visibleHeight() {
		return false
	}
	index := p.offset + row
	if index < 0 || index >= len(p.options) {
		return false
	}
	p.cursor = index
	p.ensureVisible()
	return true
}

func (p *SelectPicker) View() string {
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
		line := " " + opt.label
		if idx == p.cursor {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return padLines(lines, p.width)
}

func (p *SelectPicker) visibleHeight() int {
	if p.height <= 0 {
		return 0
	}
	return p.height
}

func (p *SelectPicker) ensureVisible() {
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.visibleHeight() {
		p.offset = p.cursor - p.visibleHeight() + 1
	}
	p.clampOffset()
}

func (p *SelectPicker) clampOffset() {
	if p.offset < 0 {
		p.offset = 0
	}
	maxOffset := max(0, len(p.options)-p.visibleHeight())
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
}
