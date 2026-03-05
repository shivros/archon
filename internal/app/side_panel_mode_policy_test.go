package app

import "testing"

func TestDefaultSidePanelModePolicyResolveNilModel(t *testing.T) {
	policy := defaultSidePanelModePolicy{}
	if got := policy.Resolve(nil); got != sidePanelModeNone {
		t.Fatalf("expected none mode for nil model, got %v", got)
	}
}

func TestDefaultSidePanelModePolicyResolveNoneFallback(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNormal
	m.notesPanelOpen = false
	m.appState.DebugStreamsEnabled = false
	if got := m.activeSidePanelMode(); got != sidePanelModeNone {
		t.Fatalf("expected none mode fallback, got %v", got)
	}
}

func TestWithSidePanelModePolicyNilResetsToDefault(t *testing.T) {
	m := NewModel(nil,
		WithSidePanelModePolicy(stubSidePanelModePolicy{mode: sidePanelModeNotes}),
		WithSidePanelModePolicy(nil),
	)
	m.mode = uiModeCompose
	if got := m.activeSidePanelMode(); got != sidePanelModeContext {
		t.Fatalf("expected reset default mode context in compose, got %v", got)
	}
}

func TestWithSidePanelModePolicyOptionNilModelNoPanic(t *testing.T) {
	opt := WithSidePanelModePolicy(nil)
	var m *Model
	opt(m)
}
