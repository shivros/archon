package app

import (
	"log"
	"strings"
	"sync"
	"time"
)

const (
	transcriptMetricSessionReload   = "session_reload"
	transcriptMetricStreamClose     = "stream_close"
	transcriptMetricReconnect       = "reconnect"
	transcriptMetricStaleRevision   = "stale_revision_drop"
	transcriptMetricTranscriptReset = "transcript_reset"
	transcriptMetricApprovalRefresh = "approval_refresh"
)

const (
	transcriptOutcomeAttempt = "attempt"
	transcriptOutcomeSuccess = "success"
	transcriptOutcomeSkipped = "skipped"
	transcriptOutcomeNoop    = "noop"
	transcriptOutcomeDropped = "dropped"
	transcriptOutcomeError   = "error"
)

const (
	transcriptReasonNotSessionSelection                        = "not_session_selection"
	transcriptReasonSelectedSessionFromNon                     = "selected_session_from_non_session"
	transcriptReasonSelectedSessionChanged                     = "selected_session_changed"
	transcriptReasonSelectedKeyChanged                         = "selected_key_changed"
	transcriptReasonSelectedRevisionChanged                    = "selected_revision_changed"
	transcriptReasonSelectedRevisionUnchanged                  = "selected_revision_unchanged"
	transcriptReasonReloadSkipNotesMode                        = "notes_mode"
	transcriptReasonReloadSkipFollowPaused                     = "follow_paused_same_selection"
	transcriptReasonReloadSkipPolicy                           = "policy_skip"
	transcriptReasonReloadVolatileMetadataIgnored              = "volatile_metadata_ignored"
	transcriptReasonReloadCoalescedMetadataUpdate              = "coalesced_metadata_update"
	transcriptReasonReloadSemanticCapabilityChanged            = "semantic_capability_changed"
	transcriptReasonProjectionNonVersioned                     = "non_versioned_projection"
	transcriptReasonProjectionMissingToken                     = "missing_projection_token"
	transcriptReasonProjectionTrackerMissing                   = "projection_tracker_missing"
	transcriptReasonProjectionTokenUntracked                   = "projection_token_untracked"
	transcriptReasonProjectionSuperseded                       = "projection_superseded"
	transcriptReasonProjectionFutureSequence                   = "projection_future_sequence"
	transcriptReasonProjectionMismatch                         = "projection_mismatch"
	transcriptReasonReconnectStreamDisconnected                = "stream_disconnected"
	transcriptReasonReconnectStreamError                       = "stream_error"
	transcriptReasonReconnectStreamAttached                    = "stream_attached"
	transcriptReasonReconnectMismatchedSession                 = "mismatched_session"
	transcriptReasonReconnectMatchedSession                    = "matched_active_session"
	transcriptReasonReconnectStaleGeneration                   = "stale_generation"
	transcriptReasonReconnectUnhealthyGeneration               = "unhealthy_generation"
	transcriptReasonStreamCloseTailChannel                     = "tail_channel_closed"
	transcriptReasonStreamCloseEventsChannel                   = "events_channel_closed"
	transcriptReasonStreamCloseItemsChannel                    = "items_channel_closed"
	transcriptReasonSnapshotSuperseded                         = "snapshot_superseded"
	transcriptReasonRecoveryRevisionRewind                     = "revision_rewind"
	transcriptReasonApprovalRefreshCapabilitySupported         = "approval_capability_supported"
	transcriptReasonApprovalRefreshCapabilityUnsupported       = "approval_capability_unsupported"
	transcriptReasonApprovalRefreshProviderFallbackSupported   = "approval_provider_fallback_supported"
	transcriptReasonApprovalRefreshProviderFallbackUnsupported = "approval_provider_fallback_unsupported"
)

const (
	transcriptResetReasonUnspecified               = "unspecified"
	transcriptResetReasonSelectionCleared          = "selection_cleared"
	transcriptResetReasonSelectionLoad             = "selection_load"
	transcriptResetReasonSidebarUnavailable        = "sidebar_unavailable"
	transcriptResetReasonSidebarNoSelection        = "sidebar_no_selection"
	transcriptResetReasonSidebarNonSessionSelected = "sidebar_non_session_selection"
	transcriptResetReasonProviderSelectionChanged  = "provider_selection_changed"
	transcriptResetReasonNewSessionStartRequested  = "new_session_start_requested"
	transcriptResetReasonStartSessionResponse      = "start_session_response"
)

