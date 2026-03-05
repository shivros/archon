package app

import (
	"testing"

	"control/internal/types"
)

func TestPanelLayoutPersistencePolicySupportsAllPersistedModes(t *testing.T) {
	state := &types.AppState{}
	cases := []struct {
		mode  sidePanelMode
		width int
	}{
		{mode: sidePanelModeNotes, width: 33},
		{mode: sidePanelModeDebug, width: 37},
		{mode: sidePanelModeContext, width: 41},
	}
	for _, tc := range cases {
		policy := panelLayoutPersistencePolicy(tc.mode)
		if !policy.SetWidth(state, tc.width) {
			t.Fatalf("expected width set for mode %v", tc.mode)
		}
		if got := policy.Width(state); got != tc.width {
			t.Fatalf("expected width %d for mode %v, got %d", tc.width, tc.mode, got)
		}
	}
}

func TestPanelLayoutPersistencePolicyUnknownModeIsNoOp(t *testing.T) {
	state := &types.AppState{NotesPanelWidth: 35}
	policy := panelLayoutPersistencePolicy(sidePanelModeNone)
	if policy.SetWidth(state, 99) {
		t.Fatalf("expected no-op set for unsupported panel mode")
	}
	if got := policy.Width(state); got != 0 {
		t.Fatalf("expected no-op width 0 for unsupported mode, got %d", got)
	}
	if state.NotesPanelWidth != 35 {
		t.Fatalf("expected unrelated panel widths to remain unchanged")
	}
}

func TestPanelWidthAccessorsGuardNilStateAndNoopSet(t *testing.T) {
	accessors := panelWidthAccessors{
		get: func(state *types.AppState) int { return state.NotesPanelWidth },
		set: func(state *types.AppState, width int) { state.NotesPanelWidth = width },
	}
	if got := accessors.Width(nil); got != 0 {
		t.Fatalf("expected width 0 for nil state, got %d", got)
	}
	state := &types.AppState{NotesPanelWidth: 28}
	if changed := accessors.SetWidth(state, 28); changed {
		t.Fatalf("expected no change when setting same value")
	}
	if changed := accessors.SetWidth(nil, 30); changed {
		t.Fatalf("expected no change for nil state")
	}
}

func TestPanelWidthAccessorsGuardNilGetSet(t *testing.T) {
	state := &types.AppState{NotesPanelWidth: 30}
	getNil := panelWidthAccessors{
		set: func(state *types.AppState, width int) { state.NotesPanelWidth = width },
	}
	if got := getNil.Width(state); got != 0 {
		t.Fatalf("expected width 0 when getter is nil, got %d", got)
	}
	if changed := getNil.SetWidth(state, 31); changed {
		t.Fatalf("expected no change when getter is nil")
	}

	setNil := panelWidthAccessors{
		get: func(state *types.AppState) int { return state.NotesPanelWidth },
	}
	if changed := setNil.SetWidth(state, 31); changed {
		t.Fatalf("expected no change when setter is nil")
	}
}
