package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

func TestClassifySessionReloadReason(t *testing.T) {
	prev := sessionSelectionSnapshot{isSession: true, sessionID: "s1", key: "sess:s1", revision: "r1"}
	next := sessionSelectionSnapshot{isSession: true, sessionID: "s1", key: "sess:s1", revision: "r2"}
	if got := classifySessionReloadReason(prev, next); got != transcriptReasonSelectedRevisionChanged {
		t.Fatalf("expected revision change reason, got %q", got)
	}
	if got := classifySessionReloadReason(prev, sessionSelectionSnapshot{}); got != transcriptReasonNotSessionSelection {
		t.Fatalf("expected non-session reason, got %q", got)
	}
}

func TestClassifySessionReloadSkipReason(t *testing.T) {
	prev := sessionSelectionSnapshot{isSession: true, sessionID: "s1", key: "sess:s1"}
	next := sessionSelectionSnapshot{isSession: true, sessionID: "s1", key: "sess:s1"}
	if got := classifySessionReloadSkipReason(prev, next, uiModeNotes, true); got != transcriptReasonReloadSkipNotesMode {
		t.Fatalf("expected notes mode skip reason, got %q", got)
	}
	if got := classifySessionReloadSkipReason(prev, next, uiModeCompose, false); got != transcriptReasonReloadSkipFollowPaused {
		t.Fatalf("expected follow pause skip reason, got %q", got)
	}
}

func TestClassifyProjectionDropReason(t *testing.T) {
	reason := classifyProjectionDropReason("sess:s1", "s1", 1, map[string]int{"key:sess:s1": 2})
	if reason != transcriptReasonProjectionSuperseded {
		t.Fatalf("expected superseded reason, got %q", reason)
	}
}

func TestReduceStateMessagesRecordsStaleProjectionDropMetric(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := NewModel(nil, WithTranscriptBoundaryMetricsSink(sink))
	m.sessionProjectionLatest["key:sess:s1"] = 2

	handled, cmd := m.reduceStateMessages(sessionBlocksProjectedMsg{
		id:            "s1",
		key:           "sess:s1",
		provider:      "codex",
		projectionSeq: 1,
	})
	if !handled {
		t.Fatalf("expected projected message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected stale projection to stop without follow-up command")
	}
	metrics := sink.Snapshot()
	if len(metrics) != 1 {
		t.Fatalf("expected one metric, got %d", len(metrics))
	}
	got := metrics[0]
	if got.Name != transcriptMetricStaleRevision || got.Reason != transcriptReasonProjectionSuperseded || got.Outcome != transcriptOutcomeDropped {
		t.Fatalf("unexpected metric: %#v", got)
	}
}

func TestSessionsWithMetaRecordsReloadDecisionMetric(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("codex")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	current := m.sessions[0]
	handled, cmd := m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: []*types.Session{
			{
				ID:        current.ID,
				Provider:  current.Provider,
				Status:    current.Status,
				CreatedAt: current.CreatedAt,
				Title:     current.Title,
			},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-2", LastActiveAt: ptrObsTime(time.Now().UTC())},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected reload command")
	}
	metrics := sink.Snapshot()
	if len(metrics) == 0 {
		t.Fatalf("expected reload metric")
	}
	found := false
	for _, metric := range metrics {
		if metric.Name == transcriptMetricSessionReload && metric.Outcome == transcriptOutcomeSuccess && metric.Reason == transcriptReasonSelectedRevisionChanged {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected successful session_reload metric, got %#v", metrics)
	}
}

func TestReconnectAttemptAndOutcomeMetrics(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("kilocode")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	m.enterCompose("s1")
	if cmd := m.submitComposeInput("hello"); cmd == nil {
		t.Fatalf("expected send command")
	}

	metrics := sink.Snapshot()
	if len(metrics) == 0 {
		t.Fatalf("expected reconnect attempt metric")
	}
	last := metrics[len(metrics)-1]
	if last.Name != transcriptMetricReconnect || last.Outcome != transcriptOutcomeAttempt || last.Stream != "items" {
		t.Fatalf("unexpected reconnect attempt metric: %#v", last)
	}

	m.applyItemsStreamMsg(itemsStreamMsg{id: "s1", ch: make(chan map[string]any)})
	metrics = sink.Snapshot()
	if len(metrics) < 2 {
		t.Fatalf("expected reconnect outcome metric")
	}
	last = metrics[len(metrics)-1]
	if last.Name != transcriptMetricReconnect || last.Outcome != transcriptOutcomeSuccess || last.Reason != transcriptReasonReconnectStreamAttached {
		t.Fatalf("unexpected reconnect outcome metric: %#v", last)
	}
}

