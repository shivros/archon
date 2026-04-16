package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"control/internal/apicode"
	"control/internal/client"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
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

func TestApplyTranscriptStreamMsgCanceledErrorIsSilent(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("codex")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	m.recordReconnectAttempt("s1", "codex", "transcript", transcriptSourceApplyEventsStream)

	m.applyTranscriptStreamMsg(transcriptStreamMsg{
		id:  "s1",
		err: context.Canceled,
	})

	if strings.Contains(strings.ToLower(m.status), "transcript stream error") {
		t.Fatalf("expected canceled transcript stream open to stay silent, got status %q", m.status)
	}
	metrics := sink.Snapshot()
	if len(metrics) == 0 {
		t.Fatalf("expected reconnect metric for canceled stream open")
	}
	last := metrics[len(metrics)-1]
	if last.Name != transcriptMetricReconnect || last.Outcome != transcriptOutcomeSkipped || last.Reason != transcriptReasonReconnectStreamCanceled {
		t.Fatalf("unexpected canceled reconnect metric: %#v", last)
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

func TestApplyTranscriptSnapshotMsgPendingErrorRetriesWithoutUserVisibleError(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"

	cmd := m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:     "s1",
		key:    "sess:s1",
		source: transcriptAttachmentSourceSelectionLoad,
		err: &client.APIError{
			StatusCode: 500,
			Message:    "transcript history pending",
			Code:       apicode.ErrorCodeTranscriptHistoryPending,
		},
		requestedLines: 200,
	})
	if cmd == nil {
		t.Fatalf("expected pending snapshot error to trigger follow-up commands")
	}
	if !m.loading || m.loadingKey != "sess:s1" {
		t.Fatalf("expected pending snapshot error to keep loading state visible during retries")
	}
	if strings.Contains(strings.ToLower(m.status), "error") {
		t.Fatalf("expected pending snapshot error not to set an error status, got %q", m.status)
	}
	if got := m.pendingTranscriptSnapshotRetryCount["sess:s1"]; got != 1 {
		t.Fatalf("expected one pending snapshot retry to be queued, got %d", got)
	}

	raw := cmd()
	batch, ok := raw.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected batched follow-up commands, got %T", raw)
	}
	if len(batch) != 2 {
		t.Fatalf("expected follow stream open and delayed retry commands, got %d", len(batch))
	}
}

func TestHandleTranscriptSnapshotPendingReturnsNilWhenNoRetryOrFollowAvailable(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"
	m.sessionTranscriptAPI = nil

	cmd := m.handleTranscriptSnapshotPending(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
	}, transcriptAttachmentSourceSelectionLoad, "sess:s1")
	if cmd != nil {
		t.Fatalf("expected no pending follow-up command without transcript API")
	}
	if m.loading || m.loadingKey != "" {
		t.Fatalf("expected pending handler to clear loading state for matching key")
	}
	if !strings.Contains(strings.ToLower(m.status), "pending") {
		t.Fatalf("expected pending status message, got %q", m.status)
	}
}

func TestHandleTranscriptSnapshotPendingTerminalStateFinishesLatencyWithError(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := newPhase0ModelWithSession("codex")
	WithUILatencySink(sink)(&m)
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"
	m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")
	m.sessionTranscriptAPI = nil

	cmd := m.handleTranscriptSnapshotPending(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
	}, transcriptAttachmentSourceSelectionLoad, "sess:s1")
	if cmd != nil {
		t.Fatalf("expected no pending follow-up command without transcript API")
	}
	if m.loading || m.loadingKey != "" {
		t.Fatalf("expected terminal pending snapshot state to clear loading")
	}
	if !hasLatencyMetric(sink.Snapshot(), uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeError) {
		t.Fatalf("expected terminal pending snapshot state to finish switch-session latency with error")
	}
}

func TestHandleTranscriptSnapshotPendingTerminalStateFallsBackToSessionIDLoadingSignal(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := newPhase0ModelWithSession("codex")
	WithUILatencySink(sink)(&m)
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"
	m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")
	m.sessionTranscriptAPI = nil

	cmd := m.handleTranscriptSnapshotPending(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:other",
	}, transcriptAttachmentSourceSelectionLoad, "sess:other")
	if cmd != nil {
		t.Fatalf("expected no pending follow-up command without transcript API")
	}
	if m.loading || m.loadingKey != "" {
		t.Fatalf("expected fallback loading-signal path to clear loading")
	}
	if !hasLatencyMetric(sink.Snapshot(), uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeError) {
		t.Fatalf("expected fallback loading-signal path to finish switch-session latency with error")
	}
}

