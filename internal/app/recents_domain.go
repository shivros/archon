package app

import (
	"sort"
	"strings"
	"time"

	"control/internal/types"
)

type recentsRun struct {
	SessionID      string
	BaselineTurnID string
	StartedAt      time.Time
}

type recentsReadyItem struct {
	SessionID       string
	CompletionTurn  string
	CompletedAt     time.Time
	LastKnownTurnID string
}

type recentsReadyQueueEntry struct {
	SessionID string
	Seq       int64
}

type RecentsEventType string

const (
	RecentsEventRunStarted    RecentsEventType = "run_started"
	RecentsEventRunCanceled   RecentsEventType = "run_canceled"
	RecentsEventRunCompleted  RecentsEventType = "run_completed"
	RecentsEventMetaObserved  RecentsEventType = "meta_observed"
	RecentsEventReadyDismiss  RecentsEventType = "ready_dismissed"
	RecentsEventSessionsPrune RecentsEventType = "sessions_pruned"
)

type RecentsEvent struct {
	Type              RecentsEventType
	SessionID         string
	BaselineTurnID    string
	ExpectedTurnID    string
	CompletionTurnID  string
	ObservedTurnID    string
	At                time.Time
	PresentSessionIDs []string
}

type RecentsTransition struct {
	Changed       bool
	ReadyEnqueued bool
	ReadyItem     recentsReadyItem
	Ignored       bool
	Reason        string
}

type RecentsSnapshot struct {
	Running       map[string]recentsRun
	Ready         map[string]recentsReadyItem
	ReadyQueue    []recentsReadyQueueEntry
	DismissedTurn map[string]string
}

type RecentsStateMachine struct {
	running       map[string]recentsRun
	ready         map[string]recentsReadyItem
	readyQueue    []recentsReadyQueueEntry
	dismissedTurn map[string]string
	nextReadySeq  int64
}

type RecentsTracker struct {
	stateMachine *RecentsStateMachine
}

var _ recentsDomain = (*RecentsTracker)(nil)

const recentsUnknownCompletionTurn = "__unknown_completion__"

func NewRecentsStateMachine() *RecentsStateMachine {
	return &RecentsStateMachine{
		running:       map[string]recentsRun{},
		ready:         map[string]recentsReadyItem{},
		readyQueue:    []recentsReadyQueueEntry{},
		dismissedTurn: map[string]string{},
	}
}

func (s *RecentsStateMachine) Snapshot() RecentsSnapshot {
	if s == nil {
		return RecentsSnapshot{}
	}
	running := make(map[string]recentsRun, len(s.running))
	for key, value := range s.running {
		running[key] = value
	}
	ready := make(map[string]recentsReadyItem, len(s.ready))
	for key, value := range s.ready {
		ready[key] = value
	}
	dismissedTurn := make(map[string]string, len(s.dismissedTurn))
	for key, value := range s.dismissedTurn {
		dismissedTurn[key] = value
	}
	readyQueue := append([]recentsReadyQueueEntry(nil), s.readyQueue...)
	return RecentsSnapshot{
		Running:       running,
		Ready:         ready,
		ReadyQueue:    readyQueue,
		DismissedTurn: dismissedTurn,
	}
}

func (s *RecentsStateMachine) Apply(event RecentsEvent) RecentsTransition {
	if s == nil {
		return RecentsTransition{Ignored: true, Reason: "nil state machine"}
	}
	switch event.Type {
	case RecentsEventRunStarted:
		return s.applyRunStarted(event)
	case RecentsEventRunCanceled:
		return s.applyRunCanceled(event)
	case RecentsEventRunCompleted:
		return s.applyRunCompleted(event)
	case RecentsEventMetaObserved:
		return s.applyMetaObserved(event)
	case RecentsEventReadyDismiss:
		return s.applyReadyDismiss(event)
	case RecentsEventSessionsPrune:
		return s.applySessionsPrune(event)
	default:
		return RecentsTransition{Ignored: true, Reason: "unknown event"}
	}
}

func (s *RecentsStateMachine) RunningIDs() []string {
	if s == nil || len(s.running) == 0 {
		return nil
	}
	runs := make([]recentsRun, 0, len(s.running))
	for _, run := range s.running {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		left := runs[i].StartedAt
		right := runs[j].StartedAt
		if left.Equal(right) {
			return runs[i].SessionID < runs[j].SessionID
		}
		return left.Before(right)
	})
	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.SessionID)
	}
	return ids
}

