package app

import (
	"strings"
	"sync"
	"time"
)

type TranscriptRecoveryState struct {
	SessionID   string
	Generation  uint64
	Reason      string
	Rewound     bool
	DetectedAt  time.Time
	RecoveredAt time.Time
}

type TranscriptRecoveryCoordinator interface {
	FlagRewind(sessionID string, generation uint64, reason string)
	ShouldApplyAuthoritativeSnapshot(sessionID string) bool
	MarkRecovered(sessionID string)
	Clear(sessionID string)
	State(sessionID string) (TranscriptRecoveryState, bool)
	Reset()
}

type defaultTranscriptRecoveryCoordinator struct {
	mu     sync.Mutex
	states map[string]TranscriptRecoveryState
	nowFn  func() time.Time
}

func NewDefaultTranscriptRecoveryCoordinator() TranscriptRecoveryCoordinator {
	return &defaultTranscriptRecoveryCoordinator{
		states: map[string]TranscriptRecoveryState{},
		nowFn:  time.Now,
	}
}

func (c *defaultTranscriptRecoveryCoordinator) FlagRewind(sessionID string, generation uint64, reason string) {
	if c == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = transcriptReasonRecoveryRevisionRewind
	}
	now := time.Now().UTC()
	if c.nowFn != nil {
		now = c.nowFn().UTC()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.states[sessionID]
	state.SessionID = sessionID
	state.Generation = generation
	state.Reason = reason
	state.Rewound = true
	state.DetectedAt = now
	state.RecoveredAt = time.Time{}
	c.states[sessionID] = state
}

func (c *defaultTranscriptRecoveryCoordinator) ShouldApplyAuthoritativeSnapshot(sessionID string) bool {
	if c == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.states[sessionID]
	if !ok {
		return false
	}
	return state.Rewound
}

func (c *defaultTranscriptRecoveryCoordinator) MarkRecovered(sessionID string) {
	if c == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	now := time.Now().UTC()
	if c.nowFn != nil {
		now = c.nowFn().UTC()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.states[sessionID]
	if !ok {
		return
	}
	state.Rewound = false
	state.RecoveredAt = now
	c.states[sessionID] = state
}

func (c *defaultTranscriptRecoveryCoordinator) Clear(sessionID string) {
	if c == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.states, sessionID)
}

func (c *defaultTranscriptRecoveryCoordinator) State(sessionID string) (TranscriptRecoveryState, bool) {
	if c == nil {
		return TranscriptRecoveryState{}, false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return TranscriptRecoveryState{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.states[sessionID]
	if !ok {
		return TranscriptRecoveryState{}, false
	}
	return state, true
}

func (c *defaultTranscriptRecoveryCoordinator) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.states)
}

func (m *Model) transcriptRecoveryCoordinatorOrDefault() TranscriptRecoveryCoordinator {
	if m == nil || m.transcriptRecoveryCoordinator == nil {
		return NewDefaultTranscriptRecoveryCoordinator()
	}
	return m.transcriptRecoveryCoordinator
}

func WithTranscriptRecoveryCoordinator(coordinator TranscriptRecoveryCoordinator) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if coordinator == nil {
			m.transcriptRecoveryCoordinator = NewDefaultTranscriptRecoveryCoordinator()
			return
		}
		m.transcriptRecoveryCoordinator = coordinator
	}
}
