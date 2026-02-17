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

type RecentsTracker struct {
	running       map[string]recentsRun
	ready         map[string]recentsReadyItem
	readyOrder    []string
	dismissedTurn map[string]string
}

const recentsUnknownCompletionTurn = "__unknown_completion__"

func NewRecentsTracker() *RecentsTracker {
	return &RecentsTracker{
		running:       map[string]recentsRun{},
		ready:         map[string]recentsReadyItem{},
		readyOrder:    []string{},
		dismissedTurn: map[string]string{},
	}
}

func (t *RecentsTracker) StartRun(sessionID, baselineTurnID string, startedAt time.Time) {
	if t == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	t.removeReady(sessionID)
	delete(t.dismissedTurn, sessionID)
	t.running[sessionID] = recentsRun{
		SessionID:      sessionID,
		BaselineTurnID: strings.TrimSpace(baselineTurnID),
		StartedAt:      startedAt.UTC(),
	}
}

func (t *RecentsTracker) CancelRun(sessionID string) {
	if t == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	delete(t.running, sessionID)
}

func (t *RecentsTracker) ObserveMeta(meta map[string]*types.SessionMeta, observedAt time.Time) []recentsReadyItem {
	if t == nil {
		return nil
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	observedAt = observedAt.UTC()
	ids := t.RunningIDs()
	if len(ids) == 0 {
		return nil
	}
	ready := make([]recentsReadyItem, 0, len(ids))
	for _, sessionID := range ids {
		run, ok := t.running[sessionID]
		if !ok {
			continue
		}
		turnID := ""
		if entry := meta[sessionID]; entry != nil {
			turnID = strings.TrimSpace(entry.LastTurnID)
		}
		if turnID == "" || turnID == strings.TrimSpace(run.BaselineTurnID) {
			continue
		}
		item, enqueued := t.CompleteRun(sessionID, "", turnID, observedAt)
		if enqueued {
			ready = append(ready, item)
		}
	}
	return ready
}

func (t *RecentsTracker) CompleteRun(sessionID, expectedTurn, completionTurn string, completedAt time.Time) (recentsReadyItem, bool) {
	if t == nil {
		return recentsReadyItem{}, false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return recentsReadyItem{}, false
	}
	run, ok := t.running[sessionID]
	if !ok {
		return recentsReadyItem{}, false
	}
	expectedTurn = strings.TrimSpace(expectedTurn)
	if expectedTurn != "" && strings.TrimSpace(run.BaselineTurnID) != expectedTurn {
		return recentsReadyItem{}, false
	}
	completionTurn = strings.TrimSpace(completionTurn)
	if completionTurn == "" {
		completionTurn = strings.TrimSpace(run.BaselineTurnID)
	}
	if completionTurn == "" {
		completionTurn = recentsUnknownCompletionTurn
	}
	delete(t.running, sessionID)
	return t.enqueueReady(sessionID, completionTurn, completedAt)
}

func (t *RecentsTracker) ObserveSessions(sessions []*types.Session) {
	if t == nil {
		return
	}
	if len(sessions) == 0 {
		t.running = map[string]recentsRun{}
		t.ready = map[string]recentsReadyItem{}
		t.readyOrder = nil
		t.dismissedTurn = map[string]string{}
		return
	}
	present := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		id := strings.TrimSpace(session.ID)
		if id == "" {
			continue
		}
		present[id] = struct{}{}
	}
	for id := range t.running {
		if _, ok := present[id]; !ok {
			delete(t.running, id)
		}
	}
	for id := range t.ready {
		if _, ok := present[id]; !ok {
			delete(t.ready, id)
		}
	}
	for id := range t.dismissedTurn {
		if _, ok := present[id]; !ok {
			delete(t.dismissedTurn, id)
		}
	}
	filtered := make([]string, 0, len(t.readyOrder))
	for _, id := range t.readyOrder {
		if _, ok := t.ready[id]; ok {
			filtered = append(filtered, id)
		}
	}
	t.readyOrder = filtered
}

func (t *RecentsTracker) DismissReady(sessionID string) bool {
	if t == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	item, ok := t.ready[sessionID]
	if !ok {
		return false
	}
	t.dismissedTurn[sessionID] = strings.TrimSpace(item.CompletionTurn)
	t.removeReady(sessionID)
	return true
}

func (t *RecentsTracker) RunningIDs() []string {
	if t == nil || len(t.running) == 0 {
		return nil
	}
	runs := make([]recentsRun, 0, len(t.running))
	for _, run := range t.running {
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

func (t *RecentsTracker) ReadyIDs() []string {
	if t == nil || len(t.readyOrder) == 0 {
		return nil
	}
	ids := make([]string, 0, len(t.readyOrder))
	for _, id := range t.readyOrder {
		if _, ok := t.ready[id]; ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func (t *RecentsTracker) ReadyItem(sessionID string) (recentsReadyItem, bool) {
	if t == nil {
		return recentsReadyItem{}, false
	}
	item, ok := t.ready[strings.TrimSpace(sessionID)]
	if !ok {
		return recentsReadyItem{}, false
	}
	return item, true
}

func (t *RecentsTracker) IsReady(sessionID string) bool {
	if t == nil {
		return false
	}
	_, ok := t.ready[strings.TrimSpace(sessionID)]
	return ok
}

func (t *RecentsTracker) IsRunning(sessionID string) bool {
	if t == nil {
		return false
	}
	_, ok := t.running[strings.TrimSpace(sessionID)]
	return ok
}

func (t *RecentsTracker) ReadyCount() int {
	if t == nil {
		return 0
	}
	return len(t.ready)
}

func (t *RecentsTracker) RunningCount() int {
	if t == nil {
		return 0
	}
	return len(t.running)
}

func (t *RecentsTracker) enqueueReady(sessionID, completionTurn string, completedAt time.Time) (recentsReadyItem, bool) {
	if t == nil {
		return recentsReadyItem{}, false
	}
	sessionID = strings.TrimSpace(sessionID)
	completionTurn = strings.TrimSpace(completionTurn)
	if sessionID == "" || completionTurn == "" {
		return recentsReadyItem{}, false
	}
	if dismissed, ok := t.dismissedTurn[sessionID]; ok && strings.TrimSpace(dismissed) == completionTurn {
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
	if existing, ok := t.ready[sessionID]; ok {
		if strings.TrimSpace(existing.CompletionTurn) == completionTurn {
			return existing, false
		}
		t.removeReady(sessionID)
	}
	t.ready[sessionID] = item
	t.readyOrder = append(t.readyOrder, sessionID)
	return item, true
}

func (t *RecentsTracker) removeReady(sessionID string) {
	if t == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	delete(t.ready, sessionID)
	if len(t.readyOrder) == 0 {
		return
	}
	filtered := t.readyOrder[:0]
	for _, id := range t.readyOrder {
		if id == sessionID {
			continue
		}
		filtered = append(filtered, id)
	}
	t.readyOrder = filtered
}