const (
	transcriptSourceModelResetStream     = "model.reset_stream"
	transcriptSourceConsumeStreamTick    = "model.consume_stream_tick"
	transcriptSourceConsumeCodexTick     = "model.consume_codex_tick"
	transcriptSourceConsumeItemTick      = "model.consume_item_tick"
	transcriptSourceSessionsWithMeta     = "model.sessions_with_meta"
	transcriptSourceSessionBlocksProject = "model.session_blocks_projected"
	transcriptSourceSendMsg              = "model.send_msg"
	transcriptSourceSubmitComposeInput   = "model.submit_compose_input"
	transcriptSourceAutoRefreshHistory   = "model.auto_refresh_history"
	transcriptSourceApplyEventsStream    = "model.apply_events_stream"
	transcriptSourceApplyItemsStream     = "model.apply_items_stream"
)

const (
	transcriptReconnectMaxEntries = 256
	transcriptReconnectTTL        = 10 * time.Minute
)

type TranscriptBoundaryMetric struct {
	Name      string
	Reason    string
	Outcome   string
	Source    string
	SessionID string
	Provider  string
	Stream    string
	Attempt   int
	At        time.Time
}

type TranscriptBoundaryMetricsSink interface {
	RecordTranscriptBoundaryMetric(metric TranscriptBoundaryMetric)
}

type TranscriptDebugLogger interface {
	Printf(format string, args ...any)
}

type inMemoryTranscriptBoundaryMetricsSink struct {
	mu      sync.Mutex
	metrics []TranscriptBoundaryMetric
}

func NewInMemoryTranscriptBoundaryMetricsSink() *inMemoryTranscriptBoundaryMetricsSink {
	return &inMemoryTranscriptBoundaryMetricsSink{metrics: []TranscriptBoundaryMetric{}}
}

func (s *inMemoryTranscriptBoundaryMetricsSink) RecordTranscriptBoundaryMetric(metric TranscriptBoundaryMetric) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = append(s.metrics, metric)
}

func (s *inMemoryTranscriptBoundaryMetricsSink) Snapshot() []TranscriptBoundaryMetric {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TranscriptBoundaryMetric, len(s.metrics))
	copy(out, s.metrics)
	return out
}

type transcriptMetricRecorder struct {
	mu    sync.Mutex
	sink  TranscriptBoundaryMetricsSink
	nowFn func() time.Time
}

func newTranscriptMetricRecorder(sink TranscriptBoundaryMetricsSink) *transcriptMetricRecorder {
	return &transcriptMetricRecorder{
		sink:  sink,
		nowFn: time.Now,
	}
}

func (r *transcriptMetricRecorder) setSink(sink TranscriptBoundaryMetricsSink) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sink = sink
}

func (r *transcriptMetricRecorder) now() time.Time {
	if r == nil || r.nowFn == nil {
		return time.Now().UTC()
	}
	return r.nowFn().UTC()
}

func (r *transcriptMetricRecorder) record(metric TranscriptBoundaryMetric) (TranscriptBoundaryMetric, bool) {
	if r == nil {
		return TranscriptBoundaryMetric{}, false
	}
	normalized, ok := normalizeTranscriptMetric(metric, r.now())
	if !ok {
		return TranscriptBoundaryMetric{}, false
	}
	r.mu.Lock()
	sink := r.sink
	r.mu.Unlock()
	if sink != nil {
		sink.RecordTranscriptBoundaryMetric(normalized)
	}
	return normalized, true
}

func normalizeTranscriptMetric(metric TranscriptBoundaryMetric, at time.Time) (TranscriptBoundaryMetric, bool) {
	metric.Name = strings.TrimSpace(metric.Name)
	metric.Reason = strings.TrimSpace(metric.Reason)
	metric.Outcome = strings.TrimSpace(metric.Outcome)
	metric.Source = strings.TrimSpace(metric.Source)
	metric.SessionID = strings.TrimSpace(metric.SessionID)
	metric.Provider = strings.TrimSpace(metric.Provider)
	metric.Stream = strings.TrimSpace(metric.Stream)
	if metric.Name == "" {
		return TranscriptBoundaryMetric{}, false
	}
	if metric.At.IsZero() {
		metric.At = at
	}
	return metric, true
}

