package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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

func TestApplyTranscriptSnapshotMsgErrorClearsLoadingAndOpensFollow(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"

	cmd := m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:     "s1",
		key:    "sess:s1",
		source: transcriptAttachmentSourceSelectionLoad,
		err:    errors.New("boom"),
	})
	if cmd == nil {
		t.Fatalf("expected non-canceled snapshot error to open follow stream")
	}
	if m.loading || m.loadingKey != "" {
		t.Fatalf("expected matching loading key to clear loading state after error")
	}
	if !strings.Contains(strings.ToLower(m.status), "transcript snapshot error") {
		t.Fatalf("expected status to include snapshot error, got %q", m.status)
	}

	raw := cmd()
	stream, ok := raw.(transcriptStreamMsg)
	if !ok {
		t.Fatalf("expected follow-open command to emit transcriptStreamMsg, got %T", raw)
	}
	if stream.id != "s1" || stream.generation == 0 {
		t.Fatalf("expected generation-aware follow open for s1, got %#v", stream)
	}
}

func TestApplyTranscriptSnapshotMsgErrorWithMismatchedKeyDropsResponse(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"

	cmd := m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:     "s1",
		key:    "sess:other",
		source: transcriptAttachmentSourceSelectionLoad,
		err:    errors.New("boom"),
	})
	if cmd != nil {
		t.Fatalf("expected mismatched-key snapshot error to be dropped")
	}
	if !m.loading || m.loadingKey != "sess:s1" {
		t.Fatalf("expected dropped mismatched-key error not to touch loading state")
	}
}

func TestApplyTranscriptSnapshotMsgSelectionLoadOpensFollowFromSnapshotRevision(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	m.pendingSessionKey = "sess:s1"

	cmd := m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:     "s1",
		key:    "sess:s1",
		source: transcriptAttachmentSourceSelectionLoad,
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("5"),
			Blocks: []transcriptdomain.Block{
				{Kind: "user_message", Role: "user", Text: "prompt"},
				{Kind: "assistant_message", Role: "assistant", Text: "snapshot"},
			},
		},
	})
	if cmd == nil {
		t.Fatalf("expected follow stream command after snapshot apply")
	}
	raw := cmd()
	msg := transcriptStreamMsg{}
	switch typed := raw.(type) {
	case transcriptStreamMsg:
		msg = typed
	case tea.BatchMsg:
		for _, candidate := range typed {
			streamMsg, ok := candidate().(transcriptStreamMsg)
			if ok {
				msg = streamMsg
				break
			}
		}
	default:
		t.Fatalf("expected transcriptStreamMsg or tea.BatchMsg, got %T", raw)
	}
	if msg.id == "" {
		t.Fatalf("expected follow stream command in snapshot result, got %#v", raw)
	}
	if msg.revision != "5" {
		t.Fatalf("expected follow stream after_revision=5, got %#v", msg)
	}
	if msg.generation == 0 {
		t.Fatalf("expected generation-aware stream attachment, got %#v", msg)
	}
}

func TestApplyTranscriptStreamMsgDropsStaleGenerationAndKeepsCurrent(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	coordinator := m.transcriptAttachmentCoordinatorOrDefault()
	stale := coordinator.Begin("s1", transcriptAttachmentSourceSelectionLoad, "1")
	current := coordinator.Begin("s1", transcriptAttachmentSourceSelectionLoad, "2")

	staleCanceled := false
	m.applyTranscriptStreamMsg(transcriptStreamMsg{
		id:         "s1",
		ch:         make(chan transcriptdomain.TranscriptEvent),
		generation: stale.Generation,
		cancel: func() {
			staleCanceled = true
		},
	})
	if !staleCanceled {
		t.Fatalf("expected stale generation response to cancel stream")
	}
	if m.transcriptStream != nil && m.transcriptStream.HasStream() {
		t.Fatalf("expected stale generation not to attach stream")
	}

	m.applyTranscriptStreamMsg(transcriptStreamMsg{
		id:         "s1",
		ch:         make(chan transcriptdomain.TranscriptEvent),
		generation: current.Generation,
	})
	if m.transcriptStream == nil || !m.transcriptStream.HasStream() {
		t.Fatalf("expected current generation to attach stream")
	}
	if got := m.transcriptStream.Generation(); got != current.Generation {
		t.Fatalf("expected active stream generation %d, got %d", current.Generation, got)
	}
}

func TestApplyTranscriptSnapshotMsgRecoveryAuthoritativeReplacesOlderRevision(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	m.pendingSessionKey = "sess:s1"
	if m.transcriptStream != nil {
		_, _ = m.transcriptStream.SetSnapshot(transcriptdomain.TranscriptSnapshot{
			Revision: transcriptdomain.MustParseRevisionToken("10"),
			Blocks:   []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "corrupted"}},
		})
	}
	m.transcriptRecoveryCoordinatorOrDefault().FlagRewind("s1", 4, transcriptReasonRecoveryRevisionRewind)

	cmd := m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:            "s1",
		key:           "sess:s1",
		source:        transcriptAttachmentSourceRecovery,
		authoritative: true,
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("5"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "recovered"}},
		},
	})
	if cmd == nil {
		t.Fatalf("expected recovery snapshot to open a fresh follow stream")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "recovered" {
		t.Fatalf("expected authoritative recovery snapshot to replace stale transcript, got %q", got)
	}
	if m.transcriptRecoveryCoordinatorOrDefault().ShouldApplyAuthoritativeSnapshot("s1") {
		t.Fatalf("expected recovery coordinator to mark session recovered")
	}
}
