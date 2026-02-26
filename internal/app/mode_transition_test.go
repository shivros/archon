package app

import "testing"

func TestApplyModeTransitionNilModelNoPanic(t *testing.T) {
	var m *Model
	m.applyModeTransition(modeTransitionRequest{
		toMode:      uiModeNormal,
		forceReflow: true,
	})
}

func TestApplyModeTransitionFocusesChatAndSetsStatus(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)

	target := focusChatInput
	m.applyModeTransition(modeTransitionRequest{
		toMode: uiModeCompose,
		status: "compose active",
		focus:  &target,
	})

	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode, got %v", m.mode)
	}
	if m.input == nil || !m.input.IsChatFocused() {
		t.Fatalf("expected chat focus after transition")
	}
	if got := m.status; got != "compose active" {
		t.Fatalf("expected status %q, got %q", "compose active", got)
	}
}

func TestApplyModeTransitionSkipsResizeWhenReflowNotNeeded(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.viewport.SetHeight(99)

	m.applyModeTransition(modeTransitionRequest{
		toMode: uiModeNormal,
	})

	if got := m.viewport.Height(); got != 99 {
		t.Fatalf("expected viewport height to remain unchanged without reflow, got %d", got)
	}
}

func TestApplyModeTransitionForceReflowSkipsResizeWhenDimensionsUnset(t *testing.T) {
	m := NewModel(nil)
	m.viewport.SetHeight(77)

	m.applyModeTransition(modeTransitionRequest{
		toMode:      uiModeNormal,
		forceReflow: true,
	})

	if got := m.viewport.Height(); got != 77 {
		t.Fatalf("expected viewport height to remain unchanged when dimensions are unset, got %d", got)
	}
}
