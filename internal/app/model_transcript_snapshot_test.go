package app

import (
	"context"
	"testing"
	"time"

	"control/internal/client"
	"control/internal/daemon/transcriptdomain"
)

func TestTranscriptSnapshotStaleRevisionDoesNotOverwriteVisibleBlocks(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("codex")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	m.pendingSessionKey = "sess:s1"

	handled, _ := m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("2"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "new"}},
		},
	})
	if !handled {
		t.Fatalf("expected initial transcript snapshot to be handled")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "new" {
		t.Fatalf("expected latest transcript to be new, got %q", got)
	}

	handled, _ = m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "old"}},
		},
	})
	if !handled {
		t.Fatalf("expected stale snapshot message to be handled")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "new" {
		t.Fatalf("expected stale snapshot to be ignored, got %q", got)
	}

	metrics := sink.Snapshot()
	if len(metrics) == 0 {
		t.Fatalf("expected stale revision metric")
	}
	last := metrics[len(metrics)-1]
	if last.Name != transcriptMetricStaleRevision || last.Reason != transcriptReasonSnapshotSuperseded {
		t.Fatalf("unexpected stale snapshot metric: %#v", last)
	}
}

func TestTranscriptSnapshotEqualRevisionDoesNotOverwriteVisibleBlocks(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.pendingSessionKey = "sess:s1"

	handled, _ := m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("5"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "same-rev"}},
		},
	})
	if !handled {
		t.Fatalf("expected initial snapshot to be handled")
	}
	handled, _ = m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("5"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "should-not-apply"}},
		},
	})
	if !handled {
		t.Fatalf("expected equal revision snapshot to be handled")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "same-rev" {
		t.Fatalf("expected equal revision snapshot to be ignored, got %q", got)
	}
}

type transcriptSnapshotHistoryBackfillStub struct {
	calls int
}

func (s *transcriptSnapshotHistoryBackfillStub) History(context.Context, string, int) (*client.TailItemsResponse, error) {
	s.calls++
	return &client.TailItemsResponse{
		Items: []map[string]any{
			{"type": "userMessage", "content": []any{map[string]any{"type": "text", "text": "user turn"}}},
			{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "assistant turn"}}}},
		},
	}, nil
}

func TestTranscriptSnapshotMissingUserTurnTriggersHistoryBackfill(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.pendingSessionKey = "sess:s1"
	history := &transcriptSnapshotHistoryBackfillStub{}
	m.sessionHistoryAPI = history

	handled, cmd := m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("2"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "assistant-only"}},
		},
	})
	if !handled {
		t.Fatalf("expected transcript snapshot to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected missing-user snapshot to trigger history backfill")
	}
	_ = cmd()
	if history.calls != 1 {
		t.Fatalf("expected one history backfill call, got %d", history.calls)
	}
}

func TestTranscriptSnapshotRetainsOptimisticUserEchoUntilCanonicalUserArrives(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.pendingSessionKey = "sess:s1"
	m.pendingSends[1] = pendingSend{
		key:       "sess:s1",
		sessionID: "s1",
		turnID:    "turn-1",
		state:     pendingSendStateAcknowledged,
	}
	m.optimisticSends[1] = optimisticSendEntry{
		token:      1,
		key:        "sess:s1",
		sessionID:  "s1",
		headerLine: 0,
		text:       "hello from compose",
		createdAt:  time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
		status:     ChatStatusNone,
		turnID:     "turn-1",
	}

	handled, _ := m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
			Blocks: []transcriptdomain.Block{
				{ID: "a-1", Kind: "assistant_message", Role: "assistant", Text: "assistant reply", Meta: map[string]any{"turn_id": "turn-1"}},
			},
		},
	})
	if !handled {
		t.Fatalf("expected transcript snapshot to be handled")
	}
	blocks := m.currentBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected optimistic user block to be preserved alongside assistant snapshot, got %#v", blocks)
	}
	if blocks[0].Role != ChatRoleUser || blocks[0].Text != "hello from compose" {
		t.Fatalf("expected optimistic user block to remain visible, got %#v", blocks)
	}

	handled, _ = m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("2"),
			Blocks: []transcriptdomain.Block{
				{ID: "u-1", Kind: "user_message", Role: "user", Text: "hello from compose", Meta: map[string]any{"turn_id": "turn-1"}},
				{ID: "a-1", Kind: "assistant_message", Role: "assistant", Text: "assistant reply", Meta: map[string]any{"turn_id": "turn-1"}},
			},
		},
	})
	if !handled {
		t.Fatalf("expected second transcript snapshot to be handled")
	}
	blocks = m.currentBlocks()
	userCount := 0
	for _, block := range blocks {
		if block.Role == ChatRoleUser && block.Text == "hello from compose" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("expected canonical user block to replace optimistic overlay without duplication, got %#v", blocks)
	}
	if _, ok := m.pendingSends[1]; ok {
		t.Fatalf("expected optimistic send to prune after canonical user turn appears")
	}
	if _, ok := m.optimisticSends[1]; ok {
		t.Fatalf("expected optimistic overlay entry to prune after canonical user turn appears")
	}
}
