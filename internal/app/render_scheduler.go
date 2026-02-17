package app

import "time"

const defaultStreamRenderInterval = 180 * time.Millisecond

type RenderScheduler interface {
	Request(now time.Time) bool
	ShouldRender(now time.Time) bool
	MarkRendered(now time.Time)
}

type throttledRenderScheduler struct {
	minInterval  time.Duration
	lastRendered time.Time
	pending      bool
}

func NewDefaultRenderScheduler() RenderScheduler {
	return NewThrottledRenderScheduler(defaultStreamRenderInterval)
}

func NewThrottledRenderScheduler(minInterval time.Duration) RenderScheduler {
	if minInterval < 0 {
		minInterval = 0
	}
	return &throttledRenderScheduler{minInterval: minInterval}
}

func (s *throttledRenderScheduler) Request(now time.Time) bool {
	if s == nil {
		return true
	}
	if s.minInterval <= 0 || s.ready(now) {
		return true
	}
	s.pending = true
	return false
}

func (s *throttledRenderScheduler) ShouldRender(now time.Time) bool {
	if s == nil {
		return false
	}
	if !s.pending {
		return false
	}
	if s.minInterval <= 0 || s.ready(now) {
		return true
	}
	return false
}

func (s *throttledRenderScheduler) MarkRendered(now time.Time) {
	if s == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	s.pending = false
	s.lastRendered = now
}

func (s *throttledRenderScheduler) ready(now time.Time) bool {
	if s == nil {
		return true
	}
	if now.IsZero() {
		now = time.Now()
	}
	if s.lastRendered.IsZero() {
		return true
	}
	return now.Sub(s.lastRendered) >= s.minInterval
}

func WithRenderScheduler(scheduler RenderScheduler) ModelOption {
	return func(m *Model) {
		if m == nil || scheduler == nil {
			return
		}
		m.streamRenderScheduler = scheduler
	}
}
