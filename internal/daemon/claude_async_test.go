package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"control/internal/types"
)

func TestClaudeTurnSchedulerSerializesJobs(t *testing.T) {
	executor := &schedulerOrderExecutor{
		started: make(chan string, 2),
		allow:   make(chan struct{}, 2),
	}
	scheduler := newClaudeTurnScheduler(8, executor, nil, nil, nil)
	defer scheduler.Close()

	job1 := claudeTurnJob{
		session:  &types.Session{ID: "s1", Provider: "claude"},
		prepared: claudePreparedTurn{TurnID: "turn-1"},
	}
	job2 := claudeTurnJob{
		session:  &types.Session{ID: "s1", Provider: "claude"},
		prepared: claudePreparedTurn{TurnID: "turn-2"},
	}
	if err := scheduler.Enqueue(job1); err != nil {
		t.Fatalf("enqueue job1: %v", err)
	}
	if err := scheduler.Enqueue(job2); err != nil {
		t.Fatalf("enqueue job2: %v", err)
	}

	select {
	case got := <-executor.started:
		if got != "turn-1" {
			t.Fatalf("expected first started turn-1, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected first scheduled turn to start")
	}

	select {
	case got := <-executor.started:
		t.Fatalf("unexpected concurrent execution: %q", got)
	case <-time.After(80 * time.Millisecond):
	}

	executor.allow <- struct{}{}
	select {
	case got := <-executor.started:
		if got != "turn-2" {
			t.Fatalf("expected second started turn-2, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected second scheduled turn to start after first completed")
	}
	executor.allow <- struct{}{}
}

func TestClaudeTurnSchedulerReportsFailures(t *testing.T) {
	executor := &schedulerOrderExecutor{
		execErr: errors.New("boom"),
		started: make(chan string, 1),
		allow:   make(chan struct{}, 1),
	}
	reporter := &capturingFailureReporter{}
	scheduler := newClaudeTurnScheduler(2, executor, reporter, nil, nil)
	defer scheduler.Close()

	job := claudeTurnJob{
		session:  &types.Session{ID: "s1", Provider: "claude"},
		prepared: claudePreparedTurn{TurnID: "turn-fail"},
	}
	if err := scheduler.Enqueue(job); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	select {
	case <-executor.started:
		executor.allow <- struct{}{}
	case <-time.After(time.Second):
		t.Fatalf("expected scheduler to execute job")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		reporter.mu.Lock()
		count := reporter.calls
		lastErr := reporter.lastErr
		reporter.mu.Unlock()
		if count > 0 {
			if lastErr == nil || !strings.Contains(lastErr.Error(), "boom") {
				t.Fatalf("expected captured failure error, got %v", lastErr)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected failure reporter call")
}

func TestClaudeTurnSchedulerEnqueueNilReceiver(t *testing.T) {
	var scheduler *claudeTurnScheduler
	err := scheduler.Enqueue(claudeTurnJob{})
	if err == nil {
		t.Fatalf("expected unavailable error for nil scheduler")
	}
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
}

func TestClaudeTurnSchedulerEnqueueAfterClose(t *testing.T) {
	scheduler := newClaudeTurnScheduler(1, &schedulerOrderExecutor{
		started: make(chan string, 1),
		allow:   make(chan struct{}, 1),
	}, nil, nil, nil)
	scheduler.Close()
	err := scheduler.Enqueue(claudeTurnJob{})
	if err == nil {
		t.Fatalf("expected invalid error for closed scheduler")
	}
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
}

func TestClaudeTurnSchedulerEnqueueQueueFull(t *testing.T) {
	executor := &schedulerOrderExecutor{
		started: make(chan string, 1),
		allow:   make(chan struct{}),
	}
	scheduler := newClaudeTurnScheduler(1, executor, nil, nil, nil)
	defer scheduler.Close()

	if err := scheduler.Enqueue(claudeTurnJob{prepared: claudePreparedTurn{TurnID: "turn-1"}}); err != nil {
		t.Fatalf("enqueue first job: %v", err)
	}
	// Wait until first job is actively executing, then queue second (buffered),
	// and verify third overflows.
	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatalf("first job did not start")
	}
	if err := scheduler.Enqueue(claudeTurnJob{prepared: claudePreparedTurn{TurnID: "turn-2"}}); err != nil {
		t.Fatalf("enqueue buffered job: %v", err)
	}
	err := scheduler.Enqueue(claudeTurnJob{prepared: claudePreparedTurn{TurnID: "turn-3"}})
	if err == nil {
		t.Fatalf("expected queue-full error")
	}
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
}

func TestClaudeTurnSchedulerCloseIdempotent(t *testing.T) {
	scheduler := newClaudeTurnScheduler(1, nil, nil, nil, nil)
	scheduler.Close()
	scheduler.Close()
}

func TestDefaultClaudeTurnFailureReporterFanout(t *testing.T) {
	repository := &recordingTurnArtifactRepository{}
	notifier := &stubTurnCompletionNotifier{}
	debugWriter := &stubClaudeDebugWriter{}
	reporter := defaultClaudeTurnFailureReporter{
		sessionID:    "s-default",
		providerName: "claude",
		debugWriter:  debugWriter,
		repository:   repository,
		notifier:     notifier,
	}
	job := claudeTurnJob{
		session: &types.Session{ID: "s1", Provider: "claude"},
		meta: &types.SessionMeta{
			WorkspaceID: "ws1",
			WorktreeID:  "wt1",
		},
		prepared: claudePreparedTurn{TurnID: "turn-1"},
	}
	reporter.Report(job, errors.New("send failed"))

	if got := strings.TrimSpace(debugWriter.lastChunk); got == "" || !strings.Contains(got, "turn-1") {
		t.Fatalf("expected debug failure chunk, got %q", got)
	}
	items := repository.Snapshot()
	if len(items) == 0 {
		t.Fatalf("expected failure artifact items")
	}
	if asString(items[0]["turn_id"]) != "turn-1" {
		t.Fatalf("expected turn_id in artifact, got %#v", items[0]["turn_id"])
	}
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if notifier.calls == 0 {
		t.Fatalf("expected failure completion notification")
	}
	if notifier.lastEvent.Status != "failed" {
		t.Fatalf("expected failed status, got %q", notifier.lastEvent.Status)
	}
	if notifier.lastEvent.WorkspaceID != "ws1" || notifier.lastEvent.WorktreeID != "wt1" {
		t.Fatalf("expected workspace/worktree metadata, got %+v", notifier.lastEvent)
	}
}

func TestDefaultClaudeTurnFailureReporterNilErrorNoop(t *testing.T) {
	reporter := defaultClaudeTurnFailureReporter{
		repository: &recordingTurnArtifactRepository{},
		notifier:   &stubTurnCompletionNotifier{},
	}
	reporter.Report(claudeTurnJob{}, nil)
	repo := reporter.repository.(*recordingTurnArtifactRepository)
	if len(repo.Snapshot()) != 0 {
		t.Fatalf("expected no artifacts for nil error")
	}
}

func TestDefaultClaudeTurnFailureReporterFallbackProvider(t *testing.T) {
	notifier := &stubTurnCompletionNotifier{}
	reporter := defaultClaudeTurnFailureReporter{
		sessionID: "s1",
		notifier:  notifier,
	}
	reporter.Report(claudeTurnJob{
		prepared: claudePreparedTurn{TurnID: "turn-p"},
	}, errors.New("boom"))
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if notifier.calls == 0 {
		t.Fatalf("expected notification call")
	}
	if notifier.lastEvent.Provider != "claude" {
		t.Fatalf("expected fallback provider claude, got %q", notifier.lastEvent.Provider)
	}
}

func TestDefaultClaudeTurnFailureReporterWithoutNotifier(t *testing.T) {
	repository := &recordingTurnArtifactRepository{}
	reporter := defaultClaudeTurnFailureReporter{
		sessionID:  "s1",
		repository: repository,
	}
	reporter.Report(claudeTurnJob{
		prepared: claudePreparedTurn{TurnID: "turn-no-notifier"},
	}, errors.New("err"))
	if len(repository.Snapshot()) == 0 {
		t.Fatalf("expected artifacts even without notifier")
	}
}

type schedulerOrderExecutor struct {
	execErr error
	started chan string
	allow   chan struct{}
}

func (e *schedulerOrderExecutor) ExecutePreparedTurn(
	_ context.Context,
	_ claudeSendContext,
	_ *types.Session,
	_ *types.SessionMeta,
	prepared claudePreparedTurn,
) error {
	e.started <- strings.TrimSpace(prepared.TurnID)
	<-e.allow
	return e.execErr
}

type capturingFailureReporter struct {
	mu      sync.Mutex
	calls   int
	lastJob claudeTurnJob
	lastErr error
}

func (r *capturingFailureReporter) Report(job claudeTurnJob, turnErr error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.lastJob = job
	r.lastErr = turnErr
}

type stubClaudeDebugWriter struct {
	mu         sync.Mutex
	lastID     string
	lastStream string
	lastChunk  string
}

func (s *stubClaudeDebugWriter) WriteSessionDebug(id, stream string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastID = id
	s.lastStream = stream
	s.lastChunk = string(data)
	return nil
}
