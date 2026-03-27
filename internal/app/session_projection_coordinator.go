package app

import (
	"context"
	"strings"
)

type sessionProjectionCoordinator interface {
	Schedule(token string, parent context.Context) (context.Context, int)
	IsCurrent(token string, seq int) bool
	HasPending(token string) bool
	Consume(token string, seq int)
	LatestByToken() map[string]int
}

type sessionProjectionTracker interface {
	Next(token string, maxTracked int) int
	IsCurrent(token string, seq int) bool
	HasPending(token string) bool
	Consume(token string, seq int)
	LatestByToken() map[string]int
}

type defaultSessionProjectionCoordinator struct {
	policy  SessionProjectionPolicy
	tracker sessionProjectionTracker
	cancels map[string]context.CancelFunc
}

func NewDefaultSessionProjectionCoordinator(
	policy SessionProjectionPolicy,
	tracker sessionProjectionTracker,
) sessionProjectionCoordinator {
	if policy == nil {
		policy = defaultSessionProjectionPolicy{}
	}
	if tracker == nil {
		tracker = newDefaultSessionProjectionTracker()
	}
	return &defaultSessionProjectionCoordinator{
		policy:  policy,
		tracker: tracker,
		cancels: map[string]context.CancelFunc{},
	}
}

func (c *defaultSessionProjectionCoordinator) Schedule(token string, parent context.Context) (context.Context, int) {
	parent = commandParentContext(parent)
	if c == nil {
		return parent, 0
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return parent, 0
	}
	seq := c.tracker.Next(token, c.policy.MaxTrackedProjectionTokens())
	c.release(token)
	ctx, cancel := context.WithCancel(parent)
	if c.cancels == nil {
		c.cancels = map[string]context.CancelFunc{}
	}
	c.cancels[token] = cancel
	c.pruneCancels()
	return ctx, seq
}

func (c *defaultSessionProjectionCoordinator) IsCurrent(token string, seq int) bool {
	if c == nil || c.tracker == nil {
		return seq <= 0
	}
	return c.tracker.IsCurrent(strings.TrimSpace(token), seq)
}

func (c *defaultSessionProjectionCoordinator) HasPending(token string) bool {
	if c == nil || c.tracker == nil {
		return false
	}
	return c.tracker.HasPending(strings.TrimSpace(token))
}

func (c *defaultSessionProjectionCoordinator) Consume(token string, seq int) {
	if c == nil || c.tracker == nil {
		return
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	c.tracker.Consume(token, seq)
	if !c.tracker.HasPending(token) {
		c.release(token)
	}
}

func (c *defaultSessionProjectionCoordinator) LatestByToken() map[string]int {
	if c == nil || c.tracker == nil {
		return nil
	}
	return c.tracker.LatestByToken()
}

func (c *defaultSessionProjectionCoordinator) release(token string) {
	if c == nil || c.cancels == nil {
		return
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	cancel, ok := c.cancels[token]
	if !ok {
		return
	}
	if cancel != nil {
		cancel()
	}
	delete(c.cancels, token)
}

func (c *defaultSessionProjectionCoordinator) pruneCancels() {
	if c == nil || c.cancels == nil || c.tracker == nil {
		return
	}
	latest := c.tracker.LatestByToken()
	for token := range c.cancels {
		if _, ok := latest[token]; ok {
			continue
		}
		c.release(token)
	}
}

type defaultSessionProjectionTracker struct {
	nextSeq int
	latest  map[string]int
}

func newDefaultSessionProjectionTracker() *defaultSessionProjectionTracker {
	return &defaultSessionProjectionTracker{
		latest: map[string]int{},
	}
}

func (t *defaultSessionProjectionTracker) Next(token string, maxTracked int) int {
	if t == nil {
		return 0
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return 0
	}
	if t.latest == nil {
		t.latest = map[string]int{}
	}
	t.nextSeq++
	t.latest[token] = t.nextSeq
	t.prune(maxTracked)
	return t.nextSeq
}

func (t *defaultSessionProjectionTracker) IsCurrent(token string, seq int) bool {
	if t == nil {
		return seq <= 0
	}
	if seq <= 0 {
		return true
	}
	token = strings.TrimSpace(token)
	if token == "" || t.latest == nil {
		return false
	}
	latest, ok := t.latest[token]
	if !ok {
		return false
	}
	return latest == seq
}

func (t *defaultSessionProjectionTracker) HasPending(token string) bool {
	if t == nil || t.latest == nil {
		return false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	_, ok := t.latest[token]
	return ok
}

func (t *defaultSessionProjectionTracker) Consume(token string, seq int) {
	if t == nil || seq <= 0 || t.latest == nil {
		return
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	latest, ok := t.latest[token]
	if !ok || latest != seq {
		return
	}
	delete(t.latest, token)
}

func (t *defaultSessionProjectionTracker) LatestByToken() map[string]int {
	if t == nil || t.latest == nil {
		return nil
	}
	snapshot := make(map[string]int, len(t.latest))
	for token, seq := range t.latest {
		snapshot[token] = seq
	}
	return snapshot
}

func (t *defaultSessionProjectionTracker) prune(maxTracked int) {
	if t == nil || len(t.latest) == 0 {
		return
	}
	if maxTracked <= 0 {
		maxTracked = 1
	}
	for len(t.latest) > maxTracked {
		oldestToken := ""
		oldestSeq := 0
		for token, seq := range t.latest {
			if oldestToken == "" || seq < oldestSeq {
				oldestToken = token
				oldestSeq = seq
			}
		}
		if oldestToken == "" {
			return
		}
		delete(t.latest, oldestToken)
	}
}

func (m *Model) sessionProjectionCoordinatorOrDefault() sessionProjectionCoordinator {
	if m == nil || m.sessionProjectionCoordinator == nil {
		if m == nil {
			return NewDefaultSessionProjectionCoordinator(defaultSessionProjectionPolicy{}, nil)
		}
		m.sessionProjectionCoordinator = NewDefaultSessionProjectionCoordinator(m.sessionProjectionPolicyOrDefault(), nil)
		return m.sessionProjectionCoordinator
	}
	return m.sessionProjectionCoordinator
}