func TestReconnectAttemptTrackerClearSession(t *testing.T) {
	tracker := newReconnectAttemptTracker(8, time.Minute)
	if got := tracker.markAttempt("items", "s1"); got != 1 {
		t.Fatalf("expected first attempt to be 1, got %d", got)
	}
	tracker.clearSession("s1")
	if _, ok := tracker.popAttempt("items", "s1"); ok {
		t.Fatalf("expected cleared session attempts to be removed")
	}
}

func TestReconnectAttemptTrackerPrunesExpiredEntries(t *testing.T) {
	tracker := newReconnectAttemptTracker(8, 5*time.Second)
	now := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	tracker.nowFn = func() time.Time { return now }
	if got := tracker.markAttempt("items", "s1"); got != 1 {
		t.Fatalf("expected first attempt to be 1, got %d", got)
	}
	now = now.Add(10 * time.Second)
	if _, ok := tracker.popAttempt("items", "s1"); ok {
		t.Fatalf("expected expired attempt entry to be pruned")
	}
}

func TestNormalizeTranscriptMetricRequiresName(t *testing.T) {
	normalized, ok := normalizeTranscriptMetric(TranscriptBoundaryMetric{}, time.Now().UTC())
	if ok || normalized.Name != "" {
		t.Fatalf("expected empty-name metric to be rejected")
	}
}

func TestReconnectAttemptTrackerEvictsOldestWhenCapacityReached(t *testing.T) {
	tracker := newReconnectAttemptTracker(1, time.Hour)
	if got := tracker.markAttempt("items", "s1"); got != 1 {
		t.Fatalf("expected first attempt for s1")
	}
	if got := tracker.markAttempt("items", "s2"); got != 1 {
		t.Fatalf("expected first attempt for s2")
	}
	if _, ok := tracker.popAttempt("items", "s1"); ok {
		t.Fatalf("expected oldest session attempt to be evicted")
	}
	if got, ok := tracker.popAttempt("items", "s2"); !ok || got != 1 {
		t.Fatalf("expected newest session attempt to remain, got=%d ok=%v", got, ok)
	}
}

func TestWithTranscriptBoundaryDebugOption(t *testing.T) {
	m := NewModel(nil)
	WithTranscriptBoundaryDebug(true)(&m)
	if m.transcriptBoundary == nil || m.transcriptBoundary.debug == nil {
		t.Fatalf("expected transcript boundary observer with debug emitter")
	}
	m.transcriptBoundary.debug.mu.Lock()
	enabled := m.transcriptBoundary.debug.enabled
	m.transcriptBoundary.debug.mu.Unlock()
	if !enabled {
		t.Fatalf("expected debug option to enable emitter")
	}
}

func TestNoopTranscriptDebugLoggerPrintf(t *testing.T) {
	noopTranscriptDebugLogger{}.Printf("ignored %s", "line")
}

type captureTranscriptDebugLogger struct {
	lines []string
}

func (l *captureTranscriptDebugLogger) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func TestTranscriptDebugEmitterUsesLoggerAbstraction(t *testing.T) {
	logger := &captureTranscriptDebugLogger{}
	emitter := newTranscriptDebugEmitter(logger)
	emitter.setEnabled(true)
	emitter.emit(newReconnectMetric(
		transcriptReasonReconnectStreamAttached,
		transcriptOutcomeSuccess,
		transcriptSourceApplyItemsStream,
		"s1",
		"kilocode",
		"items",
		1,
	))
	if len(logger.lines) != 1 {
		t.Fatalf("expected one debug line, got %d", len(logger.lines))
	}
	if !strings.Contains(logger.lines[0], "transcript_boundary") {
		t.Fatalf("expected transcript_boundary debug line, got %q", logger.lines[0])
	}
}

func ptrObsTime(v time.Time) *time.Time {
	return &v
}
