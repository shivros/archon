package guidedworkflows

import "sync"

// RunLockManager provides per-run locking for finer granularity.
// This allows concurrent operations on different runs without blocking each other.
type RunLockManager interface {
	Lock(runID string) func()
	RLock(runID string) func()
	Remove(runID string)
}

// PerRunLockManager manages individual RWMutex locks per workflow run.
type PerRunLockManager struct {
	mu      sync.Mutex
	locks   map[string]*sync.RWMutex
	cleanup []string
}

// NewPerRunLockManager creates a new PerRunLockManager.
func NewPerRunLockManager() *PerRunLockManager {
	return &PerRunLockManager{
		locks: make(map[string]*sync.RWMutex),
	}
}

// getLock returns the RWMutex for the given runID, creating one if needed.
func (m *PerRunLockManager) getLock(runID string) *sync.RWMutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locks == nil {
		m.locks = make(map[string]*sync.RWMutex)
	}
	if m.locks[runID] == nil {
		m.locks[runID] = &sync.RWMutex{}
	}
	return m.locks[runID]
}

// Lock acquires a write lock for the given runID and returns an unlock function.
func (m *PerRunLockManager) Lock(runID string) func() {
	lock := m.getLock(runID)
	lock.Lock()
	return func() { lock.Unlock() }
}

// RLock acquires a read lock for the given runID and returns an unlock function.
func (m *PerRunLockManager) RLock(runID string) func() {
	lock := m.getLock(runID)
	lock.RLock()
	return func() { lock.RUnlock() }
}

// Remove removes the lock for a runID. Should be called when a run is deleted.
// The lock must not be held when calling this method.
func (m *PerRunLockManager) Remove(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locks != nil {
		delete(m.locks, runID)
	}
}

// noopRunLockManager is a no-op implementation for when per-run locking is disabled.
type noopRunLockManager struct{}

func (noopRunLockManager) Lock(runID string) func()  { return func() {} }
func (noopRunLockManager) RLock(runID string) func() { return func() {} }
func (noopRunLockManager) Remove(runID string)       {}
