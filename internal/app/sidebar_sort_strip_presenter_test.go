package app

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestSidebarSortStripPresenterLayoutAndHitExpanded(t *testing.T) {
	p := NewSidebarSortStripPresenter()
	vm := sidebarSortStripViewModel{
		Width:          40,
		RowStart:       1,
		SortState:      sidebarSortState{Key: sidebarSortKeyCreated},
		FilterActive:   false,
		SidebarFocused: true,
		FocusedSegment: sidebarSortStripSegmentSortKey,
		FilterBadge:    "[Ctrl+F]",
		ReverseBadge:   "[Alt+R]",
	}
	layout := p.Layout(vm)
	if got := layout.height(); got != 2 {
		t.Fatalf("expected two expanded rows, got %d", got)
	}
	if layout.rowStart != 1 {
		t.Fatalf("expected rowStart 1, got %d", layout.rowStart)
	}

	if action, ok := p.Hit(layout, 1, 0); !ok || action != sidebarSortStripActionFilter {
		t.Fatalf("expected filter hit on row1 col0, got ok=%v action=%v", ok, action)
	}
	if action, ok := p.Hit(layout, 1, len(layout.rows[0].text)-1); !ok || action != sidebarSortStripActionReverse {
		t.Fatalf("expected reverse hit on row1 end, got ok=%v action=%v", ok, action)
	}
	if action, ok := p.Hit(layout, 2, 1); !ok || action != sidebarSortStripActionSortPrev {
		t.Fatalf("expected sort prev hit on row2 arrow, got ok=%v action=%v", ok, action)
	}
	nextSpan := layout.rows[1].spans[len(layout.rows[1].spans)-1]
	if action, ok := p.Hit(layout, 2, nextSpan.start); !ok || action != sidebarSortStripActionSortNext {
		t.Fatalf("expected sort next hit on row2 right arrow, got ok=%v action=%v", ok, action)
	}
	if action, ok := p.Hit(layout, 0, 0); ok || action != sidebarSortStripActionNone {
		t.Fatalf("expected miss above rowStart")
	}
}

func TestSidebarSortStripPresenterLayoutCompactAndRenderStates(t *testing.T) {
	p := NewSidebarSortStripPresenter()
	vm := sidebarSortStripViewModel{
		Width:          24,
		SortState:      sidebarSortState{Key: sidebarSortKeyActivity, Reverse: true},
		FilterActive:   true,
		SidebarFocused: true,
		FocusedSegment: sidebarSortStripSegmentFilter,
	}
	layout := p.Layout(vm)
	if got := layout.height(); got != 1 {
		t.Fatalf("expected compact single row, got %d", got)
	}
	if !strings.Contains(layout.rows[0].text, "Sort: Activity") {
		t.Fatalf("expected compact sort label, got %q", layout.rows[0].text)
	}
	rendered := xansi.Strip(p.Render(vm))
	if !strings.Contains(rendered, "Sort:") {
		t.Fatalf("expected rendered compact strip, got %q", rendered)
	}
}

func TestSidebarSortStripPresenterRenderUsesBadgesWhenNotActive(t *testing.T) {
	p := NewSidebarSortStripPresenter()
	vm := sidebarSortStripViewModel{
		Width:        40,
		SortState:    sidebarSortState{Key: sidebarSortKeyName},
		FilterBadge:  "[Ctrl+F]",
		ReverseBadge: "[Alt+R]",
	}
	rendered := p.Render(vm)
	if !strings.Contains(rendered, "[Ctrl+F]") {
		t.Fatalf("expected filter badge when filter inactive: %q", rendered)
	}
	if !strings.Contains(rendered, "[Alt+R]") {
		t.Fatalf("expected reverse badge: %q", rendered)
	}
}
