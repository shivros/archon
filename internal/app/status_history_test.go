package app

import (
	"sort"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestStatusHistoryStoreAppliesCapAndAdjacentDedupe(t *testing.T) {
	store := newStatusHistoryStore(3)
	store.Append("one")
	store.Append("one")
	store.Append("two")
	store.Append("three")
	store.Append("four")

	got := store.SnapshotNewestFirst()
	if len(got) != 3 {
		t.Fatalf("expected capped history length 3, got %d", len(got))
	}
	if got[0] != "four" || got[1] != "three" || got[2] != "two" {
		t.Fatalf("unexpected ordering/content: %#v", got)
	}
}

func TestStatusHistoryPresenterTruncatesRowsAndExposesCopyHitbox(t *testing.T) {
	presenter := newDefaultStatusHistoryOverlayPresenter(defaultStatusHistoryOverlayConfig())
	long := strings.Repeat("a", 100)
	view := presenter.Render(statusHistoryOverlayRenderInput{
		entries:       []string{long, "short"},
		selectedIndex: 0,
		scrollOffset:  0,
		width:         120,
		rightStart:    30,
		bodyHeight:    20,
	})
	if strings.TrimSpace(view.block) == "" {
		t.Fatalf("expected rendered overlay block")
	}
	plain := xansi.Strip(view.block)
	if !strings.Contains(plain, strings.Repeat("a", 63)+"…") {
		t.Fatalf("expected history row truncation to 64 characters")
	}
	if !view.hitbox.copyAvailable || view.hitbox.copyRowY <= 0 {
		t.Fatalf("expected copy hitbox when entry is selected")
	}
}

func TestStatusHistoryControllerMoveAndScrollBoundaries(t *testing.T) {
	controller := newStatusHistoryOverlayController()
	if controller.Move(1, 0, 3) {
		t.Fatalf("expected move to ignore empty totals")
	}
	if controller.Scroll(1, 0, 3) {
		t.Fatalf("expected scroll to ignore empty totals")
	}
	controller.Open()
	if !controller.IsOpen() {
		t.Fatalf("expected controller to be open")
	}
	if !controller.Move(1, 5, 3) {
		t.Fatalf("expected first move to select first row")
	}
	if got := controller.SelectedIndex(); got != 0 {
		t.Fatalf("expected selected index 0, got %d", got)
	}
	controller.selectedIndex = -1
	if !controller.Move(-1, 5, 3) {
		t.Fatalf("expected upward move from no selection to jump to last row")
	}
	if got := controller.SelectedIndex(); got != 4 {
		t.Fatalf("expected selected index 4, got %d", got)
	}
	if controller.Select(4, 5, 3) {
		t.Fatalf("expected selecting current row to report no change")
	}
	if !controller.Select(3, 5, 3) {
		t.Fatalf("expected selecting new row to report change")
	}
	if got := controller.ScrollOffset(); got != 2 {
		t.Fatalf("expected ensureVisible to keep offset at 2, got %d", got)
	}
	if !controller.Scroll(-10, 5, 3) {
		t.Fatalf("expected scroll up with clamp to be treated as change")
	}
	if got := controller.ScrollOffset(); got != 0 {
		t.Fatalf("expected clamped scroll offset 0, got %d", got)
	}
	controller.Reconcile(0, 3)
	if got := controller.SelectedIndex(); got != -1 {
		t.Fatalf("expected reconcile on empty to clear selection, got %d", got)
	}
}

func TestStatusHistoryPresenterLayoutEdgeCases(t *testing.T) {
	presenter := newDefaultStatusHistoryOverlayPresenter(defaultStatusHistoryOverlayConfig())
	tooNarrow := presenter.Render(statusHistoryOverlayRenderInput{
		entries:       []string{"one"},
		selectedIndex: -1,
		scrollOffset:  0,
		width:         20,
		rightStart:    10,
		bodyHeight:    10,
	})
	if strings.TrimSpace(tooNarrow.block) != "" {
		t.Fatalf("expected no overlay when width is below minimum")
	}

	clipped := presenter.Render(statusHistoryOverlayRenderInput{
		entries:       []string{"one", "two", "three", "four"},
		selectedIndex: 3,
		scrollOffset:  2,
		width:         50,
		rightStart:    0,
		bodyHeight:    3,
	})
	if strings.TrimSpace(clipped.block) == "" {
		t.Fatalf("expected clipped overlay to still render")
	}
	if clipped.row != 0 {
		t.Fatalf("expected clipped overlay to anchor at row 0, got %d", clipped.row)
	}
}

func TestStatusHistoryHitboxTolerances(t *testing.T) {
	hit := statusHistoryOverlayHitbox{
		panelLeftX:    10,
		panelRightX:   20,
		panelTopY:     5,
		panelBottomY:  8,
		listIndexByY:  map[int]int{6: 2},
		copyRowY:      8,
		copyStartX:    12,
		copyEndX:      18,
		copyAvailable: true,
	}
	if !hit.contains(10, 5) {
		t.Fatalf("expected exact bounds to be contained")
	}
	if !hit.contains(11, 6) {
		t.Fatalf("expected in-bounds point to be contained")
	}
	if idx, ok := hit.listIndexAt(6); !ok || idx != 2 {
		t.Fatalf("expected exact row to resolve list index, got idx=%d ok=%v", idx, ok)
	}
	if _, ok := hit.listIndexAt(7); ok {
		t.Fatalf("expected adjacent row miss to avoid selecting a neighboring entry")
	}
	if hit.copyContains(12, 7) {
		t.Fatalf("expected copy hitbox to reject adjacent in-range row")
	}
	if !hit.copyContains(12, 9) {
		t.Fatalf("expected copy row to accept bottom boundary clamp")
	}

	boundary := statusHistoryOverlayHitbox{
		panelTopY:    5,
		panelBottomY: 8,
		listIndexByY: map[int]int{8: 3},
	}
	if idx, ok := boundary.listIndexAt(9); !ok || idx != 3 {
		t.Fatalf("expected bottom boundary clamp to resolve last row, got idx=%d ok=%v", idx, ok)
	}
	if statusHistoryMouseRowInRange(4, 5, 8) || !statusHistoryMouseRowInRange(9, 5, 8) {
		t.Fatalf("expected bottom-only one-row boundary clamp around panel rows")
	}
}

func TestStatusHistoryHitboxListIndexAtGuards(t *testing.T) {
	empty := statusHistoryOverlayHitbox{}
	if _, ok := empty.listIndexAt(5); ok {
		t.Fatalf("expected empty list index map to miss")
	}

	hit := statusHistoryOverlayHitbox{
		panelTopY:    5,
		panelBottomY: 8,
		listIndexByY: map[int]int{6: 2},
	}
	if _, ok := hit.listIndexAt(2); ok {
		t.Fatalf("expected out-of-panel row to miss")
	}
}

func TestStatusHistoryHitboxCopyContainsGuards(t *testing.T) {
	hit := statusHistoryOverlayHitbox{
		panelTopY:     5,
		panelBottomY:  8,
		copyRowY:      8,
		copyStartX:    12,
		copyEndX:      18,
		copyAvailable: false,
	}
	if hit.copyContains(12, 8) {
		t.Fatalf("expected copy hit to reject when copy control is unavailable")
	}

	hit.copyAvailable = true
	hit.copyRowY = -1
	if hit.copyContains(12, 8) {
		t.Fatalf("expected negative copy row to be rejected")
	}

	hit.copyRowY = 8
	if hit.copyContains(11, 8) {
		t.Fatalf("expected out-of-range copy X to be rejected")
	}
	if hit.copyContains(12, 3) {
		t.Fatalf("expected far-out copy Y to be rejected")
	}
}

func TestStatusHistoryPresenterHitboxMappingWhenClipped(t *testing.T) {
	presenter := newDefaultStatusHistoryOverlayPresenter(defaultStatusHistoryOverlayConfig())
	view := presenter.Render(statusHistoryOverlayRenderInput{
		entries:       []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight"},
		selectedIndex: -1,
		scrollOffset:  0,
		width:         120,
		rightStart:    0,
		bodyHeight:    3,
	})
	if strings.TrimSpace(view.block) == "" {
		t.Fatalf("expected clipped overlay to render")
	}
	if len(view.hitbox.listIndexByY) == 0 {
		t.Fatalf("expected clipped overlay to still expose list hit targets")
	}

	ys := make([]int, 0, len(view.hitbox.listIndexByY))
	for y := range view.hitbox.listIndexByY {
		ys = append(ys, y)
	}
	sort.Ints(ys)
	for i, y := range ys {
		if y < view.hitbox.panelTopY || y > view.hitbox.panelBottomY {
			t.Fatalf("expected mapped list row %d to stay within panel bounds [%d,%d]", y, view.hitbox.panelTopY, view.hitbox.panelBottomY)
		}
		if i > 0 && y != ys[i-1]+1 {
			t.Fatalf("expected mapped list rows to remain contiguous when clipped, got %d then %d", ys[i-1], y)
		}
		idx, ok := view.hitbox.listIndexAt(y)
		if !ok {
			t.Fatalf("expected mapped clipped row %d to resolve", y)
		}
		if want := view.hitbox.listIndexByY[y]; idx != want {
			t.Fatalf("expected clipped row %d to resolve index %d, got %d", y, want, idx)
		}
	}
}
