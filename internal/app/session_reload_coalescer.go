package app

import (
	"strings"
	"time"
)

const defaultSessionReloadNoopCoalesceWindow = 250 * time.Millisecond

type SessionReloadCoalescer interface {
	NoopReason(decision sessionReloadDecision, next sessionSelectionSnapshot, now time.Time) string
	Reset()
}

type defaultSessionReloadCoalescer struct {
	window  time.Duration
	lastKey string
	lastAt  time.Time
}

func NewDefaultSessionReloadCoalescer(window time.Duration) SessionReloadCoalescer {
	if window <= 0 {
		window = defaultSessionReloadNoopCoalesceWindow
	}
	return &defaultSessionReloadCoalescer{window: window}
}

func WithSessionReloadCoalescer(coalescer SessionReloadCoalescer) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if coalescer == nil {
			m.sessionReloadCoalescer = NewDefaultSessionReloadCoalescer(defaultSessionReloadNoopCoalesceWindow)
			return
		}
		m.sessionReloadCoalescer = coalescer
	}
}

func (c *defaultSessionReloadCoalescer) NoopReason(decision sessionReloadDecision, next sessionSelectionSnapshot, now time.Time) string {
	if c == nil {
		return decision.Reason
	}
	if decision.Reload {
		c.Reset()
		return decision.Reason
	}
	if strings.TrimSpace(decision.Reason) != transcriptReasonReloadVolatileMetadataIgnored {
		return decision.Reason
	}
	key := coalesceKeyForSelection(next)
	if strings.TrimSpace(key) == "" {
		return decision.Reason
	}
	coalesced := c.lastKey != "" &&
		c.lastKey == key &&
		!c.lastAt.IsZero() &&
		now.Sub(c.lastAt) <= c.window
	c.lastKey = key
	c.lastAt = now
	if coalesced {
		return transcriptReasonReloadCoalescedMetadataUpdate
	}
	return decision.Reason
}

func (c *defaultSessionReloadCoalescer) Reset() {
	if c == nil {
		return
	}
	c.lastKey = ""
	c.lastAt = time.Time{}
}

func (m *Model) sessionReloadCoalescerOrDefault() SessionReloadCoalescer {
	if m == nil || m.sessionReloadCoalescer == nil {
		return NewDefaultSessionReloadCoalescer(defaultSessionReloadNoopCoalesceWindow)
	}
	return m.sessionReloadCoalescer
}
