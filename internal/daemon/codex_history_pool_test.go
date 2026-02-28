package daemon

import (
	"context"
	"errors"
	"testing"

	"control/internal/logging"
)

func TestCodexHistoryPoolReusesClientPerWorkspace(t *testing.T) {
	starts := 0
	pool := &codexHistoryPool{
		clients:    map[string]*pooledCodexHistoryClient{},
		idleTTL:    0,
		maxClients: 4,
		logger:     logging.Nop(),
		startFn: func(context.Context, string, string, logging.Logger) (codexHistoryClient, error) {
			starts++
			return &stubCodexHistoryClient{
				thread: &codexThread{ID: "thread-1"},
			}, nil
		},
	}

	ctx := context.Background()
	for i := 0; i < 2; i++ {
		thread, err := pool.ReadThread(ctx, "/repo", "/repo/.codex", "thread-1")
		if err != nil {
			t.Fatalf("read %d failed: %v", i, err)
		}
		if thread == nil || thread.ID != "thread-1" {
			t.Fatalf("unexpected thread on read %d: %#v", i, thread)
		}
	}
	if starts != 1 {
		t.Fatalf("expected one client start, got %d", starts)
	}
}

func TestCodexHistoryPoolRetriesAfterClosedPipe(t *testing.T) {
	starts := 0
	first := &stubCodexHistoryClient{err: errors.New("broken pipe")}
	second := &stubCodexHistoryClient{thread: &codexThread{ID: "thread-2"}}

	pool := &codexHistoryPool{
		clients:    map[string]*pooledCodexHistoryClient{},
		idleTTL:    0,
		maxClients: 4,
		logger:     logging.Nop(),
		startFn: func(context.Context, string, string, logging.Logger) (codexHistoryClient, error) {
			starts++
			if starts == 1 {
				return first, nil
			}
			return second, nil
		},
	}

	thread, err := pool.ReadThread(context.Background(), "/repo", "/repo/.codex", "thread-2")
	if err != nil {
		t.Fatalf("expected retry to succeed, got err=%v", err)
	}
	if thread == nil || thread.ID != "thread-2" {
		t.Fatalf("unexpected thread after retry: %#v", thread)
	}
	if starts != 2 {
		t.Fatalf("expected two starts after retry, got %d", starts)
	}
	if first.closeCalls != 1 {
		t.Fatalf("expected first client to close once, got %d", first.closeCalls)
	}
}

func TestCodexHistoryPoolRecoversWhenThreadNotLoaded(t *testing.T) {
	client := &stubCodexHistoryClient{
		readErrs: []error{
			errors.New("rpc error -32600: thread not loaded: thread-3"),
			nil,
		},
		readThreads: []*codexThread{
			nil,
			{ID: "thread-3"},
		},
	}

	pool := &codexHistoryPool{
		clients:    map[string]*pooledCodexHistoryClient{},
		idleTTL:    0,
		maxClients: 4,
		logger:     logging.Nop(),
		startFn: func(context.Context, string, string, logging.Logger) (codexHistoryClient, error) {
			return client, nil
		},
	}

	thread, err := pool.ReadThread(context.Background(), "/repo", "/repo/.codex", "thread-3")
	if err != nil {
		t.Fatalf("expected resume recovery to succeed, got err=%v", err)
	}
	if thread == nil || thread.ID != "thread-3" {
		t.Fatalf("unexpected thread after resume recovery: %#v", thread)
	}
	if client.resumeCalls != 1 {
		t.Fatalf("expected one resume call, got %d", client.resumeCalls)
	}
	if client.readCalls != 2 {
		t.Fatalf("expected two read calls, got %d", client.readCalls)
	}
}

