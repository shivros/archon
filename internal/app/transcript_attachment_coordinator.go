package app

import (
	"strings"
	"sync"
	"time"
)

type TranscriptAttachmentSource string

const (
	transcriptAttachmentSourceUnknown       TranscriptAttachmentSource = "unknown"
	transcriptAttachmentSourceSelectionLoad TranscriptAttachmentSource = "selection_load"
	transcriptAttachmentSourceSessionStart  TranscriptAttachmentSource = "session_start"
	transcriptAttachmentSourceSendReconnect TranscriptAttachmentSource = "send_reconnect"
	transcriptAttachmentSourceSubmitInput   TranscriptAttachmentSource = "submit_input"
	transcriptAttachmentSourceRecovery      TranscriptAttachmentSource = "recovery"
	transcriptAttachmentSourceManualRefresh TranscriptAttachmentSource = "manual_refresh"
)

type TranscriptAttachmentGeneration struct {
	SessionID     string
	Generation    uint64
	Source        TranscriptAttachmentSource
	AfterRevision string
	CreatedAt     time.Time
}

type TranscriptAttachmentDecision struct {
	Accept bool
	Reason string
}

type TranscriptAttachmentCoordinator interface {
	Begin(sessionID string, source TranscriptAttachmentSource, afterRevision string) TranscriptAttachmentGeneration
	Evaluate(sessionID string, generation uint64) TranscriptAttachmentDecision
	Current(sessionID string) (TranscriptAttachmentGeneration, bool)
	MarkGenerationUnhealthy(sessionID string, generation uint64, reason string)
	ClearSession(sessionID string)
	Reset()
}

type transcriptAttachmentSessionState struct {
	nextGeneration uint64
	current        TranscriptAttachmentGeneration
	hasCurrent     bool
	unhealthy      map[uint64]string
}

type defaultTranscriptAttachmentCoordinator struct {
	mu       sync.Mutex
	sessions map[string]transcriptAttachmentSessionState
	nowFn    func() time.Time
}

func NewDefaultTranscriptAttachmentCoordinator() TranscriptAttachmentCoordinator {
	return &defaultTranscriptAttachmentCoordinator{
		sessions: map[string]transcriptAttachmentSessionState{},
		nowFn:    time.Now,
	}
}

func (c *defaultTranscriptAttachmentCoordinator) Begin(sessionID string, source TranscriptAttachmentSource, afterRevision string) TranscriptAttachmentGeneration {
	if c == nil {
		return TranscriptAttachmentGeneration{}
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return TranscriptAttachmentGeneration{}
	}
	source = normalizeTranscriptAttachmentSource(source)
	afterRevision = strings.TrimSpace(afterRevision)

	now := time.Now().UTC()
	if c.nowFn != nil {
		now = c.nowFn().UTC()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.sessions[sessionID]
	state.nextGeneration++
	state.current = TranscriptAttachmentGeneration{
		SessionID:     sessionID,
		Generation:    state.nextGeneration,
		Source:        source,
		AfterRevision: afterRevision,
		CreatedAt:     now,
	}
	state.hasCurrent = true
	if state.unhealthy == nil {
		state.unhealthy = map[uint64]string{}
	}
	c.sessions[sessionID] = state
	return state.current
}

func (c *defaultTranscriptAttachmentCoordinator) Evaluate(sessionID string, generation uint64) TranscriptAttachmentDecision {
	if c == nil {
		return TranscriptAttachmentDecision{Accept: false, Reason: transcriptReasonReconnectStaleGeneration}
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || generation == 0 {
		return TranscriptAttachmentDecision{Accept: false, Reason: transcriptReasonReconnectStaleGeneration}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.sessions[sessionID]
	if !ok || !state.hasCurrent {
		return TranscriptAttachmentDecision{Accept: false, Reason: transcriptReasonReconnectStaleGeneration}
	}
	if generation != state.current.Generation {
		return TranscriptAttachmentDecision{Accept: false, Reason: transcriptReasonReconnectStaleGeneration}
	}
	if reason := strings.TrimSpace(state.unhealthy[generation]); reason != "" {
		return TranscriptAttachmentDecision{Accept: false, Reason: transcriptReasonReconnectUnhealthyGeneration}
	}
	return TranscriptAttachmentDecision{Accept: true, Reason: transcriptReasonReconnectMatchedSession}
}

func (c *defaultTranscriptAttachmentCoordinator) Current(sessionID string) (TranscriptAttachmentGeneration, bool) {
	if c == nil {
		return TranscriptAttachmentGeneration{}, false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return TranscriptAttachmentGeneration{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.sessions[sessionID]
	if !ok || !state.hasCurrent {
		return TranscriptAttachmentGeneration{}, false
	}
	return state.current, true
}

func (c *defaultTranscriptAttachmentCoordinator) MarkGenerationUnhealthy(sessionID string, generation uint64, reason string) {
	if c == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || generation == 0 {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = transcriptReasonRecoveryRevisionRewind
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.sessions[sessionID]
	if state.unhealthy == nil {
		state.unhealthy = map[uint64]string{}
	}
	state.unhealthy[generation] = reason
	c.sessions[sessionID] = state
}

func (c *defaultTranscriptAttachmentCoordinator) ClearSession(sessionID string) {
	if c == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sessions, sessionID)
}

func (c *defaultTranscriptAttachmentCoordinator) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.sessions)
}

func normalizeTranscriptAttachmentSource(source TranscriptAttachmentSource) TranscriptAttachmentSource {
	source = TranscriptAttachmentSource(strings.TrimSpace(string(source)))
	if source == "" {
		return transcriptAttachmentSourceUnknown
	}
	return source
}

func (m *Model) transcriptAttachmentCoordinatorOrDefault() TranscriptAttachmentCoordinator {
	if m == nil || m.transcriptAttachmentCoordinator == nil {
		return NewDefaultTranscriptAttachmentCoordinator()
	}
	return m.transcriptAttachmentCoordinator
}

func WithTranscriptAttachmentCoordinator(coordinator TranscriptAttachmentCoordinator) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if coordinator == nil {
			m.transcriptAttachmentCoordinator = NewDefaultTranscriptAttachmentCoordinator()
			return
		}
		m.transcriptAttachmentCoordinator = coordinator
	}
}
