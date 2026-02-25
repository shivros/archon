package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type sidebarSortStripSpan struct {
	start  int
	end    int
	action sidebarSortStripAction
}

type sidebarSortStripRow struct {
	text  string
	spans []sidebarSortStripSpan
}

type sidebarSortStripLayout struct {
	rowStart int
	rows     []sidebarSortStripRow
}

func (l sidebarSortStripLayout) height() int {
	return len(l.rows)
}

type sidebarSortStripViewModel struct {
	Width          int
	RowStart       int
	SortState      sidebarSortState
	FilterActive   bool
	SidebarFocused bool
	FocusedSegment sidebarSortStripSegment
	FilterBadge    string
	ReverseBadge   string
}

type SidebarSortStripPresenter struct{}

func NewSidebarSortStripPresenter() *SidebarSortStripPresenter {
	return &SidebarSortStripPresenter{}
}

func (p *SidebarSortStripPresenter) Render(vm sidebarSortStripViewModel) string {
	layout := p.Layout(vm)
	if len(layout.rows) == 0 {
		return ""
	}
	width := vm.Width
	if width < 1 {
		width = minListWidth
	}
	lines := make([]string, 0, len(layout.rows))
	for _, row := range layout.rows {
		line := truncateToWidth(row.text, width)
		lines = append(lines, p.renderLine(line, row, vm))
	}
	return strings.Join(lines, "\n")
}

func (p *SidebarSortStripPresenter) Layout(vm sidebarSortStripViewModel) sidebarSortStripLayout {
	width := vm.Width
	if width <= 0 {
		width = minListWidth
	}
	layout := sidebarSortStripLayout{rowStart: max(0, vm.RowStart)}
	compact := width <= 30
	if compact {
		sortLabel := sidebarSortLabel(vm.SortState.Key)
		compactLabel := "Sort: " + sortLabel + " ↔ Rev"
		layout.rows = append(layout.rows, sidebarSortStripRow{
			text: compactLabel,
			spans: []sidebarSortStripSpan{
				{start: 0, end: max(0, len("Sort")-1), action: sidebarSortStripActionFilter},
				{start: len("Sort: "), end: len("Sort: ") + len(sortLabel) + 1, action: sidebarSortStripActionSortPrev},
				{start: len(compactLabel) - len("Rev"), end: len(compactLabel) - 1, action: sidebarSortStripActionReverse},
			},
		})
		return layout
	}

	filterLabel := "Filter"
	if vm.FilterActive {
		filterLabel += " [On]"
	} else if strings.TrimSpace(vm.FilterBadge) != "" {
		filterLabel += " " + strings.TrimSpace(vm.FilterBadge)
	}
	reverseLabel := "Reverse"
	if strings.TrimSpace(vm.ReverseBadge) != "" {
		reverseLabel += " " + strings.TrimSpace(vm.ReverseBadge)
	}
	if vm.SortState.Reverse {
		reverseLabel += " [On]"
	}
	row1 := filterLabel + " | " + reverseLabel
	reverseStart := len(filterLabel) + len(" | ")
	layout.rows = append(layout.rows, sidebarSortStripRow{
		text: row1,
		spans: []sidebarSortStripSpan{
			{start: 0, end: len(filterLabel) - 1, action: sidebarSortStripActionFilter},
			{start: reverseStart, end: reverseStart + len(reverseLabel) - 1, action: sidebarSortStripActionReverse},
		},
	})
	sortLabel := sidebarSortLabel(vm.SortState.Key)
	row2 := "[←] " + sortLabel + " [→]"
	layout.rows = append(layout.rows, sidebarSortStripRow{
		text: row2,
		spans: []sidebarSortStripSpan{
			{start: 0, end: 2, action: sidebarSortStripActionSortPrev},
			{start: 6 + len(sortLabel), end: 8 + len(sortLabel), action: sidebarSortStripActionSortNext},
		},
	})
	return layout
}

func (p *SidebarSortStripPresenter) Hit(layout sidebarSortStripLayout, row, col int) (sidebarSortStripAction, bool) {
	rel := row - layout.rowStart
	if rel < 0 || rel >= len(layout.rows) {
		return sidebarSortStripActionNone, false
	}
	for _, span := range layout.rows[rel].spans {
		if col < span.start || col > span.end {
			continue
		}
		return span.action, span.action != sidebarSortStripActionNone
	}
	return sidebarSortStripActionNone, false
}

func (p *SidebarSortStripPresenter) renderLine(line string, row sidebarSortStripRow, vm sidebarSortStripViewModel) string {
	if strings.TrimSpace(line) == "" {
		return line
	}
	base := worktreeStyle.Copy().Foreground(lipgloss.Color("243"))
	out := line
	for _, span := range row.spans {
		if span.start < 0 || span.end < span.start {
			continue
		}
		label := ansi.Cut(row.text, span.start, span.end+1)
		if strings.TrimSpace(label) == "" {
			continue
		}
		style := base
		switch span.action {
		case sidebarSortStripActionFilter:
			if vm.FilterActive {
				style = selectedStyle.Copy().Background(lipgloss.Color("60"))
			} else if vm.SidebarFocused && vm.FocusedSegment == sidebarSortStripSegmentFilter {
				style = selectedStyle.Copy()
			}
		case sidebarSortStripActionReverse:
			if vm.SidebarFocused && vm.FocusedSegment == sidebarSortStripSegmentReverse {
				style = selectedStyle.Copy()
			}
		case sidebarSortStripActionSortPrev, sidebarSortStripActionSortNext:
			if vm.SidebarFocused && vm.FocusedSegment == sidebarSortStripSegmentSortKey {
				style = selectedStyle.Copy()
			}
		}
		out = strings.Replace(out, label, style.Render(label), 1)
	}
	return base.Render(out)
}
