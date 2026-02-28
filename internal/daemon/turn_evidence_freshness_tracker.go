package daemon

import (
	"strings"
	"sync"
)

type TurnEvidenceFreshnessTracker interface {
	MarkFresh(sessionID, evidenceKey, output string) bool
}

type inMemoryTurnEvidenceFreshnessTracker struct {
	mu            sync.Mutex
	lastBySession map[string]string
}

func NewTurnEvidenceFreshnessTracker() TurnEvidenceFreshnessTracker {
	return &inMemoryTurnEvidenceFreshnessTracker{
		lastBySession: map[string]string{},
	}
}

func (t *inMemoryTurnEvidenceFreshnessTracker) MarkFresh(sessionID, evidenceKey, output string) bool {
	if t == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	evidenceKey = strings.TrimSpace(evidenceKey)
	if evidenceKey == "" {
		return strings.TrimSpace(output) != ""
	}
	if sessionID == "" {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lastBySession == nil {
		t.lastBySession = map[string]string{}
	}
	if t.lastBySession[sessionID] == evidenceKey {
		return false
	}
	t.lastBySession[sessionID] = evidenceKey
	return true
}
