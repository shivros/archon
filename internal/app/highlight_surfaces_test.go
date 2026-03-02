package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type stubTranscriptPort struct {
	height      int
	mouseInputY map[int]bool
	byPoint     map[[2]int]int
}

func (s stubTranscriptPort) ViewportHeight() int { return s.height }
func (s stubTranscriptPort) MouseOverInput(y int) bool {
	return s.mouseInputY[y]
}
func (s stubTranscriptPort) BlockIndexByViewportPoint(col, line int) int {
	if idx, ok := s.byPoint[[2]int{col, line}]; ok {
		return idx
	}
	return -1
}

type stubNotesPanelPort struct {
	open    bool
	visible bool
	height  int
	byPoint map[[2]int]int
}

func (s stubNotesPanelPort) NotesPanelOpen() bool          { return s.open }
func (s stubNotesPanelPort) NotesPanelVisible() bool       { return s.visible }
func (s stubNotesPanelPort) NotesPanelViewportHeight() int { return s.height }
func (s stubNotesPanelPort) NotePanelBlockIndexByViewportPoint(col, line int) int {
	if idx, ok := s.byPoint[[2]int{col, line}]; ok {
		return idx
	}
	return -1
}

type stubSidebarPort struct {
	width int
	keys  map[int]string
}

func (s stubSidebarPort) SidebarWidth() int                  { return s.width }
func (s stubSidebarPort) SidebarItemKeyAtRow(row int) string { return s.keys[row] }
func (s stubSidebarPort) SidebarHighlightedKeysBetweenRows(fromRow, toRow int) map[string]struct{} {
	if fromRow > toRow {
		fromRow, toRow = toRow, fromRow
	}
	out := map[string]struct{}{}
	for row := fromRow; row <= toRow; row++ {
		if key := s.keys[row]; key != "" {
			out[key] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func TestTranscriptHighlightSurfacePointFromMouse(t *testing.T) {
	surface := NewTranscriptHighlightSurface(stubTranscriptPort{
		height: 20,
		byPoint: map[[2]int]int{
			{2, 3}: 4,
		},
	})
	point, ok := surface.PointFromMouse(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 12, Y: 4}, mouseLayout{rightStart: 10})
	if !ok {
		t.Fatalf("expected transcript hit")
	}
	if point.BlockIndex != 4 {
		t.Fatalf("unexpected point: %#v", point)
	}
}

func TestNotesPanelHighlightSurfacePointFromMouse(t *testing.T) {
	surface := NewNotesPanelHighlightSurface(stubNotesPanelPort{
		open:    true,
		visible: true,
		height:  10,
		byPoint: map[[2]int]int{{1, 1}: 2},
	})
	point, ok := surface.PointFromMouse(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 31, Y: 2}, mouseLayout{panelVisible: true, panelStart: 30, panelWidth: 20})
	if !ok {
		t.Fatalf("expected panel hit")
	}
	if point.BlockIndex != 2 {
		t.Fatalf("unexpected point: %#v", point)
	}
}

func TestNotesPanelHighlightSurfaceRangeFromPoints(t *testing.T) {
	surface := NewNotesPanelHighlightSurface(stubNotesPanelPort{
		open:    true,
		visible: true,
		height:  8,
		byPoint: map[[2]int]int{{1, 1}: 2},
	})
	rangeState, ok := surface.RangeFromPoints(highlightPoint{BlockIndex: 5}, highlightPoint{BlockIndex: 2})
	if !ok {
		t.Fatalf("expected notes panel range")
	}
	if rangeState.BlockStart != 2 || rangeState.BlockEnd != 5 {
		t.Fatalf("unexpected notes panel range: %#v", rangeState)
	}
}

func TestTranscriptHighlightSurfacePointFromMouseRejectsInputArea(t *testing.T) {
	surface := NewTranscriptHighlightSurface(stubTranscriptPort{
		height:      20,
		mouseInputY: map[int]bool{4: true},
		byPoint:     map[[2]int]int{{2, 3}: 4},
	})
	if _, ok := surface.PointFromMouse(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 12, Y: 4}, mouseLayout{rightStart: 10}); ok {
		t.Fatalf("expected transcript miss when hovering input area")
	}
}

func TestSidebarHighlightSurfacePointFromMouseRejectsScrollbar(t *testing.T) {
	surface := NewSidebarHighlightSurface(stubSidebarPort{
		width: 20,
		keys:  map[int]string{2: "sess:s1"},
	})
	if _, ok := surface.PointFromMouse(
		tea.MouseMotionMsg{Button: tea.MouseLeft, X: 19, Y: 2},
		mouseLayout{listWidth: 20, barStart: 19, barWidth: 1},
	); ok {
		t.Fatalf("expected sidebar miss in scrollbar region")
	}
}

func TestSidebarHighlightSurfaceRangeFromPoints(t *testing.T) {
	surface := NewSidebarHighlightSurface(stubSidebarPort{
		width: 20,
		keys: map[int]string{
			2: "sess:s1",
			3: "sess:s2",
		},
	})
	rangeState, ok := surface.RangeFromPoints(highlightPoint{SidebarRow: 2}, highlightPoint{SidebarRow: 3})
	if !ok {
		t.Fatalf("expected sidebar range")
	}
	if len(rangeState.SidebarKeys) != 2 {
		t.Fatalf("unexpected keys: %#v", rangeState.SidebarKeys)
	}
}

func TestTranscriptHighlightSurfaceRangeFromPointsRejectsInvalid(t *testing.T) {
	surface := NewTranscriptHighlightSurface(stubTranscriptPort{})
	if _, ok := surface.RangeFromPoints(highlightPoint{BlockIndex: -1}, highlightPoint{BlockIndex: 3}); ok {
		t.Fatalf("expected invalid anchor to be rejected")
	}
}

func TestSidebarHighlightSurfaceRangeFromPointsRejectsEmptySelection(t *testing.T) {
	surface := NewSidebarHighlightSurface(stubSidebarPort{
		width: 20,
		keys:  map[int]string{},
	})
	if _, ok := surface.RangeFromPoints(highlightPoint{SidebarRow: 1}, highlightPoint{SidebarRow: 3}); ok {
		t.Fatalf("expected empty sidebar selection to be rejected")
	}
}
