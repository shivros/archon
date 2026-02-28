package guidedworkflows

import (
	"context"
	"sync"
)

// RunPersistenceService handles persistence of run snapshots.
// Implementations must be safe for concurrent use.
type RunPersistenceService interface {
	PersistSync(ctx context.Context, snapshot RunStatusSnapshot)
	PersistAsync(ctx context.Context, snapshot RunStatusSnapshot)
	Flush()
}

type storeBackedRunPersistenceService struct {
	store RunSnapshotStore
	wg    sync.WaitGroup
}

func newRunPersistenceService(store RunSnapshotStore) RunPersistenceService {
	if store == nil {
		return noopRunPersistenceService{}
	}
	return &storeBackedRunPersistenceService{store: store}
}

func (p *storeBackedRunPersistenceService) PersistSync(ctx context.Context, snapshot RunStatusSnapshot) {
	if p == nil || p.store == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = p.store.UpsertWorkflowRun(ctx, snapshot)
}

func (p *storeBackedRunPersistenceService) PersistAsync(ctx context.Context, snapshot RunStatusSnapshot) {
	if p == nil || p.store == nil {
		return
	}
	// Async persistence should be best-effort durable and not be coupled to
	// request-scoped cancellations/timeouts.
	ctx = context.Background()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		_ = p.store.UpsertWorkflowRun(ctx, snapshot)
	}()
}

func (p *storeBackedRunPersistenceService) Flush() {
	if p == nil {
		return
	}
	p.wg.Wait()
}

type noopRunPersistenceService struct{}

func (noopRunPersistenceService) PersistSync(context.Context, RunStatusSnapshot)  {}
func (noopRunPersistenceService) PersistAsync(context.Context, RunStatusSnapshot) {}
func (noopRunPersistenceService) Flush()                                          {}
