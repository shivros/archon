package app

import "testing"

func TestSidebarControllerScrollingDisabled(t *testing.T) {
	controller := NewSidebarController()

	if got := controller.ScrollbarWidth(); got != 0 {
		t.Fatalf("expected no sidebar scrollbar width when scrolling is disabled, got %d", got)
	}
	if controller.Scroll(1) {
		t.Fatalf("expected sidebar scroll to be disabled")
	}
	if controller.ScrollbarSelect(0) {
		t.Fatalf("expected sidebar scrollbar selection to be disabled")
	}
}