type noopTranscriptDebugLogger struct{}

func (noopTranscriptDebugLogger) Printf(string, ...any) {}

type transcriptDebugEmitter struct {
	mu      sync.Mutex
	enabled bool
	logger  TranscriptDebugLogger
}

func newTranscriptDebugEmitter(logger TranscriptDebugLogger) *transcriptDebugEmitter {
	if logger == nil {
		logger = noopTranscriptDebugLogger{}
	}
	return &transcriptDebugEmitter{logger: logger}
}

func (e *transcriptDebugEmitter) setEnabled(enabled bool) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enabled = enabled
}

func (e *transcriptDebugEmitter) emit(metric TranscriptBoundaryMetric) {
	if e == nil {
		return
	}
	e.mu.Lock()
	enabled := e.enabled
	logger := e.logger
	e.mu.Unlock()
	if !enabled || logger == nil {
		return
	}
	logger.Printf(
		"transcript_boundary metric=%s reason=%s outcome=%s source=%s session_id=%s provider=%s stream=%s attempt=%d",
		metric.Name,
		metric.Reason,
		metric.Outcome,
		metric.Source,
		metric.SessionID,
		metric.Provider,
		metric.Stream,
		metric.Attempt,
	)
}

type reconnectAttemptEntry struct {
	Attempt      int
	LastTouched  time.Time
	TrackingKey  string
	SessionIDRef string
}

type reconnectAttemptTracker struct {
	mu         sync.Mutex
	entries    map[string]reconnectAttemptEntry
	maxEntries int
	ttl        time.Duration
	nowFn      func() time.Time
}

func newReconnectAttemptTracker(maxEntries int, ttl time.Duration) *reconnectAttemptTracker {
	if maxEntries <= 0 {
		maxEntries = transcriptReconnectMaxEntries
	}
	if ttl <= 0 {
		ttl = transcriptReconnectTTL
	}
	return &reconnectAttemptTracker{
		entries:    map[string]reconnectAttemptEntry{},
		maxEntries: maxEntries,
		ttl:        ttl,
		nowFn:      time.Now,
	}
}

func (t *reconnectAttemptTracker) now() time.Time {
	if t == nil || t.nowFn == nil {
		return time.Now().UTC()
	}
	return t.nowFn().UTC()
}

func (t *reconnectAttemptTracker) markAttempt(stream, sessionID string) int {
	if t == nil {
		return 0
	}
	key := reconnectTrackingKey(stream, sessionID)
	if key == "" {
		return 0
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)
	if len(t.entries) >= t.maxEntries {
		t.evictOldestLocked()
	}
	entry := t.entries[key]
	entry.Attempt++
	entry.LastTouched = now
	entry.TrackingKey = key
	entry.SessionIDRef = strings.TrimSpace(sessionID)
	t.entries[key] = entry
	return entry.Attempt
}

func (t *reconnectAttemptTracker) popAttempt(stream, sessionID string) (attempt int, ok bool) {
	if t == nil {
		return 0, false
	}
	key := reconnectTrackingKey(stream, sessionID)
	if key == "" {
		return 0, false
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)
	entry, ok := t.entries[key]
	if !ok {
		return 0, false
	}
	delete(t.entries, key)
	return entry.Attempt, true
}

func (t *reconnectAttemptTracker) clearSession(sessionID string) {
	if t == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, entry := range t.entries {
		if entry.SessionIDRef == sessionID || strings.HasSuffix(key, ":"+sessionID) {
			delete(t.entries, key)
		}
	}
}

func (t *reconnectAttemptTracker) pruneLocked(now time.Time) {
	if t == nil || len(t.entries) == 0 || t.ttl <= 0 {
		return
	}
	for key, entry := range t.entries {
		if entry.LastTouched.IsZero() || now.Sub(entry.LastTouched) > t.ttl {
			delete(t.entries, key)
		}
	}
}

