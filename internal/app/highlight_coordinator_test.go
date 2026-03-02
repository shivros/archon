package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type stubHighlightModeSource struct{ mode uiMode }

func (s stubHighlightModeSource) CurrentUIMode() uiMode { return s.mode }

type stubHighlightSurface struct {
	ctx         highlightContext
	pointByY    map[int]highlightPoint
	rangeResult highlightRange
	rangeOK     bool
}

func (s stubHighlightSurface) Context() highlightContext { return s.ctx }

func (s stubHighlightSurface) PointFromMouse(msg tea.MouseMsg, _ mouseLayout) (highlightPoint, bool) {
	mouse := msg.Mouse()
	point, ok := s.pointByY[mouse.Y]
	return point, ok
}

func (s stubHighlightSurface) RangeFromPoints(anchor, focus highlightPoint) (highlightRange, bool) {
	if !s.rangeOK {
		return highlightRange{}, false
	}
	out := s.rangeResult
	if out.BlockStart == 0 && out.BlockEnd == 0 {
		start := anchor.BlockIndex
		end := focus.BlockIndex
		if start > end {
			start, end = end, start
		}
		out.BlockStart = start
		out.BlockEnd = end
	}
	return out, true
}

func TestHighlightCoordinatorLocksContextAndBuildsRange(t *testing.T) {
	coord := NewDefaultHighlightCoordinator(
		stubHighlightModeSource{mode: uiModeNormal},
		NewDefaultHighlightContextPolicy(),
		stubHighlightSurface{
			ctx: highlightContextChatTranscript,
			pointByY: map[int]highlightPoint{
				2: {BlockIndex: 1},
				5: {BlockIndex: 3},
			},
			rangeOK: true,
		},
	)
	if !coord.Begin(tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 2}, mouseLayout{}) {
		t.Fatalf("expected begin to activate")
	}
	if !coord.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 10, Y: 5}, mouseLayout{}) {
		t.Fatalf("expected update to register drag")
	}
	if !coord.End(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: 10, Y: 5}, mouseLayout{}) {
		t.Fatalf("expected end to commit drag")
	}
	state := coord.State()
	if !state.Range.HasSelection {
		t.Fatalf("expected persisted range")
	}
	if state.Range.Context != highlightContextChatTranscript {
		t.Fatalf("unexpected context: %v", state.Range.Context)
	}
	if state.Range.BlockStart != 1 || state.Range.BlockEnd != 3 {
		t.Fatalf("unexpected range: %#v", state.Range)
	}
}

func TestHighlightCoordinatorRespectsPolicy(t *testing.T) {
	coord := NewDefaultHighlightCoordinator(
		stubHighlightModeSource{mode: uiModeNotes},
		NewDefaultHighlightContextPolicy(),
		stubHighlightSurface{
			ctx: highlightContextChatTranscript,
			pointByY: map[int]highlightPoint{
				2: {BlockIndex: 1},
			},
			rangeOK: true,
		},
	)
	if coord.Begin(tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 2}, mouseLayout{}) {
		t.Fatalf("expected begin denied by context policy")
	}
}

func TestHighlightCoordinatorClearLifecycle(t *testing.T) {
	coord := NewDefaultHighlightCoordinator(
		stubHighlightModeSource{mode: uiModeNormal},
		NewDefaultHighlightContextPolicy(),
		stubHighlightSurface{
			ctx: highlightContextChatTranscript,
			pointByY: map[int]highlightPoint{
				2: {BlockIndex: 1},
				4: {BlockIndex: 2},
			},
			rangeOK: true,
		},
	)
	if coord.Clear() {
		t.Fatalf("expected clear to report no-op when state is empty")
	}
	if !coord.Begin(tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 2}, mouseLayout{}) {
		t.Fatalf("expected begin to activate")
	}
	if !coord.Clear() {
		t.Fatalf("expected clear to reset active state")
	}
	state := coord.State()
	if state.Active || state.Context != highlightContextNone || state.Range.HasSelection {
		t.Fatalf("expected empty state after clear, got %#v", state)
	}
}