func (s *RecentsStateMachine) ReadyIDs() []string {
	if s == nil || len(s.readyQueue) == 0 {
		return nil
	}
	ids := make([]string, 0, len(s.readyQueue))
	seen := make(map[string]struct{}, len(s.readyQueue))
	for _, entry := range s.readyQueue {
		sessionID := strings.TrimSpace(entry.SessionID)
		if sessionID == "" {
			continue
		}
		if _, exists := seen[sessionID]; exists {
			continue
		}
		if _, ok := s.ready[sessionID]; !ok {
			continue
		}
		ids = append(ids, sessionID)
		seen[sessionID] = struct{}{}
	}
	return ids
}

func (s *RecentsStateMachine) ReadyItem(sessionID string) (recentsReadyItem, bool) {
	if s == nil {
		return recentsReadyItem{}, false
	}
	item, ok := s.ready[strings.TrimSpace(sessionID)]
	if !ok {
		return recentsReadyItem{}, false
	}
	return item, true
}

func (s *RecentsStateMachine) IsReady(sessionID string) bool {
	if s == nil {
		return false
	}
	_, ok := s.ready[strings.TrimSpace(sessionID)]
	return ok
}

func (s *RecentsStateMachine) IsRunning(sessionID string) bool {
	if s == nil {
		return false
	}
	_, ok := s.running[strings.TrimSpace(sessionID)]
	return ok
}

func (s *RecentsStateMachine) ReadyCount() int {
	if s == nil {
		return 0
	}
	return len(s.ready)
}

func (s *RecentsStateMachine) RunningCount() int {
	if s == nil {
		return 0
	}
	return len(s.running)
}

func (s *RecentsStateMachine) applyRunStarted(event RecentsEvent) RecentsTransition {
	sessionID := strings.TrimSpace(event.SessionID)
	if sessionID == "" {
		return RecentsTransition{Ignored: true, Reason: "missing session id"}
	}
	baselineTurnID := strings.TrimSpace(event.BaselineTurnID)
	startedAt := event.At.UTC()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	changed := false
	if s.removeReady(sessionID) {
		changed = true
	}
	if _, dismissed := s.dismissedTurn[sessionID]; dismissed {
		delete(s.dismissedTurn, sessionID)
		changed = true
	}
	if current, exists := s.running[sessionID]; exists && strings.TrimSpace(current.BaselineTurnID) == baselineTurnID {
		// Duplicate start events for the same cycle are idempotent and should not
		// perturb running order.
		return RecentsTransition{Changed: changed}
	}
	nextRun := recentsRun{
		SessionID:      sessionID,
		BaselineTurnID: baselineTurnID,
		StartedAt:      startedAt,
	}
	if current, exists := s.running[sessionID]; !exists || current != nextRun {
		s.running[sessionID] = nextRun
		changed = true
	}
	return RecentsTransition{Changed: changed}
}

func (s *RecentsStateMachine) applyRunCanceled(event RecentsEvent) RecentsTransition {
	sessionID := strings.TrimSpace(event.SessionID)
	if sessionID == "" {
		return RecentsTransition{Ignored: true, Reason: "missing session id"}
	}
	if _, ok := s.running[sessionID]; !ok {
		return RecentsTransition{Ignored: true, Reason: "run not found"}
	}
	delete(s.running, sessionID)
	return RecentsTransition{Changed: true}
}

func (s *RecentsStateMachine) applyMetaObserved(event RecentsEvent) RecentsTransition {
	sessionID := strings.TrimSpace(event.SessionID)
	if sessionID == "" {
		return RecentsTransition{Ignored: true, Reason: "missing session id"}
	}
	run, ok := s.running[sessionID]
	if !ok {
		return RecentsTransition{Ignored: true, Reason: "run not found"}
	}
	observedTurnID := strings.TrimSpace(event.ObservedTurnID)
	if observedTurnID == "" || observedTurnID == strings.TrimSpace(run.BaselineTurnID) {
		return RecentsTransition{Ignored: true, Reason: "meta has no completion turn"}
	}
	return s.applyRunCompleted(RecentsEvent{
		Type:             RecentsEventRunCompleted,
		SessionID:        sessionID,
		CompletionTurnID: observedTurnID,
		At:               event.At,
	})
}