func TestCodexHistoryPoolEvictsLRUWhenOverLimit(t *testing.T) {
	clients := map[string]*stubCodexHistoryClient{}
	pool := &codexHistoryPool{
		clients:    map[string]*pooledCodexHistoryClient{},
		idleTTL:    0,
		maxClients: 1,
		logger:     logging.Nop(),
		startFn: func(_ context.Context, cwd, _ string, _ logging.Logger) (codexHistoryClient, error) {
			client := &stubCodexHistoryClient{thread: &codexThread{ID: cwd}}
			clients[cwd] = client
			return client, nil
		},
	}

	_, _ = pool.ReadThread(context.Background(), "repo-a", "", "thread-a")
	_, _ = pool.ReadThread(context.Background(), "repo-b", "", "thread-b")

	if len(pool.clients) != 1 {
		t.Fatalf("expected pool size 1, got %d", len(pool.clients))
	}
	if clients["repo-a"].closeCalls != 1 {
		t.Fatalf("expected repo-a client eviction close, got %d", clients["repo-a"].closeCalls)
	}
}

func TestCodexHistoryPoolRetriesOnNewThreadNotYetIndexed(t *testing.T) {
	client := &stubCodexHistoryClient{
		readErrs: []error{
			// tryReadWithResume attempt 0: ReadThread fails
			errors.New("rpc error -32600: thread not loaded: thread-new"),
			// tryReadWithResume attempt 0: resume fails → no second ReadThread
			// retry 1: ReadThread fails again
			errors.New("rpc error -32600: thread not loaded: thread-new"),
			// retry 1: resume succeeds → second ReadThread succeeds
			nil,
		},
		readThreads: []*codexThread{
			nil,
			nil,
			{ID: "thread-new"},
		},
		resumeErrs: []error{
			errors.New("no rollout found for thread id thread-new"), // attempt 0
			nil, // retry 1: resume succeeds
		},
	}

	pool := &codexHistoryPool{
		clients:    map[string]*pooledCodexHistoryClient{},
		idleTTL:    0,
		maxClients: 4,
		logger:     logging.Nop(),
		startFn: func(context.Context, string, string, logging.Logger) (codexHistoryClient, error) {
			return client, nil
		},
	}

	thread, err := pool.ReadThread(context.Background(), "/repo", "/repo/.codex", "thread-new")
	if err != nil {
		t.Fatalf("expected retry to succeed, got err=%v", err)
	}
	if thread == nil || thread.ID != "thread-new" {
		t.Fatalf("unexpected thread: %#v", thread)
	}
	if client.resumeCalls != 2 {
		t.Fatalf("expected 2 resume calls, got %d", client.resumeCalls)
	}
	if client.readCalls != 3 {
		t.Fatalf("expected 3 read calls, got %d", client.readCalls)
	}
}

func TestCodexHistoryPoolRetryRespectsContextCancellation(t *testing.T) {
	client := &stubCodexHistoryClient{
		readErrs: []error{
			errors.New("rpc error -32600: thread not loaded: thread-x"),
		},
		resumeErrs: []error{
			errors.New("no rollout found for thread id thread-x"),
		},
	}

	pool := &codexHistoryPool{
		clients:    map[string]*pooledCodexHistoryClient{},
		idleTTL:    0,
		maxClients: 4,
		logger:     logging.Nop(),
		startFn: func(context.Context, string, string, logging.Logger) (codexHistoryClient, error) {
			return client, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pool.ReadThread(ctx, "/repo", "/repo/.codex", "thread-x")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

type stubCodexHistoryClient struct {
	thread      *codexThread
	err         error
	readErrs    []error
	readThreads []*codexThread
	readCalls   int
	resumeCalls int
	resumeErr   error
	resumeErrs  []error
	closeCalls  int
}

func (s *stubCodexHistoryClient) ReadThread(context.Context, string) (*codexThread, error) {
	s.readCalls++
	if idx := s.readCalls - 1; idx >= 0 && idx < len(s.readErrs) {
		if s.readErrs[idx] != nil {
			return nil, s.readErrs[idx]
		}
		if idx < len(s.readThreads) && s.readThreads[idx] != nil {
			return s.readThreads[idx], nil
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.thread != nil {
		return s.thread, nil
	}
	return &codexThread{}, nil
}

func (s *stubCodexHistoryClient) ResumeThread(context.Context, string) error {
	s.resumeCalls++
	if idx := s.resumeCalls - 1; idx >= 0 && idx < len(s.resumeErrs) {
		return s.resumeErrs[idx]
	}
	return s.resumeErr
}

func (s *stubCodexHistoryClient) Close() {
	s.closeCalls++
}
