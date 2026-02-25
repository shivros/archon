package app

import (
	"errors"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

func TestToggleDebugStreamsEnablesAndStartsActiveSessionStream(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.resize(180, 40)

	cmd := m.toggleDebugStreams()
	if !m.appState.DebugStreamsEnabled {
		t.Fatalf("expected debug streams to be enabled")
	}
	if m.status != "debug streams enabled" {
		t.Fatalf("unexpected status %q", m.status)
	}
	if !m.debugPanelVisible {
		t.Fatalf("expected debug panel to become visible")
	}
	if cmd == nil {
		t.Fatalf("expected debug stream command")
	}
}

func TestToggleDebugStreamsDisableResetsController(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.appState.DebugStreamsEnabled = true
	cancelCalls := 0
	m.debugStream.SetStream(make(chan types.DebugEvent), func() {
		cancelCalls++
	})
	m.debugStream.lines = []string{"existing"}

	cmd := m.toggleDebugStreams()
	if m.appState.DebugStreamsEnabled {
		t.Fatalf("expected debug streams to be disabled")
	}
	if m.status != "debug streams disabled" {
		t.Fatalf("unexpected status %q", m.status)
	}
	_ = cmd
	if cancelCalls != 1 {
		t.Fatalf("expected reset to cancel active stream once, got %d", cancelCalls)
	}
	if m.debugStream.HasStream() {
		t.Fatalf("expected debug stream to be cleared")
	}
	if len(m.debugStream.Lines()) != 0 {
		t.Fatalf("expected debug lines to be cleared")
	}
}

func TestApplyDebugStreamMsgSetsErrorStatus(t *testing.T) {
	m := newPhase0ModelWithSession("codex")

	m.applyDebugStreamMsg(debugStreamMsg{id: "s1", err: errors.New("boom")})

	if m.status != "debug stream error: boom" {
		t.Fatalf("unexpected status %q", m.status)
	}
	if m.toastText != "debug stream error: boom" {
		t.Fatalf("expected error toast, got %q", m.toastText)
	}
}

func TestApplyDebugStreamMsgCancelsInactiveSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	canceled := 0

	m.applyDebugStreamMsg(debugStreamMsg{
		id: "s2",
		ch: make(chan types.DebugEvent),
		cancel: func() {
			canceled++
		},
	})

	if canceled != 1 {
		t.Fatalf("expected cancel for non-active session stream, got %d", canceled)
	}
	if m.debugStream.HasStream() {
		t.Fatalf("expected debug stream to remain unchanged")
	}
}

func TestApplyDebugStreamMsgSetsActiveSessionStream(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	ch := make(chan types.DebugEvent)

	m.applyDebugStreamMsg(debugStreamMsg{id: "s1", ch: ch})

	if !m.debugStream.HasStream() {
		t.Fatalf("expected debug stream to be attached")
	}
	if m.status != "streaming debug" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestRenderDebugPanelViewShowsFallbackAndStreamLines(t *testing.T) {
	m := NewModel(nil)
	m.debugPanelWidth = 36

	fallback := xansi.Strip(m.renderDebugPanelView())
	if !strings.Contains(fallback, "Debug") || !strings.Contains(fallback, "Waiting for debug stream...") {
		t.Fatalf("unexpected fallback panel: %q", fallback)
	}

	ch := make(chan types.DebugEvent, 1)
	m.debugStream.SetStream(ch, nil)
	ch <- types.DebugEvent{Chunk: "line one\nline two\n"}
	_, changed, _ := m.debugStream.ConsumeTick()
	if !changed {
		t.Fatalf("expected debug stream tick to change lines")
	}

	panel := xansi.Strip(m.renderDebugPanelView())
	if !strings.Contains(panel, "line one") || !strings.Contains(panel, "line two") {
		t.Fatalf("expected stream lines in panel, got %q", panel)
	}
}
