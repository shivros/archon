package app

import (
	"context"
	"testing"

	"control/internal/client"
)

type historyTraversalHistoryAPIStub struct {
	lines []int
}

func (s *historyTraversalHistoryAPIStub) History(_ context.Context, _ string, lines int) (*client.TailItemsResponse, error) {
	s.lines = append(s.lines, lines)
	return &client.TailItemsResponse{}, nil
}

func TestModelRequestsOlderHistoryAtViewportTop(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := &historyTraversalHistoryAPIStub{}
	m.sessionHistoryAPI = api
	m.mode = uiModeCompose
	m.viewport.SetYOffset(0)

	cmd := m.maybeRequestOlderHistoryOnTop()
	if cmd == nil {
		t.Fatalf("expected older-history command at viewport top")
	}
	_ = cmd()
	if len(api.lines) != 1 {
		t.Fatalf("expected one transcript snapshot request, got %d", len(api.lines))
	}
	want := defaultInitialHistoryLines + historyTraverseStepLines
	if api.lines[0] != want {
		t.Fatalf("expected first traversal request to ask for %d lines, got %d", want, api.lines[0])
	}

	if cmd := m.maybeRequestOlderHistoryOnTop(); cmd != nil {
		t.Fatalf("expected in-flight traversal request to prevent duplicate fetch")
	}

	// Simulate a full-window response so traversal can continue.
	_, _ = m.reduceStateMessages(historyMsg{
		id:             "s1",
		key:            "sess:s1",
		requestedLines: want,
		items:          make([]map[string]any, want),
	})
	m.viewport.SetYOffset(0)

	cmd = m.maybeRequestOlderHistoryOnTop()
	if cmd == nil {
		t.Fatalf("expected next traversal request after prior completion")
	}
	_ = cmd()
	if len(api.lines) != 2 {
		t.Fatalf("expected two transcript snapshot requests, got %d", len(api.lines))
	}
	if api.lines[1] <= api.lines[0] {
		t.Fatalf("expected traversal window to grow, got %d then %d", api.lines[0], api.lines[1])
	}
}
