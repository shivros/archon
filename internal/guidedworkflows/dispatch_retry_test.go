package guidedworkflows

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fixedDispatchRetryPolicy struct {
	delay time.Duration
}

func (p fixedDispatchRetryPolicy) NextDelay(int) (time.Duration, bool) {
	return p.delay, true
}

type stoppingDispatchRetryPolicy struct {
	calls atomic.Int32
}

func (p *stoppingDispatchRetryPolicy) NextDelay(int) (time.Duration, bool) {
	if p == nil {
		return 0, false
	}
	p.calls.Add(1)
	return 0, false
}

func TestDefaultDispatchErrorClassifier(t *testing.T) {
	classifier := defaultDispatchErrorClassifier{}
	if got := classifier.Classify(nil); got != DispatchErrorDispositionNone {
		t.Fatalf("expected nil error to classify as none, got %v", got)
	}
	if got := classifier.Classify(ErrStepDispatchDeferred); got != DispatchErrorDispositionDeferred {
		t.Fatalf("expected deferred error classification, got %v", got)
	}
	if got := classifier.Classify(errors.New("boom")); got != DispatchErrorDispositionFatal {
		t.Fatalf("expected unknown errors to classify as fatal, got %v", got)
	}
}

func TestBoundedExponentialDispatchRetryPolicy(t *testing.T) {
	policy := BoundedExponentialDispatchRetryPolicy{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
	}
	assertDelay := func(attempt int, want time.Duration) {
		t.Helper()
		got, ok := policy.NextDelay(attempt)
		if !ok {
			t.Fatalf("expected attempt %d to be enabled", attempt)
		}
		if got != want {
			t.Fatalf("unexpected delay for attempt %d: got=%s want=%s", attempt, got, want)
		}
	}
	assertDelay(1, 100*time.Millisecond)
	assertDelay(2, 200*time.Millisecond)
	assertDelay(3, 400*time.Millisecond)
	assertDelay(4, 500*time.Millisecond)
	assertDelay(12, 500*time.Millisecond)
}

func TestDispatchRetrySchedulerDeduplicatesRunIDs(t *testing.T) {
	var calls atomic.Int32
	scheduler := NewDispatchRetryScheduler(
		fixedDispatchRetryPolicy{delay: 1 * time.Millisecond},
		func(string) bool {
			calls.Add(1)
			return true
		},
	)
	if scheduler == nil {
		t.Fatalf("expected scheduler")
	}
	defer scheduler.Close()

	scheduler.Enqueue("run-1")
	scheduler.Enqueue("run-1")
	scheduler.Enqueue("run-1")
	time.Sleep(40 * time.Millisecond)

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected one callback for deduped run id, got %d", got)
	}
}

func TestDispatchRetrySchedulerRepeatsUntilDone(t *testing.T) {
	var calls atomic.Int32
	scheduler := NewDispatchRetryScheduler(
		fixedDispatchRetryPolicy{delay: 1 * time.Millisecond},
		func(string) bool {
			return calls.Add(1) >= 3
		},
	)
	if scheduler == nil {
		t.Fatalf("expected scheduler")
	}
	defer scheduler.Close()

	scheduler.Enqueue("run-1")
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) && calls.Load() < 3 {
		time.Sleep(2 * time.Millisecond)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected three retries before done, got %d", got)
	}
}

func TestDispatchRetrySchedulerReturnsNilWhenAttemptNil(t *testing.T) {
	scheduler := NewDispatchRetryScheduler(fixedDispatchRetryPolicy{delay: 1 * time.Millisecond}, nil)
	if scheduler != nil {
		t.Fatalf("expected nil scheduler when attempt callback is nil")
	}
}

func TestDispatchRetrySchedulerUsesDefaultPolicyWhenNil(t *testing.T) {
	var calls atomic.Int32
	scheduler := NewDispatchRetryScheduler(
		nil,
		func(string) bool {
			calls.Add(1)
			return true
		},
	)
	if scheduler == nil {
		t.Fatalf("expected scheduler")
	}
	defer scheduler.Close()

	scheduler.Enqueue("run-default-policy")
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && calls.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatalf("expected callback to be invoked with default retry policy")
	}
}

func TestDispatchRetrySchedulerEnqueueAfterCloseIsIgnored(t *testing.T) {
	var calls atomic.Int32
	scheduler := NewDispatchRetryScheduler(
		fixedDispatchRetryPolicy{delay: 1 * time.Millisecond},
		func(string) bool {
			calls.Add(1)
			return true
		},
	)
	if scheduler == nil {
		t.Fatalf("expected scheduler")
	}
	scheduler.Close()
	scheduler.Enqueue("run-closed")
	time.Sleep(40 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("expected closed scheduler to ignore enqueue, got %d callbacks", got)
	}
}

func TestDispatchRetrySchedulerCloseIsIdempotent(t *testing.T) {
	scheduler := NewDispatchRetryScheduler(
		fixedDispatchRetryPolicy{delay: 1 * time.Millisecond},
		func(string) bool {
			return true
		},
	)
	if scheduler == nil {
		t.Fatalf("expected scheduler")
	}
	scheduler.Close()
	scheduler.Close()
}

func TestDispatchRetrySchedulerStopsWhenPolicyDisablesRetry(t *testing.T) {
	var calls atomic.Int32
	policy := &stoppingDispatchRetryPolicy{}
	scheduler := NewDispatchRetryScheduler(
		policy,
		func(string) bool {
			calls.Add(1)
			return true
		},
	)
	if scheduler == nil {
		t.Fatalf("expected scheduler")
	}
	defer scheduler.Close()

	scheduler.Enqueue("run-1")
	time.Sleep(20 * time.Millisecond)
	scheduler.Enqueue("run-1")
	time.Sleep(20 * time.Millisecond)

	if got := calls.Load(); got != 0 {
		t.Fatalf("expected attempt callback to never run when policy disables retries, got %d", got)
	}
	if got := policy.calls.Load(); got < 2 {
		t.Fatalf("expected policy to be consulted on both enqueues, got %d", got)
	}
}

func TestDispatchRetrySchedulerCloseCancelsPendingTimer(t *testing.T) {
	var calls atomic.Int32
	scheduler := NewDispatchRetryScheduler(
		fixedDispatchRetryPolicy{delay: 500 * time.Millisecond},
		func(string) bool {
			calls.Add(1)
			return true
		},
	)
	if scheduler == nil {
		t.Fatalf("expected scheduler")
	}

	scheduler.Enqueue("run-cancel")
	time.Sleep(25 * time.Millisecond)
	scheduler.Close()
	time.Sleep(550 * time.Millisecond)

	if got := calls.Load(); got != 0 {
		t.Fatalf("expected pending timer to be canceled before callback, got %d callbacks", got)
	}
}
