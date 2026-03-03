package app

import (
	"context"
	"errors"
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestApplyTranscriptStreamMsgErrorRecordsReconnectError(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("codex")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	m.recordReconnectAttempt("s1", "codex", "transcript", transcriptSourceSendMsg)

	m.applyTranscriptStreamMsg(transcriptStreamMsg{
		id:  "s1",
		err: errors.New("stream failed"),
	})

	metrics := sink.Snapshot()
	if len(metrics) == 0 {
		t.Fatalf("expected reconnect error metric")
	}
	last := metrics[len(metrics)-1]
	if last.Name != transcriptMetricReconnect || last.Outcome != transcriptOutcomeError || last.Reason != transcriptReasonReconnectStreamError || last.Stream != "transcript" {
		t.Fatalf("unexpected reconnect error metric: %#v", last)
	}
}

func TestApplyTranscriptStreamMsgMismatchedSessionSkipsAndCancels(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("codex")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	m.recordReconnectAttempt("s2", "codex", "transcript", transcriptSourceSendMsg)

	canceled := false
	m.applyTranscriptStreamMsg(transcriptStreamMsg{
		id: "s2",
		ch: make(chan transcriptdomain.TranscriptEvent),
		cancel: func() {
			canceled = true
		},
	})

	if !canceled {
		t.Fatalf("expected mismatched stream to cancel")
	}
	metrics := sink.Snapshot()
	last := metrics[len(metrics)-1]
	if last.Name != transcriptMetricReconnect || last.Outcome != transcriptOutcomeSkipped || last.Reason != transcriptReasonReconnectMismatchedSession {
		t.Fatalf("unexpected skipped reconnect metric: %#v", last)
	}
}

func TestApplyTranscriptStreamMsgReconnectReasonByRevision(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("codex")
	WithTranscriptBoundaryMetricsSink(sink)(&m)

	m.recordReconnectAttempt("s1", "codex", "transcript", transcriptSourceSendMsg)
	m.applyTranscriptStreamMsg(transcriptStreamMsg{
		id:       "s1",
		ch:       make(chan transcriptdomain.TranscriptEvent),
		revision: "6",
	})
	metrics := sink.Snapshot()
	last := metrics[len(metrics)-1]
	if last.Reason != transcriptReasonReconnectStreamAttached {
		t.Fatalf("expected stream_attached for revision reconnect, got %#v", last)
	}

	m.recordReconnectAttempt("s1", "codex", "transcript", transcriptSourceSendMsg)
	m.applyTranscriptStreamMsg(transcriptStreamMsg{
		id: "s1",
		ch: make(chan transcriptdomain.TranscriptEvent),
	})
	metrics = sink.Snapshot()
	last = metrics[len(metrics)-1]
	if last.Reason != transcriptReasonReconnectMatchedSession {
		t.Fatalf("expected matched_active_session when revision is empty, got %#v", last)
	}
}

func TestApplyTranscriptSnapshotMsgCanceledErrorAndKeyMismatchAreNoops(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"

	cmd := m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		err: context.Canceled,
	})
	if cmd != nil {
		t.Fatalf("expected no follow-up command on canceled snapshot error")
	}
	if !m.loading {
		t.Fatalf("expected canceled snapshot not to clear loading")
	}

	cmd = m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s2",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "ignored"}},
		},
	})
	if cmd != nil {
		t.Fatalf("expected no follow-up command for key mismatch")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got == "ignored" {
		t.Fatalf("expected key mismatch snapshot not to apply blocks")
	}
}
