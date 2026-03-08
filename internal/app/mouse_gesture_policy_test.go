package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type testMouseGesturePolicy struct {
	linkOpen    bool
	preserve    bool
	allowHilite bool
}

func (p testMouseGesturePolicy) IsLinkOpenGesture(tea.MouseMsg) bool  { return p.linkOpen }
func (p testMouseGesturePolicy) PreservesSelection(tea.MouseMsg) bool { return p.preserve }
func (p testMouseGesturePolicy) AllowsHighlight(tea.MouseMsg) bool    { return p.allowHilite }

func TestDefaultMouseGesturePolicy(t *testing.T) {
	policy := defaultMouseGesturePolicy{}
	ctrlClick := tea.MouseClickMsg{Button: tea.MouseLeft, Mod: tea.ModCtrl, X: 1, Y: 1}
	plainClick := tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: 1}
	ctrlMotion := tea.MouseMotionMsg{Button: tea.MouseLeft, Mod: tea.ModCtrl, X: 1, Y: 1}

	if !policy.IsLinkOpenGesture(ctrlClick) {
		t.Fatalf("expected ctrl+left click to be link-open gesture")
	}
	if policy.IsLinkOpenGesture(plainClick) {
		t.Fatalf("expected plain left click not to be link-open gesture")
	}
	if !policy.PreservesSelection(ctrlClick) {
		t.Fatalf("expected ctrl+left click to preserve selection")
	}
	if policy.PreservesSelection(plainClick) {
		t.Fatalf("expected plain left click not to preserve selection")
	}
	if policy.AllowsHighlight(ctrlMotion) {
		t.Fatalf("expected ctrl-modified mouse input to block highlight")
	}
	if !policy.AllowsHighlight(plainClick) {
		t.Fatalf("expected plain click to allow highlight")
	}
}

func TestWithMouseGesturePolicyOption(t *testing.T) {
	custom := testMouseGesturePolicy{linkOpen: true, preserve: true, allowHilite: false}
	m := NewModel(nil, WithMouseGesturePolicy(custom))

	got, ok := m.mouseGesturePolicy.(testMouseGesturePolicy)
	if !ok {
		t.Fatalf("expected custom policy, got %T", m.mouseGesturePolicy)
	}
	if !got.linkOpen || !got.preserve || got.allowHilite {
		t.Fatalf("unexpected custom policy state: %#v", got)
	}

	WithMouseGesturePolicy(nil)(&m)
	if _, ok := m.mouseGesturePolicy.(defaultMouseGesturePolicy); !ok {
		t.Fatalf("expected nil option to restore default policy, got %T", m.mouseGesturePolicy)
	}
}

func TestWithMouseGesturePolicyNilModelNoop(t *testing.T) {
	WithMouseGesturePolicy(testMouseGesturePolicy{linkOpen: true})(nil)
}

func TestMouseGesturePolicyOrDefaultNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.mouseGesturePolicyOrDefault().(defaultMouseGesturePolicy); !ok {
		t.Fatalf("expected nil model to return default policy")
	}
}