func (s *RecentsStateMachine) applyRunCompleted(event RecentsEvent) RecentsTransition {
	sessionID := strings.TrimSpace(event.SessionID)
	if sessionID == "" {
		return RecentsTransition{Ignored: true, Reason: "missing session id"}
	}
	expectedTurn := strings.TrimSpace(event.ExpectedTurnID)
	completionTurn := strings.TrimSpace(event.CompletionTurnID)
	completedAt := event.At.UTC()
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	run, running := s.running[sessionID]
	if !running {
		if completionTurn != "" {
			if ready, ok := s.ready[sessionID]; ok && strings.TrimSpace(ready.CompletionTurn) == completionTurn {
				return RecentsTransition{Ignored: true, Reason: "duplicate completion already ready"}
			}
			if dismissed, ok := s.dismissedTurn[sessionID]; ok && strings.TrimSpace(dismissed) == completionTurn {
				return RecentsTransition{Ignored: true, Reason: "duplicate completion already dismissed"}
			}
		}
		return RecentsTransition{Ignored: true, Reason: "run not found"}
	}
	if expectedTurn != "" && strings.TrimSpace(run.BaselineTurnID) != expectedTurn {
		return RecentsTransition{Ignored: true, Reason: "expected turn mismatch"}
	}
	if completionTurn == "" {
		completionTurn = strings.TrimSpace(run.BaselineTurnID)
	}
	if completionTurn == "" {
		completionTurn = recentsUnknownCompletionTurn
	}
	delete(s.running, sessionID)
	item, enqueued := s.enqueueReady(sessionID, completionTurn, completedAt)
	return RecentsTransition{
		Changed:       true,
		ReadyEnqueued: enqueued,
		ReadyItem:     item,
	}
}

func (s *RecentsStateMachine) applyReadyDismiss(event RecentsEvent) RecentsTransition {
	sessionID := strings.TrimSpace(event.SessionID)
	if sessionID == "" {
		return RecentsTransition{Ignored: true, Reason: "missing session id"}
	}
	item, ok := s.ready[sessionID]
	if !ok {
		return RecentsTransition{Ignored: true, Reason: "ready item not found"}
	}
	s.dismissedTurn[sessionID] = strings.TrimSpace(item.CompletionTurn)
	_ = s.removeReady(sessionID)
	return RecentsTransition{Changed: true}
}

func (s *RecentsStateMachine) applySessionsPrune(event RecentsEvent) RecentsTransition {
	if len(event.PresentSessionIDs) == 0 {
		changed := len(s.running) > 0 || len(s.ready) > 0 || len(s.readyQueue) > 0 || len(s.dismissedTurn) > 0
		s.running = map[string]recentsRun{}
		s.ready = map[string]recentsReadyItem{}
		s.readyQueue = nil
		s.dismissedTurn = map[string]string{}
		s.nextReadySeq = 0
		return RecentsTransition{Changed: changed}
	}
	present := make(map[string]struct{}, len(event.PresentSessionIDs))
	for _, id := range event.PresentSessionIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		present[trimmed] = struct{}{}
	}
	changed := false
	for id := range s.running {
		if _, ok := present[id]; !ok {
			delete(s.running, id)
			changed = true
		}
	}
	for id := range s.ready {
		if _, ok := present[id]; !ok {
			delete(s.ready, id)
			changed = true
		}
	}
	for id := range s.dismissedTurn {
		if _, ok := present[id]; !ok {
			delete(s.dismissedTurn, id)
			changed = true
		}
	}
	if s.compactReadyQueue() {
		changed = true
	}
	return RecentsTransition{Changed: changed}
}

func (s *RecentsStateMachine) enqueueReady(sessionID, completionTurn string, completedAt time.Time) (recentsReadyItem, bool) {
	sessionID = strings.TrimSpace(sessionID)
	completionTurn = strings.TrimSpace(completionTurn)
	if sessionID == "" || completionTurn == "" {
		return recentsReadyItem{}, false
	}
	if dismissed, ok := s.dismissedTurn[sessionID]; ok && strings.TrimSpace(dismissed) == completionTurn {
		return recentsReadyItem{}, false
	}
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	completedAt = completedAt.UTC()
	item := recentsReadyItem{
		SessionID:       sessionID,
		CompletionTurn:  completionTurn,
		LastKnownTurnID: completionTurn,
		CompletedAt:     completedAt,
	}
	if existing, ok := s.ready[sessionID]; ok {
		if strings.TrimSpace(existing.CompletionTurn) == completionTurn {
			return existing, false
		}
		_ = s.removeReady(sessionID)
	}
	s.nextReadySeq++
	s.ready[sessionID] = item
	s.readyQueue = append(s.readyQueue, recentsReadyQueueEntry{
		SessionID: sessionID,
		Seq:       s.nextReadySeq,
	})
	return item, true
}

func (s *RecentsStateMachine) removeReady(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	if _, exists := s.ready[sessionID]; !exists {
		return false
	}
	delete(s.ready, sessionID)
	_ = s.compactReadyQueue()
	return true
}

func (s *RecentsStateMachine) compactReadyQueue() bool {
	if len(s.readyQueue) == 0 {
		return false
	}
	before := len(s.readyQueue)
	seen := make(map[string]struct{}, len(s.readyQueue))
	filtered := s.readyQueue[:0]
	for _, entry := range s.readyQueue {
		sessionID := strings.TrimSpace(entry.SessionID)
		if sessionID == "" {
			continue
		}
		if _, ok := s.ready[sessionID]; !ok {
			continue
		}
		if _, duplicate := seen[sessionID]; duplicate {
			continue
		}
		seen[sessionID] = struct{}{}
		filtered = append(filtered, recentsReadyQueueEntry{
			SessionID: sessionID,
			Seq:       entry.Seq,
		})
	}
	s.readyQueue = filtered
	return len(s.readyQueue) != before
}