func TestHighlightCoordinatorStateClonesSidebarKeys(t *testing.T) {
	coord := NewDefaultHighlightCoordinator(
		stubHighlightModeSource{mode: uiModeNormal},
		NewDefaultHighlightContextPolicy(),
		stubHighlightSurface{
			ctx: highlightContextSidebar,
			pointByY: map[int]highlightPoint{
				2: {SidebarRow: 2, SidebarKey: "sess:s1"},
				3: {SidebarRow: 3, SidebarKey: "sess:s2"},
			},
			rangeOK: true,
			rangeResult: highlightRange{
				SidebarKeys: map[string]struct{}{
					"sess:s1": {},
					"sess:s2": {},
				},
			},
		},
	)
	if !coord.Begin(tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: 2}, mouseLayout{}) {
		t.Fatalf("expected begin to activate")
	}
	if !coord.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 1, Y: 3}, mouseLayout{}) {
		t.Fatalf("expected update to set sidebar range")
	}
	state := coord.State()
	delete(state.Range.SidebarKeys, "sess:s1")
	again := coord.State()
	if _, ok := again.Range.SidebarKeys["sess:s1"]; !ok {
		t.Fatalf("expected sidebar keys in state to be cloned")
	}
}

func TestHighlightCoordinatorEndWithoutDragClearsState(t *testing.T) {
	coord := NewDefaultHighlightCoordinator(
		stubHighlightModeSource{mode: uiModeNormal},
		NewDefaultHighlightContextPolicy(),
		stubHighlightSurface{
			ctx: highlightContextChatTranscript,
			pointByY: map[int]highlightPoint{
				2: {BlockIndex: 1},
			},
			rangeOK: true,
		},
	)
	if !coord.Begin(tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 2}, mouseLayout{}) {
		t.Fatalf("expected begin to activate")
	}
	if coord.End(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: 10, Y: 2}, mouseLayout{}) {
		t.Fatalf("expected click-without-drag to return false")
	}
	state := coord.State()
	if state.Active || state.Range.HasSelection {
		t.Fatalf("expected state reset after non-drag release, got %#v", state)
	}
}

func TestHighlightCoordinatorUpdateRangeFailureClearsSelection(t *testing.T) {
	coord := NewDefaultHighlightCoordinator(
		stubHighlightModeSource{mode: uiModeNormal},
		NewDefaultHighlightContextPolicy(),
		stubHighlightSurface{
			ctx: highlightContextChatTranscript,
			pointByY: map[int]highlightPoint{
				2: {BlockIndex: 1},
				4: {BlockIndex: 3},
			},
			rangeOK: false,
		},
	)
	if !coord.Begin(tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 2}, mouseLayout{}) {
		t.Fatalf("expected begin to activate")
	}
	if !coord.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: 10, Y: 4}, mouseLayout{}) {
		t.Fatalf("expected focus update with range failure")
	}
	state := coord.State()
	if state.Range.HasSelection || state.Range.BlockStart != 0 || state.Range.BlockEnd != 0 {
		t.Fatalf("expected empty range when range resolution fails, got %#v", state.Range)
	}
}

func TestHighlightCoordinatorFallsBackWhenPolicyNilInConstructor(t *testing.T) {
	coord := NewDefaultHighlightCoordinator(
		stubHighlightModeSource{mode: uiModeNormal},
		nil,
		stubHighlightSurface{
			ctx: highlightContextChatTranscript,
			pointByY: map[int]highlightPoint{
				2: {BlockIndex: 1},
			},
			rangeOK: true,
		},
	)
	if !coord.Begin(tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 2}, mouseLayout{}) {
		t.Fatalf("expected begin allowed when constructor receives nil policy")
	}
}
