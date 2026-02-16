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

type stubCodexHistoryClient struct {
	thread     *codexThread
	err        error
	closeCalls int
}

func (s *stubCodexHistoryClient) ReadThread(context.Context, string) (*codexThread, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.thread != nil {
		return s.thread, nil
	}
	return &codexThread{}, nil
}

func (s *stubCodexHistoryClient) Close() {
	s.closeCalls++
}
