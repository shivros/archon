package guidedworkflows

import (
	"errors"
	"strings"
	"sync"
	"time"
)

type DispatchErrorDisposition uint8

const (
	DispatchErrorDispositionNone DispatchErrorDisposition = iota
	DispatchErrorDispositionDeferred
	DispatchErrorDispositionFatal
)

type DispatchErrorClassifier interface {
	Classify(err error) DispatchErrorDisposition
}

type defaultDispatchErrorClassifier struct{}

func (defaultDispatchErrorClassifier) Classify(err error) DispatchErrorDisposition {
	if err == nil {
		return DispatchErrorDispositionNone
	}
	if errors.Is(err, ErrStepDispatchDeferred) {
		return DispatchErrorDispositionDeferred
	}
	return DispatchErrorDispositionFatal
}

type DispatchRetryPolicy interface {
	NextDelay(attempt int) (delay time.Duration, ok bool)
}

type BoundedExponentialDispatchRetryPolicy struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

func (p BoundedExponentialDispatchRetryPolicy) NextDelay(attempt int) (time.Duration, bool) {
	if attempt <= 0 {
		attempt = 1
	}
	initial := p.InitialDelay
	if initial <= 0 {
		initial = 250 * time.Millisecond
	}
	maxDelay := p.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 4 * time.Second
	}
	delay := initial
	for i := 1; i < attempt; i++ {
		if delay >= maxDelay {
			return maxDelay, true
		}
		delay *= 2
	}
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay, true
}

func defaultDispatchRetryPolicy() DispatchRetryPolicy {
	return BoundedExponentialDispatchRetryPolicy{
		InitialDelay: 250 * time.Millisecond,
		MaxDelay:     4 * time.Second,
	}
}

type DispatchRetryScheduler interface {
	Enqueue(runID string)
	Close()
}

type DispatchRetryAttemptFunc func(runID string) (done bool)

func NewDispatchRetryScheduler(policy DispatchRetryPolicy, attempt DispatchRetryAttemptFunc) DispatchRetryScheduler {
	if attempt == nil {
		return nil
	}
	if policy == nil {
		policy = defaultDispatchRetryPolicy()
	}
	return &defaultDispatchRetryScheduler{
		policy:   policy,
		attempt:  attempt,
		inFlight: map[string]struct{}{},
		stopCh:   make(chan struct{}),
	}
}

type defaultDispatchRetryScheduler struct {
	mu       sync.Mutex
	policy   DispatchRetryPolicy
	attempt  DispatchRetryAttemptFunc
	inFlight map[string]struct{}
	stopCh   chan struct{}
	closed   bool
}

func (s *defaultDispatchRetryScheduler) Enqueue(runID string) {
	if s == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if _, exists := s.inFlight[runID]; exists {
		s.mu.Unlock()
		return
	}
	s.inFlight[runID] = struct{}{}
	stopCh := s.stopCh
	s.mu.Unlock()
	go s.loop(runID, stopCh)
}

func (s *defaultDispatchRetryScheduler) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.stopCh)
	s.mu.Unlock()
}

func (s *defaultDispatchRetryScheduler) loop(runID string, stopCh <-chan struct{}) {
	for attempt := 1; ; attempt++ {
		delay, ok := s.policy.NextDelay(attempt)
		if !ok {
			s.release(runID)
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-stopCh:
			timer.Stop()
			s.release(runID)
			return
		}
		if s.attempt == nil || s.attempt(runID) {
			s.release(runID)
			return
		}
	}
}

func (s *defaultDispatchRetryScheduler) release(runID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.inFlight, runID)
	s.mu.Unlock()
}