func TestMaybeRetryPendingTranscriptSnapshotReturnsNilForMissingInputsOrLimit(t *testing.T) {
	msg := transcriptSnapshotMsg{id: "s1", key: "sess:s1", requestedLines: 200}

	mNoAPI := newPhase0ModelWithSession("codex")
	mNoAPI.sessionTranscriptAPI = nil
	if cmd := mNoAPI.maybeRetryPendingTranscriptSnapshot(msg, transcriptAttachmentSourceSelectionLoad, "sess:s1"); cmd != nil {
		t.Fatalf("expected nil retry command without transcript API")
	}

	mBlankKey := newPhase0ModelWithSession("codex")
	if cmd := mBlankKey.maybeRetryPendingTranscriptSnapshot(msg, transcriptAttachmentSourceSelectionLoad, "   "); cmd != nil {
		t.Fatalf("expected nil retry command for blank response key")
	}

	mLimited := newPhase0ModelWithSession("codex")
	mLimited.pendingTranscriptSnapshotRetryCount = map[string]int{"sess:s1": transcriptHistoryPendingRetryLimit}
	if cmd := mLimited.maybeRetryPendingTranscriptSnapshot(msg, transcriptAttachmentSourceSelectionLoad, "sess:s1"); cmd != nil {
		t.Fatalf("expected nil retry command after reaching retry limit")
	}
	if got := mLimited.pendingTranscriptSnapshotRetryCount["sess:s1"]; got != transcriptHistoryPendingRetryLimit {
		t.Fatalf("expected retry count to remain at limit, got %d", got)
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

func TestApplyLiveSessionItemsSnapshotAppliesForStartedSessionBeforeFirstEvent(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")

	handled, _ := m.reduceStateMessages(startSessionMsg{
		session: &types.Session{
			ID:       "s2",
			Provider: "codex",
			Status:   types.SessionStatusRunning,
			Title:    "Started session",
		},
		prompt: "hello from new session",
	})
	if !handled {
		t.Fatalf("expected start session message to be handled")
	}
	if got := m.requestActivity.eventCount; got != 0 {
		t.Fatalf("expected started session request activity to begin before events, got %d", got)
	}

	_, _ = m.transcriptStream.SetSnapshot(transcriptdomain.TranscriptSnapshot{
		Revision: transcriptdomain.MustParseRevisionToken("1"),
		Blocks: []transcriptdomain.Block{
			{Kind: "assistant_message", Role: "assistant", Text: "live reply"},
		},
	})

	applied := m.applyLiveSessionItemsSnapshot(sessionItemsMessageContext{
		source: sessionProjectionSourceTail,
		id:     "s2",
		key:    "sess:s2",
	})
	if !applied {
		t.Fatalf("expected started session live snapshot to apply before first event count increment")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "live reply" {
		t.Fatalf("expected live reply to be visible, got %q", got)
	}
	if got := countBlocksByRoleAndText(m.currentBlocks(), ChatRoleUser, "hello from new session"); got != 1 {
		t.Fatalf("expected optimistic prompt to remain visible during live hydration, got %#v", m.currentBlocks())
	}
}

func TestStartedSessionSnapshotRetainsOptimisticPromptUntilCanonicalUserArrives(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")

	handled, _ := m.reduceStateMessages(startSessionMsg{
		session: &types.Session{
			ID:       "s2",
			Provider: "codex",
			Status:   types.SessionStatusRunning,
			Title:    "Started session",
		},
		prompt: "hello from new session",
	})
	if !handled {
		t.Fatalf("expected start session message to be handled")
	}

	cmd := m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:     "s2",
		key:    "sess:s2",
		source: transcriptAttachmentSourceSessionStart,
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s2",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
			Blocks: []transcriptdomain.Block{
				{Kind: "assistant_message", Role: "assistant", Text: "assistant reply"},
			},
		},
	})
	if cmd == nil {
		t.Fatalf("expected started session snapshot to continue bootstrap")
	}
	if got := countBlocksByRoleAndText(m.currentBlocks(), ChatRoleUser, "hello from new session"); got != 1 {
		t.Fatalf("expected optimistic prompt to stay visible until canonical user arrives, got %#v", m.currentBlocks())
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "assistant reply" {
		t.Fatalf("expected assistant reply to be visible, got %q", got)
	}

	_ = m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:     "s2",
		key:    "sess:s2",
		source: transcriptAttachmentSourceSessionStart,
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s2",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("2"),
			Blocks: []transcriptdomain.Block{
				{Kind: "user_message", Role: "user", Text: "hello from new session"},
				{Kind: "assistant_message", Role: "assistant", Text: "assistant reply"},
			},
		},
	})
	if got := countBlocksByRoleAndText(m.currentBlocks(), ChatRoleUser, "hello from new session"); got != 1 {
		t.Fatalf("expected canonical user block to replace optimistic prompt without duplicates, got %#v", m.currentBlocks())
	}
	if len(m.pendingSends) != 0 || len(m.optimisticSends) != 0 {
		t.Fatalf("expected optimistic start-session send state to resolve once canonical user arrives")
	}
}

func TestStartedSessionPendingSnapshotKeepsPromptVisible(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}

	handled, _ := m.reduceStateMessages(startSessionMsg{
		session: &types.Session{
			ID:       "s2",
			Provider: "codex",
			Status:   types.SessionStatusRunning,
			Title:    "Started session",
		},
		prompt: "hello from new session",
	})
	if !handled {
		t.Fatalf("expected start session message to be handled")
	}

	cmd := m.applyTranscriptSnapshotMsg(transcriptSnapshotMsg{
		id:     "s2",
		key:    "sess:s2",
		source: transcriptAttachmentSourceSessionStart,
		err: &client.APIError{
			StatusCode: 500,
			Message:    "transcript history pending",
			Code:       apicode.ErrorCodeTranscriptHistoryPending,
		},
		requestedLines: 200,
	})
	if cmd == nil {
		t.Fatalf("expected pending snapshot error to trigger follow-up commands")
	}
	if got := countBlocksByRoleAndText(m.currentBlocks(), ChatRoleUser, "hello from new session"); got != 1 {
		t.Fatalf("expected prompt to remain visible during pending snapshot retries, got %#v", m.currentBlocks())
	}
}
