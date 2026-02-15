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
	options []groupOption
}

type groupOption struct {
	id       string
	label    string
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
	if p.cursor >= len(p.options) {
		p.cursor = max(0, len(p.options)-1)
	}
	p.ensureVisible()
}

func (p *GroupPicker) Move(delta int) bool {
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

func (p *GroupPicker) Toggle() bool {
	if p.cursor < 0 || p.cursor >= len(p.options) {
		return false
	}
	p.options[p.cursor].selected = !p.options[p.cursor].selected
	return true
}

func (p *GroupPicker) HandleClick(row int) bool {
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

func (p *GroupPicker) SelectedIDs() []string {
	out := make([]string, 0, len(p.options))
	for _, opt := range p.options {
		if opt.selected {
			out = append(out, opt.id)
		}
	}
	return out
}

func (p *GroupPicker) View() string {
	if p.height <= 0 {
		return ""
	}
	lines := make([]string, 0, p.visibleHeight())
	if len(p.options) == 0 {
		lines = append(lines, " (no groups)")
		return padBlock(lines, p.width)
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
	return padBlock(lines, p.width)
}

func (p *GroupPicker) visibleHeight() int {
	if p.height <= 0 {
		return 0
	}
	return p.height
}

func (p *GroupPicker) ensureVisible() {
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.visibleHeight() {
		p.offset = p.cursor - p.visibleHeight() + 1
	}
	p.clampOffset()
}

func (p *GroupPicker) clampOffset() {
	if p.offset < 0 {
		p.offset = 0
	}
	maxOffset := max(0, len(p.options)-p.visibleHeight())
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
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
