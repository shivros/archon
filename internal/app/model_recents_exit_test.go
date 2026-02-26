package app

import "testing"

func TestExitRecentsViewWhenNotInRecentsIsNoOp(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	initialMode := m.mode
	initialHeight := m.viewport.Height()
	m.recentsReplySessionID = "s1"

	m.exitRecentsView()

	if m.mode != initialMode {
		t.Fatalf("expected mode to remain %v, got %v", initialMode, m.mode)
	}
	if got := m.viewport.Height(); got != initialHeight {
		t.Fatalf("expected viewport height to remain %d, got %d", initialHeight, got)
	}
	if got := m.recentsReplySessionID; got != "s1" {
		t.Fatalf("expected recents reply session to remain unchanged, got %q", got)
	}
}
