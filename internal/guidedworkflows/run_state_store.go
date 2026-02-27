package guidedworkflows

import "sync"

// RunStateStore manages in-memory workflow run state.
// Implementations must be safe for concurrent access.
type RunStateStore interface {
	Get(runID string) (*WorkflowRun, bool)
	Set(runID string, run *WorkflowRun)
	Delete(runID string)
	List() []*WorkflowRun
	ListIncludingDismissed() []*WorkflowRun
	GetTimeline(runID string) []RunTimelineEvent
	AppendTimeline(runID string, events ...RunTimelineEvent)
	SetTimeline(runID string, timeline []RunTimelineEvent)
}

// MemoryRunStateStore is an in-memory implementation of RunStateStore.
type MemoryRunStateStore struct {
	mu        sync.RWMutex
	runs      map[string]*WorkflowRun
	timelines map[string][]RunTimelineEvent
}

func NewMemoryRunStateStore() *MemoryRunStateStore {
	return &MemoryRunStateStore{
		runs:      make(map[string]*WorkflowRun),
		timelines: make(map[string][]RunTimelineEvent),
	}
}

func (s *MemoryRunStateStore) Get(runID string) (*WorkflowRun, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[runID]
	return run, ok
}

func (s *MemoryRunStateStore) Set(runID string, run *WorkflowRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs == nil {
		s.runs = make(map[string]*WorkflowRun)
	}
	s.runs[runID] = run
}

func (s *MemoryRunStateStore) Delete(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runs, runID)
}

func (s *MemoryRunStateStore) List() []*WorkflowRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*WorkflowRun, 0, len(s.runs))
	for _, run := range s.runs {
		if run == nil || run.DismissedAt != nil {
			continue
		}
		out = append(out, run)
	}
	return out
}

func (s *MemoryRunStateStore) ListIncludingDismissed() []*WorkflowRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*WorkflowRun, 0, len(s.runs))
	for _, run := range s.runs {
		if run == nil {
			continue
		}
		out = append(out, run)
	}
	return out
}

func (s *MemoryRunStateStore) GetTimeline(runID string) []RunTimelineEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.timelines == nil {
		return nil
	}
	return append([]RunTimelineEvent(nil), s.timelines[runID]...)
}

func (s *MemoryRunStateStore) AppendTimeline(runID string, events ...RunTimelineEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timelines == nil {
		s.timelines = make(map[string][]RunTimelineEvent)
	}
	s.timelines[runID] = append(s.timelines[runID], events...)
}

func (s *MemoryRunStateStore) SetTimeline(runID string, timeline []RunTimelineEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timelines == nil {
		s.timelines = make(map[string][]RunTimelineEvent)
	}
	s.timelines[runID] = timeline
}
