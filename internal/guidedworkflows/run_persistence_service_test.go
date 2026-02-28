package guidedworkflows

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type persistenceStoreStub struct {
	mu           sync.Mutex
	calls        int
	usedCanceled bool
}

func (s *persistenceStoreStub) ListWorkflowRuns(context.Context) ([]RunStatusSnapshot, error) {
	return nil, nil
}

func (s *persistenceStoreStub) UpsertWorkflowRun(ctx context.Context, snapshot RunStatusSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if ctx != nil && ctx.Err() != nil {
		s.usedCanceled = true
	}
	return nil
}

func TestRunPersistenceServiceNoopWhenStoreNil(t *testing.T) {
	p := newRunPersistenceService(nil)
	if p == nil {
		t.Fatalf("expected noop persistence service")
	}
	p.PersistSync(context.Background(), RunStatusSnapshot{})
	p.PersistAsync(context.Background(), RunStatusSnapshot{})
	p.Flush()
}

func TestRunPersistenceServiceAsyncFlushPersists(t *testing.T) {
	store := &persistenceStoreStub{}
	p := newRunPersistenceService(store)
	if p == nil {
		t.Fatalf("expected persistence service")
	}
	snapshot := RunStatusSnapshot{
		Run: &WorkflowRun{ID: "gwf-persist"},
	}
	p.PersistAsync(context.Background(), snapshot)
	p.Flush()

	store.mu.Lock()
	calls := store.calls
	store.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected one async persistence call, got %d", calls)
	}
}

func TestRunPersistenceServiceAsyncIgnoresCanceledContext(t *testing.T) {
	store := &persistenceStoreStub{}
	p := newRunPersistenceService(store)
	if p == nil {
		t.Fatalf("expected persistence service")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.PersistAsync(ctx, RunStatusSnapshot{Run: &WorkflowRun{ID: "gwf-canceled"}})
	p.Flush()

	store.mu.Lock()
	usedCanceled := store.usedCanceled
	store.mu.Unlock()
	if usedCanceled {
		t.Fatalf("expected async persistence to avoid canceled context propagation")
	}
}

func TestRunServiceOptionWithRunPersistenceService(t *testing.T) {
	store := &persistenceStoreStub{}
	p := newRunPersistenceService(store)
	service := NewRunService(Config{Enabled: true}, WithRunPersistenceService(p))
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if strings.TrimSpace(run.ID) == "" {
		t.Fatalf("expected created run id")
	}
	service.Close()
	store.mu.Lock()
	calls := store.calls
	store.mu.Unlock()
	if calls == 0 {
		t.Fatalf("expected persistence service to be used")
	}
}
