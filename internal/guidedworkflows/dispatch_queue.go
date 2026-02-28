package guidedworkflows

import (
	"context"
	"strings"
	"sync"
)

// DispatchRequest describes a queued dispatch-related operation.
type DispatchRequest struct {
	RunID  string
	Reason string
}

type DispatchQueueResult struct {
	Done bool
	Err  error
}

// DispatchQueue processes dispatch requests asynchronously via worker goroutines.
// Enqueue waits for the worker result for a given request.
type DispatchQueue interface {
	Enqueue(ctx context.Context, req DispatchRequest) (result DispatchQueueResult, ok bool)
	Close()
}

type dispatchQueueWorkerFunc func(req DispatchRequest) DispatchQueueResult

type dispatchQueueTask struct {
	req      DispatchRequest
	resultCh chan DispatchQueueResult
}

type channelDispatchQueue struct {
	mu     sync.RWMutex
	closed bool
	tasks  chan dispatchQueueTask
	stopCh chan struct{}
	worker dispatchQueueWorkerFunc
	wg     sync.WaitGroup
}

func NewChannelDispatchQueue(buffer, workers int, worker dispatchQueueWorkerFunc) DispatchQueue {
	if worker == nil {
		return nil
	}
	if buffer <= 0 {
		buffer = 64
	}
	if workers <= 0 {
		workers = 1
	}
	q := &channelDispatchQueue{
		tasks:  make(chan dispatchQueueTask, buffer),
		stopCh: make(chan struct{}),
		worker: worker,
	}
	q.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go q.loop()
	}
	return q
}

func (q *channelDispatchQueue) Enqueue(ctx context.Context, req DispatchRequest) (result DispatchQueueResult, ok bool) {
	if q == nil {
		return DispatchQueueResult{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req.RunID = strings.TrimSpace(req.RunID)
	if req.RunID == "" {
		return DispatchQueueResult{Done: true}, true
	}
	task := dispatchQueueTask{
		req:      req,
		resultCh: make(chan DispatchQueueResult, 1),
	}
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return DispatchQueueResult{}, false
	}
	tasks := q.tasks
	stopCh := q.stopCh
	q.mu.RUnlock()

	select {
	case tasks <- task:
	case <-stopCh:
		return DispatchQueueResult{}, false
	case <-ctx.Done():
		return DispatchQueueResult{}, false
	}

	select {
	case result = <-task.resultCh:
		return result, true
	case <-stopCh:
		return DispatchQueueResult{}, false
	case <-ctx.Done():
		return DispatchQueueResult{}, false
	}
}

func (q *channelDispatchQueue) Close() {
	if q == nil {
		return
	}
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	close(q.stopCh)
	q.mu.Unlock()
	q.wg.Wait()
}

func (q *channelDispatchQueue) loop() {
	defer q.wg.Done()
	for {
		select {
		case <-q.stopCh:
			return
		case task := <-q.tasks:
			result := DispatchQueueResult{}
			if q.worker != nil {
				result = q.worker(task.req)
			}
			task.resultCh <- result
		}
	}
}
