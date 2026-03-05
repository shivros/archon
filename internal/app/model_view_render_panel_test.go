package app

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestRenderRightPaneViewUsesContextPanelInComposeMode(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.mode = uiModeCompose
	m.notesPanelOpen = true
	m.resize(180, 40)

	plain := xansi.Strip(m.renderRightPaneView())
	if !strings.Contains(plain, "Context") {
		t.Fatalf("expected context panel in compose mode, got %q", plain)
	}
}

func TestRenderRightPaneViewUsesDebugPanelWhenEnabled(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.mode = uiModeCompose
	m.appState.DebugStreamsEnabled = true
	m.resize(180, 40)

	plain := xansi.Strip(m.renderRightPaneView())
	if !strings.Contains(plain, "Debug") {
		t.Fatalf("expected debug panel when enabled, got %q", plain)
	}
}

func TestRenderRightPaneViewUsesNotesPanelOutsideCompose(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.mode = uiModeNormal
	m.notesPanelOpen = true
	m.resize(180, 40)

	plain := xansi.Strip(m.renderRightPaneView())
	if !strings.Contains(plain, "Notes") {
		t.Fatalf("expected notes panel in normal mode, got %q", plain)
	}
}

func TestContextPanelSessionIDFallsBackToSelectedSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.compose != nil {
		m.compose.SetSession("", "")
	}
	if got := m.contextPanelSessionID(); got != "s1" {
		t.Fatalf("expected selected session fallback, got %q", got)
	}
}

func TestContextPanelSessionIDNilModel(t *testing.T) {
	var m *Model
	if got := m.contextPanelSessionID(); got != "" {
		t.Fatalf("expected empty session id for nil model, got %q", got)
	}
}

func TestSessionByIDTrimsAndMisses(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if session := m.sessionByID(" s1 "); session == nil || session.ID != "s1" {
		t.Fatalf("expected trimmed lookup to match s1, got %#v", session)
	}
	if session := m.sessionByID("missing"); session != nil {
		t.Fatalf("expected miss to return nil, got %#v", session)
	}
}