func (t *reconnectAttemptTracker) evictOldestLocked() {
	if t == nil || len(t.entries) == 0 {
		return
	}
	oldestKey := ""
	oldestAt := time.Time{}
	for key, entry := range t.entries {
		if oldestKey == "" || entry.LastTouched.Before(oldestAt) {
			oldestKey = key
			oldestAt = entry.LastTouched
		}
	}
	if oldestKey != "" {
		delete(t.entries, oldestKey)
	}
}

type transcriptBoundaryObserver struct {
	recorder  *transcriptMetricRecorder
	debug     *transcriptDebugEmitter
	reconnect *reconnectAttemptTracker
}

func newTranscriptBoundaryObserver(sink TranscriptBoundaryMetricsSink) *transcriptBoundaryObserver {
	return &transcriptBoundaryObserver{
		recorder:  newTranscriptMetricRecorder(sink),
		debug:     newTranscriptDebugEmitter(log.Default()),
		reconnect: newReconnectAttemptTracker(transcriptReconnectMaxEntries, transcriptReconnectTTL),
	}
}

func (o *transcriptBoundaryObserver) setSink(sink TranscriptBoundaryMetricsSink) {
	if o == nil || o.recorder == nil {
		return
	}
	o.recorder.setSink(sink)
}

func (o *transcriptBoundaryObserver) setDebug(enabled bool) {
	if o == nil || o.debug == nil {
		return
	}
	o.debug.setEnabled(enabled)
}

func (o *transcriptBoundaryObserver) record(metric TranscriptBoundaryMetric) {
	if o == nil || o.recorder == nil {
		return
	}
	normalized, ok := o.recorder.record(metric)
	if !ok {
		return
	}
	if o.debug != nil {
		o.debug.emit(normalized)
	}
}

func (o *transcriptBoundaryObserver) markReconnectAttempt(stream, sessionID string) int {
	if o == nil || o.reconnect == nil {
		return 0
	}
	return o.reconnect.markAttempt(stream, sessionID)
}

func (o *transcriptBoundaryObserver) popReconnectAttempt(stream, sessionID string) (int, bool) {
	if o == nil || o.reconnect == nil {
		return 0, false
	}
	return o.reconnect.popAttempt(stream, sessionID)
}

func (o *transcriptBoundaryObserver) clearReconnectSession(sessionID string) {
	if o == nil || o.reconnect == nil {
		return
	}
	o.reconnect.clearSession(sessionID)
}

func reconnectTrackingKey(stream, sessionID string) string {
	stream = strings.TrimSpace(stream)
	sessionID = strings.TrimSpace(sessionID)
	if stream == "" || sessionID == "" {
		return ""
	}
	return stream + ":" + sessionID
}

func classifySessionReloadReason(previous, next sessionSelectionSnapshot) string {
	if !next.isSession {
		return transcriptReasonNotSessionSelection
	}
	if !previous.isSession {
		return transcriptReasonSelectedSessionFromNon
	}
	if previous.sessionID != next.sessionID {
		return transcriptReasonSelectedSessionChanged
	}
	if previous.key != next.key {
		return transcriptReasonSelectedKeyChanged
	}
	if previous.revision != next.revision {
		return transcriptReasonSelectedRevisionChanged
	}
	return transcriptReasonSelectedRevisionUnchanged
}

func classifySessionReloadSkipReason(previous, next sessionSelectionSnapshot, mode uiMode, follow bool) string {
	if mode == uiModeNotes || mode == uiModeAddNote {
		return transcriptReasonReloadSkipNotesMode
	}
	if !follow && previous.isSession && next.isSession && previous.sessionID == next.sessionID && previous.key == next.key {
		return transcriptReasonReloadSkipFollowPaused
	}
	return transcriptReasonReloadSkipPolicy
}

func classifyProjectionDropReason(key, id string, seq int, latest map[string]int) string {
	if seq <= 0 {
		return transcriptReasonProjectionNonVersioned
	}
	token := sessionProjectionToken(key, id)
	if token == "" {
		return transcriptReasonProjectionMissingToken
	}
	if latest == nil {
		return transcriptReasonProjectionTrackerMissing
	}
	current, ok := latest[token]
	if !ok {
		return transcriptReasonProjectionTokenUntracked
	}
	if seq < current {
		return transcriptReasonProjectionSuperseded
	}
	if seq > current {
		return transcriptReasonProjectionFutureSequence
	}
	return transcriptReasonProjectionMismatch
}

