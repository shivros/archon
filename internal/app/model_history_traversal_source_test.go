package app

import (
	"context"
	"testing"

	"control/internal/client"
	"control/internal/daemon/transcriptdomain"
)

type historyTraversalSourceTranscriptAPIStub struct {
	calls int
}

func (s *historyTraversalSourceTranscriptAPIStub) GetTranscriptSnapshot(context.Context, string, int) (*client.TranscriptSnapshotResponse, error) {
	s.calls++
	return &client.TranscriptSnapshotResponse{}, nil
}

func (*historyTraversalSourceTranscriptAPIStub) TranscriptStream(context.Context, string, string) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	ch := make(chan transcriptdomain.TranscriptEvent)
	return ch, func() {}, nil
}

type historyTraversalSourceHistoryAPIStub struct {
	calls int
}

func (s *historyTraversalSourceHistoryAPIStub) History(context.Context, string, int) (*client.TailItemsResponse, error) {
	s.calls++
	return &client.TailItemsResponse{Items: []map[string]any{}}, nil
}

func TestModelTraversalPrefersHistoryAPIOverTranscriptSnapshot(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	historyAPI := &historyTraversalSourceHistoryAPIStub{}
	transcriptAPI := &historyTraversalSourceTranscriptAPIStub{}
	m.sessionHistoryAPI = historyAPI
	m.sessionTranscriptAPI = transcriptAPI
	m.mode = uiModeCompose
	m.viewport.SetYOffset(0)

	cmd := m.maybeRequestOlderHistoryOnTop()
	if cmd == nil {
		t.Fatalf("expected traversal command")
	}
	_ = cmd()
	if historyAPI.calls != 1 {
		t.Fatalf("expected history API traversal request, got %d", historyAPI.calls)
	}
	if transcriptAPI.calls != 0 {
		t.Fatalf("expected transcript snapshot API to be skipped for traversal, got %d", transcriptAPI.calls)
	}
}