func NewRecentsTracker() *RecentsTracker {
	return &RecentsTracker{stateMachine: NewRecentsStateMachine()}
}

func (t *RecentsTracker) machine() *RecentsStateMachine {
	if t == nil {
		return nil
	}
	if t.stateMachine == nil {
		t.stateMachine = NewRecentsStateMachine()
	}
	return t.stateMachine
}

func (t *RecentsTracker) StartRun(sessionID, baselineTurnID string, startedAt time.Time) {
	if sm := t.machine(); sm != nil {
		sm.Apply(RecentsEvent{
			Type:           RecentsEventRunStarted,
			SessionID:      sessionID,
			BaselineTurnID: baselineTurnID,
			At:             startedAt,
		})
	}
}

func (t *RecentsTracker) CancelRun(sessionID string) {
	if sm := t.machine(); sm != nil {
		sm.Apply(RecentsEvent{
			Type:      RecentsEventRunCanceled,
			SessionID: sessionID,
		})
	}
}

func (t *RecentsTracker) ObserveMeta(meta map[string]*types.SessionMeta, observedAt time.Time) []recentsReadyItem {
	sm := t.machine()
	if sm == nil {
		return nil
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	ready := make([]recentsReadyItem, 0)
	for _, sessionID := range sm.RunningIDs() {
		turnID := ""
		if entry := meta[sessionID]; entry != nil {
			turnID = strings.TrimSpace(entry.LastTurnID)
		}
		transition := sm.Apply(RecentsEvent{
			Type:           RecentsEventMetaObserved,
			SessionID:      sessionID,
			ObservedTurnID: turnID,
			At:             observedAt,
		})
		if transition.ReadyEnqueued {
			ready = append(ready, transition.ReadyItem)
		}
	}
	return ready
}

func (t *RecentsTracker) CompleteRun(sessionID, expectedTurn, completionTurn string, completedAt time.Time) (recentsReadyItem, bool) {
	sm := t.machine()
	if sm == nil {
		return recentsReadyItem{}, false
	}
	transition := sm.Apply(RecentsEvent{
		Type:             RecentsEventRunCompleted,
		SessionID:        sessionID,
		ExpectedTurnID:   expectedTurn,
		CompletionTurnID: completionTurn,
		At:               completedAt,
	})
	return transition.ReadyItem, transition.ReadyEnqueued
}

func (t *RecentsTracker) ObserveSessions(sessions []*types.Session) {
	sm := t.machine()
	if sm == nil {
		return
	}
	present := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		id := strings.TrimSpace(session.ID)
		if id == "" {
			continue
		}
		present = append(present, id)
	}
	sm.Apply(RecentsEvent{
		Type:              RecentsEventSessionsPrune,
		PresentSessionIDs: present,
	})
}

func (t *RecentsTracker) DismissReady(sessionID string) bool {
	sm := t.machine()
	if sm == nil {
		return false
	}
	transition := sm.Apply(RecentsEvent{
		Type:      RecentsEventReadyDismiss,
		SessionID: sessionID,
	})
	return transition.Changed
}

func (t *RecentsTracker) RunningIDs() []string {
	sm := t.machine()
	if sm == nil {
		return nil
	}
	return sm.RunningIDs()
}

func (t *RecentsTracker) ReadyIDs() []string {
	sm := t.machine()
	if sm == nil {
		return nil
	}
	return sm.ReadyIDs()
}

func (t *RecentsTracker) ReadyItem(sessionID string) (recentsReadyItem, bool) {
	sm := t.machine()
	if sm == nil {
		return recentsReadyItem{}, false
	}
	return sm.ReadyItem(sessionID)
}

func (t *RecentsTracker) IsReady(sessionID string) bool {
	sm := t.machine()
	if sm == nil {
		return false
	}
	return sm.IsReady(sessionID)
}

func (t *RecentsTracker) IsRunning(sessionID string) bool {
	sm := t.machine()
	if sm == nil {
		return false
	}
	return sm.IsRunning(sessionID)
}

func (t *RecentsTracker) ReadyCount() int {
	sm := t.machine()
	if sm == nil {
		return 0
	}
	return sm.ReadyCount()
}

func (t *RecentsTracker) RunningCount() int {
	sm := t.machine()
	if sm == nil {
		return 0
	}
	return sm.RunningCount()
}
