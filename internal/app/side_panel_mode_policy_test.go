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

func TestDefaultSidePanelModePolicyResolveContextForSelectedSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.mode = uiModeNormal
	m.notesPanelOpen = false
	m.appState.DebugStreamsEnabled = false
	m.appState.ContextPanelHidden = false
	if got := m.activeSidePanelMode(); got != sidePanelModeContext {
		t.Fatalf("expected context mode for selected session, got %v", got)
	}
}

func TestDefaultSidePanelModePolicyResolveNoneWhenContextHidden(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.mode = uiModeNormal
	m.notesPanelOpen = false
	m.appState.DebugStreamsEnabled = false
	m.appState.ContextPanelHidden = true
	if got := m.activeSidePanelMode(); got != sidePanelModeNone {
		t.Fatalf("expected none mode when context panel hidden, got %v", got)
	}
}

func TestWithSidePanelModePolicyNilResetsToDefault(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	WithSidePanelModePolicy(stubSidePanelModePolicy{mode: sidePanelModeNotes})(&m)
	WithSidePanelModePolicy(nil)(&m)
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.mode = uiModeNormal
	if got := m.activeSidePanelMode(); got != sidePanelModeContext {
		t.Fatalf("expected reset default mode context in session view, got %v", got)
	}
}

func TestWithSidePanelModePolicyOptionNilModelNoPanic(t *testing.T) {
	opt := WithSidePanelModePolicy(nil)
	var m *Model
	opt(m)
}

func TestResolveSidePanelModeUsesPrecedenceOrder(t *testing.T) {
	if got := resolveSidePanelMode(SidePanelModeInput{
		DebugStreamsEnabled: true,
		NotesPanelOpen:      true,
		ContextPanelEnabled: true,
		HasContextSession:   true,
	}); got != sidePanelModeDebug {
		t.Fatalf("expected debug precedence, got %v", got)
	}
	if got := resolveSidePanelMode(SidePanelModeInput{
		NotesPanelOpen:      true,
		ContextPanelEnabled: true,
		HasContextSession:   true,
	}); got != sidePanelModeNotes {
		t.Fatalf("expected notes precedence over context, got %v", got)
	}
	if got := resolveSidePanelMode(SidePanelModeInput{
		ContextPanelEnabled: true,
		HasContextSession:   true,
	}); got != sidePanelModeContext {
		t.Fatalf("expected context mode, got %v", got)
	}
	if got := resolveSidePanelMode(SidePanelModeInput{
		ContextPanelEnabled: true,
		HasContextSession:   false,
	}); got != sidePanelModeNone {
		t.Fatalf("expected none without active context session, got %v", got)
	}
}

func TestSidePanelModeInputNilModel(t *testing.T) {
	var m *Model
	input := m.sidePanelModeInput()
	if input.DebugStreamsEnabled || input.NotesPanelOpen || input.ContextPanelEnabled || input.HasContextSession {
		t.Fatalf("expected zero-valued side panel mode input for nil model, got %#v", input)
	}
}
