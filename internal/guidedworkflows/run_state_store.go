package guidedworkflows

import "sync"

// RunStateStore manages in-memory workflow run state.
// Implementations must be safe for concurrent access.
type RunStateStore interface {
	Get(runID string) (*WorkflowRun, bool)
	GetCloned(runID string) (*WorkflowRun, bool)
	Set(runID string, run *WorkflowRun)
	Delete(runID string)
	List() []*WorkflowRun
	ListIncludingDismissed() []*WorkflowRun
	ListCloned(includeDismissed bool) []*WorkflowRun
	GetTimeline(runID string) []RunTimelineEvent
	GetTimelineCloned(runID string) []RunTimelineEvent
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

func (s *MemoryRunStateStore) GetCloned(runID string) (*WorkflowRun, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[runID]
	if !ok || run == nil {
		return nil, false
	}
	return cloneWorkflowRun(run), true
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

func (s *MemoryRunStateStore) ListCloned(includeDismissed bool) []*WorkflowRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*WorkflowRun, 0, len(s.runs))
	for _, run := range s.runs {
		if run == nil {
			continue
		}
		if !includeDismissed && run.DismissedAt != nil {
			continue
		}
		out = append(out, cloneWorkflowRun(run))
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

func (s *MemoryRunStateStore) GetTimelineCloned(runID string) []RunTimelineEvent {
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

// SharedLockedRunStateStore uses externally-owned maps.
// The owning service is responsible for synchronization.
type SharedLockedRunStateStore struct {
	mu        *sync.RWMutex
	runs      map[string]*WorkflowRun
	timelines map[string][]RunTimelineEvent
}

func NewSharedLockedRunStateStore(
	mu *sync.RWMutex,
	runs map[string]*WorkflowRun,
	timelines map[string][]RunTimelineEvent,
) *SharedLockedRunStateStore {
	if runs == nil {
		runs = make(map[string]*WorkflowRun)
	}
	if timelines == nil {
		timelines = make(map[string][]RunTimelineEvent)
	}
	return &SharedLockedRunStateStore{
		mu:        mu,
		runs:      runs,
		timelines: timelines,
	}
}

func (s *SharedLockedRunStateStore) Get(runID string) (*WorkflowRun, bool) {
	if s == nil {
		return nil, false
	}
	run, ok := s.runs[runID]
	return run, ok
}

func (s *SharedLockedRunStateStore) Set(runID string, run *WorkflowRun) {
	if s == nil {
		return
	}
	if s.runs == nil {
		s.runs = make(map[string]*WorkflowRun)
	}
	s.runs[runID] = run
}

func (s *SharedLockedRunStateStore) GetCloned(runID string) (*WorkflowRun, bool) {
	if s == nil {
		return nil, false
	}
	run, ok := s.runs[runID]
	if !ok || run == nil {
		return nil, false
	}
	return cloneWorkflowRun(run), true
}

func (s *SharedLockedRunStateStore) Delete(runID string) {
	if s == nil {
		return
	}
	delete(s.runs, runID)
	delete(s.timelines, runID)
}

func (s *SharedLockedRunStateStore) List() []*WorkflowRun {
	if s == nil {
		return nil
	}
	out := make([]*WorkflowRun, 0, len(s.runs))
	for _, run := range s.runs {
		if run == nil || run.DismissedAt != nil {
			continue
		}
		out = append(out, run)
	}
	return out
}

func (s *SharedLockedRunStateStore) ListIncludingDismissed() []*WorkflowRun {
	if s == nil {
		return nil
	}
	out := make([]*WorkflowRun, 0, len(s.runs))
	for _, run := range s.runs {
		if run == nil {
			continue
		}
		out = append(out, run)
	}
	return out
}

func (s *SharedLockedRunStateStore) ListCloned(includeDismissed bool) []*WorkflowRun {
	if s == nil {
		return nil
	}
	out := make([]*WorkflowRun, 0, len(s.runs))
	for _, run := range s.runs {
		if run == nil {
			continue
		}
		if !includeDismissed && run.DismissedAt != nil {
			continue
		}
		out = append(out, cloneWorkflowRun(run))
	}
	return out
}

func (s *SharedLockedRunStateStore) GetTimeline(runID string) []RunTimelineEvent {
	if s == nil {
		return nil
	}
	return append([]RunTimelineEvent(nil), s.timelines[runID]...)
}

func (s *SharedLockedRunStateStore) GetTimelineCloned(runID string) []RunTimelineEvent {
	if s == nil {
		return nil
	}
	return append([]RunTimelineEvent(nil), s.timelines[runID]...)
}

func (s *SharedLockedRunStateStore) AppendTimeline(runID string, events ...RunTimelineEvent) {
	if s == nil {
		return
	}
	if s.timelines == nil {
		s.timelines = make(map[string][]RunTimelineEvent)
	}
	s.timelines[runID] = append(s.timelines[runID], events...)
}

func (s *SharedLockedRunStateStore) SetTimeline(runID string, timeline []RunTimelineEvent) {
	if s == nil {
		return
	}
	if s.timelines == nil {
		s.timelines = make(map[string][]RunTimelineEvent)
	}
	s.timelines[runID] = timeline
}
