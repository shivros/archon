package app

import "testing"

type testHintPolicy struct{ value string }

func (p testHintPolicy) BadgeFor(string, string, *Keybindings) string { return p.value }

type testSortPolicy struct{}

func (testSortPolicy) Normalize(state sidebarSortState) sidebarSortState {
	state.Key = sidebarSortKeyName
	return state
}
func (testSortPolicy) Cycle(state sidebarSortState, _ int) sidebarSortState {
	state.Key = sidebarSortKeyActivity
	return state
}
func (testSortPolicy) ToggleReverse(state sidebarSortState) sidebarSortState {
	state.Reverse = true
	return state
}
func (testSortPolicy) Label(sidebarSortKey) string { return "X" }

type testVisibilityPolicy struct{ show bool }

func (p testVisibilityPolicy) ShowStrip(uiMode, bool) bool { return p.show }

func TestSortPolicyOptionAndFallbacks(t *testing.T) {
	m := NewModel(nil, WithSidebarSortPolicy(testSortPolicy{}))
	if got := m.sidebarSortPolicyOrDefault().Label(sidebarSortKeyCreated); got != "X" {
		t.Fatalf("expected custom sort policy label, got %q", got)
	}
	WithSidebarSortPolicy(nil)(&m)
	if got := m.sidebarSortPolicyOrDefault().Label(sidebarSortKeyCreated); got != "Created" {
		t.Fatalf("expected default sort policy after nil reset, got %q", got)
	}
}

func TestSortHintPolicyOptionAndFallbacks(t *testing.T) {
	m := NewModel(nil, WithSortStripHintPolicy(testHintPolicy{value: "[H]"}))
	if got := m.sortStripHintPolicyOrDefault().BadgeFor("", "", nil); got != "[H]" {
		t.Fatalf("expected custom hint badge, got %q", got)
	}
	WithSortStripHintPolicy(nil)(&m)
	if got := m.sortStripHintPolicyOrDefault().BadgeFor("", "", nil); got != "" {
		t.Fatalf("expected empty badge for unbound fallback default, got %q", got)
	}
}

func TestSortVisibilityPolicyOptionAndFallbacks(t *testing.T) {
	m := NewModel(nil, WithSortStripVisibilityPolicy(testVisibilityPolicy{show: false}))
	if got := m.sortStripVisibilityPolicyOrDefault().ShowStrip(uiModeNormal, false); got {
		t.Fatalf("expected custom visibility false")
	}
	WithSortStripVisibilityPolicy(nil)(&m)
	if got := m.sortStripVisibilityPolicyOrDefault().ShowStrip(uiModeNormal, false); !got {
		t.Fatalf("expected default visibility true for normal mode")
	}
	if got := m.sortStripVisibilityPolicyOrDefault().ShowStrip(uiModeAddWorkspace, false); got {
		t.Fatalf("expected default visibility false for add-workspace mode")
	}
	if got := m.sortStripVisibilityPolicyOrDefault().ShowStrip(uiModeNormal, true); got {
		t.Fatalf("expected default visibility false when sidebar collapsed")
	}
}