func newSessionReloadMetric(reason, outcome, source, sessionID, provider string) TranscriptBoundaryMetric {
	return TranscriptBoundaryMetric{
		Name:      transcriptMetricSessionReload,
		Reason:    reason,
		Outcome:   outcome,
		Source:    source,
		SessionID: sessionID,
		Provider:  provider,
	}
}

func newStaleRevisionDropMetric(reason, source, sessionID, provider string) TranscriptBoundaryMetric {
	return TranscriptBoundaryMetric{
		Name:      transcriptMetricStaleRevision,
		Reason:    reason,
		Outcome:   transcriptOutcomeDropped,
		Source:    source,
		SessionID: sessionID,
		Provider:  provider,
	}
}

func newStreamCloseMetric(reason, source, sessionID, provider, stream string) TranscriptBoundaryMetric {
	return TranscriptBoundaryMetric{
		Name:      transcriptMetricStreamClose,
		Reason:    reason,
		Outcome:   transcriptOutcomeSuccess,
		Source:    source,
		SessionID: sessionID,
		Provider:  provider,
		Stream:    stream,
	}
}

func newTranscriptResetMetric(reason, source, sessionID, provider string) TranscriptBoundaryMetric {
	return TranscriptBoundaryMetric{
		Name:      transcriptMetricTranscriptReset,
		Reason:    reason,
		Outcome:   transcriptOutcomeSuccess,
		Source:    source,
		SessionID: sessionID,
		Provider:  provider,
	}
}

func newReconnectMetric(reason, outcome, source, sessionID, provider, stream string, attempt int) TranscriptBoundaryMetric {
	return TranscriptBoundaryMetric{
		Name:      transcriptMetricReconnect,
		Reason:    reason,
		Outcome:   outcome,
		Source:    source,
		SessionID: sessionID,
		Provider:  provider,
		Stream:    stream,
		Attempt:   attempt,
	}
}

func newApprovalRefreshMetric(reason, outcome, source, sessionID, provider string) TranscriptBoundaryMetric {
	return TranscriptBoundaryMetric{
		Name:      transcriptMetricApprovalRefresh,
		Reason:    reason,
		Outcome:   outcome,
		Source:    source,
		SessionID: sessionID,
		Provider:  provider,
		Stream:    "transcript",
	}
}

func WithTranscriptBoundaryMetricsSink(sink TranscriptBoundaryMetricsSink) ModelOption {
	return func(m *Model) {
		if m == nil || m.transcriptBoundary == nil {
			return
		}
		m.transcriptBoundary.setSink(sink)
	}
}

func WithTranscriptBoundaryDebug(enabled bool) ModelOption {
	return func(m *Model) {
		if m == nil || m.transcriptBoundary == nil {
			return
		}
		m.transcriptBoundary.setDebug(enabled)
	}
}

func (m *Model) recordTranscriptBoundaryMetric(metric TranscriptBoundaryMetric) {
	if m == nil || m.transcriptBoundary == nil {
		return
	}
	m.transcriptBoundary.record(metric)
}

func (m *Model) clearReconnectAttemptsForSession(sessionID string) {
	if m == nil || m.transcriptBoundary == nil {
		return
	}
	m.transcriptBoundary.clearReconnectSession(sessionID)
}

func (m *Model) recordReconnectAttempt(sessionID, provider, stream, source string) {
	if m == nil || m.transcriptBoundary == nil {
		return
	}
	attempt := m.transcriptBoundary.markReconnectAttempt(stream, sessionID)
	if attempt <= 0 {
		return
	}
	m.recordTranscriptBoundaryMetric(newReconnectMetric(
		transcriptReasonReconnectStreamDisconnected,
		transcriptOutcomeAttempt,
		source,
		sessionID,
		provider,
		stream,
		attempt,
	))
}

func (m *Model) recordReconnectOutcome(sessionID, provider, stream, source, outcome, reason string) {
	if m == nil || m.transcriptBoundary == nil {
		return
	}
	attempt, ok := m.transcriptBoundary.popReconnectAttempt(stream, sessionID)
	if !ok {
		return
	}
	m.recordTranscriptBoundaryMetric(newReconnectMetric(reason, outcome, source, sessionID, provider, stream, attempt))
}
