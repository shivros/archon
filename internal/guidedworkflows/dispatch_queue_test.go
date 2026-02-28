package guidedworkflows

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChannelDispatchQueueNilWorker(t *testing.T) {
	if q := NewChannelDispatchQueue(8, 1, nil); q != nil {
		t.Fatalf("expected nil queue when worker is nil")
	}
}

func TestChannelDispatchQueueEnqueueProcessesRequest(t *testing.T) {
	var calls atomic.Int32
	q := NewChannelDispatchQueue(4, 1, func(req DispatchRequest) DispatchQueueResult {
		if req.RunID != "run-1" {
			t.Fatalf("unexpected run id: %q", req.RunID)
		}
		calls.Add(1)
		return DispatchQueueResult{Done: true}
	})
	if q == nil {
		t.Fatalf("expected queue")
	}
	defer q.Close()

	result, ok := q.Enqueue(context.Background(), DispatchRequest{RunID: "run-1"})
	if !ok || !result.Done || result.Err != nil {
		t.Fatalf("expected successful enqueue result, got ok=%v result=%#v", ok, result)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 worker call, got %d", calls.Load())
	}
}

func TestChannelDispatchQueueCloseAndConcurrentEnqueueNoPanic(t *testing.T) {
	q := NewChannelDispatchQueue(16, 1, func(DispatchRequest) DispatchQueueResult {
		time.Sleep(1 * time.Millisecond)
		return DispatchQueueResult{Done: true}
	})
	if q == nil {
		t.Fatalf("expected queue")
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = q.Enqueue(context.Background(), DispatchRequest{RunID: "run-race"})
		}()
	}
	time.Sleep(2 * time.Millisecond)
	q.Close()
	q.Close()
	wg.Wait()
}

func TestChannelDispatchQueueEnqueueAfterClose(t *testing.T) {
	q := NewChannelDispatchQueue(1, 1, func(DispatchRequest) DispatchQueueResult {
		return DispatchQueueResult{Done: true}
	})
	if q == nil {
		t.Fatalf("expected queue")
	}
	q.Close()

	result, ok := q.Enqueue(context.Background(), DispatchRequest{RunID: "run-closed"})
	if ok || result.Done || result.Err != nil {
		t.Fatalf("expected closed queue to reject enqueue, got ok=%v result=%#v", ok, result)
	}
}
