package app

import (
	"reflect"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

const recentsAppStateVersion = 1

func (m *Model) syncAppStateRecents() bool {
	if m == nil {
		return false
	}
	next := appStateRecentsFromSnapshot(recentsSnapshotFromDomain(m.recents))
	if appStateRecentsEqual(m.appState.Recents, next) {
		return false
	}
	m.appState.Recents = next
	return true
}

func (m *Model) requestRecentsStateSaveCmd() tea.Cmd {
	if m == nil || m.recents == nil {
		return nil
	}
	if !m.syncAppStateRecents() {
		return nil
	}
	m.hasAppState = true
	return m.requestAppStateSaveCmd()
}

func (m *Model) restoreRecentsFromAppState(state *types.AppState) {
	if m == nil || m.recents == nil || state == nil {
		return
	}
	m.recents.Restore(recentsSnapshotFromAppState(state.Recents))
	m.syncRecentsCompletionWatches()
	if m.sidebar != nil {
		m.refreshRecentsSidebarState()
	}
	if m.mode == uiModeRecents {
		m.refreshRecentsContent()
	}
}

func recentsSnapshotFromDomain(domain recentsDomain) RecentsSnapshot {
	if domain == nil {
		return RecentsSnapshot{}
	}
	return domain.Snapshot()
}

func appStateRecentsFromSnapshot(snapshot RecentsSnapshot) *types.AppStateRecents {
	running := map[string]types.AppStateRecentRun{}
	for key, run := range snapshot.Running {
		sessionID := normalizeRecentsSessionID(key, run.SessionID)
		if sessionID == "" {
			continue
		}
		running[sessionID] = types.AppStateRecentRun{
			SessionID:      sessionID,
			BaselineTurnID: strings.TrimSpace(run.BaselineTurnID),
			StartedAtUnix:  toUnixUTC(run.StartedAt),
		}
	}

	ready := map[string]types.AppStateReadyItem{}
	for key, item := range snapshot.Ready {
		sessionID := normalizeRecentsSessionID(key, item.SessionID)
		if sessionID == "" {
			continue
		}
		completionTurn := strings.TrimSpace(item.CompletionTurn)
		if completionTurn == "" {
			continue
		}
		ready[sessionID] = types.AppStateReadyItem{
			SessionID:       sessionID,
			CompletionTurn:  completionTurn,
			CompletedAtUnix: toUnixUTC(item.CompletedAt),
			LastKnownTurnID: strings.TrimSpace(item.LastKnownTurnID),
		}
	}

	readyQueue := make([]types.AppStateReadyQueueEntry, 0, len(snapshot.ReadyQueue))
	for _, entry := range snapshot.ReadyQueue {
		sessionID := strings.TrimSpace(entry.SessionID)
		if sessionID == "" {
			continue
		}
		readyQueue = append(readyQueue, types.AppStateReadyQueueEntry{
			SessionID: sessionID,
			Seq:       entry.Seq,
		})
	}

	dismissedTurn := map[string]string{}
	for key, turnID := range snapshot.DismissedTurn {
		sessionID := strings.TrimSpace(key)
		completionTurn := strings.TrimSpace(turnID)
		if sessionID == "" || completionTurn == "" {
			continue
		}
		dismissedTurn[sessionID] = completionTurn
	}

	if len(running) == 0 && len(ready) == 0 && len(readyQueue) == 0 && len(dismissedTurn) == 0 {
		return nil
	}

	return &types.AppStateRecents{
		Version:       recentsAppStateVersion,
		Running:       running,
		Ready:         ready,
		ReadyQueue:    readyQueue,
		DismissedTurn: dismissedTurn,
	}
}

func recentsSnapshotFromAppState(state *types.AppStateRecents) RecentsSnapshot {
	if state == nil {
		return RecentsSnapshot{}
	}

	running := map[string]recentsRun{}
	for key, run := range state.Running {
		sessionID := normalizeRecentsSessionID(key, run.SessionID)
		if sessionID == "" {
			continue
		}
		running[sessionID] = recentsRun{
			SessionID:      sessionID,
			BaselineTurnID: strings.TrimSpace(run.BaselineTurnID),
			StartedAt:      fromUnixUTC(run.StartedAtUnix),
		}
	}

	ready := map[string]recentsReadyItem{}
	for key, item := range state.Ready {
		sessionID := normalizeRecentsSessionID(key, item.SessionID)
		if sessionID == "" {
			continue
		}
		completionTurn := strings.TrimSpace(item.CompletionTurn)
		if completionTurn == "" {
			continue
		}
		ready[sessionID] = recentsReadyItem{
			SessionID:       sessionID,
			CompletionTurn:  completionTurn,
			CompletedAt:     fromUnixUTC(item.CompletedAtUnix),
			LastKnownTurnID: strings.TrimSpace(item.LastKnownTurnID),
		}
	}

	readyQueue := make([]recentsReadyQueueEntry, 0, len(state.ReadyQueue))
	for _, entry := range state.ReadyQueue {
		sessionID := strings.TrimSpace(entry.SessionID)
		if sessionID == "" {
			continue
		}
		readyQueue = append(readyQueue, recentsReadyQueueEntry{
			SessionID: sessionID,
			Seq:       entry.Seq,
		})
	}

	dismissedTurn := map[string]string{}
	for key, turnID := range state.DismissedTurn {
		sessionID := strings.TrimSpace(key)
		completionTurn := strings.TrimSpace(turnID)
		if sessionID == "" || completionTurn == "" {
			continue
		}
		dismissedTurn[sessionID] = completionTurn
	}

	return RecentsSnapshot{
		Running:       running,
		Ready:         ready,
		ReadyQueue:    readyQueue,
		DismissedTurn: dismissedTurn,
	}
}

func appStateRecentsEqual(left, right *types.AppStateRecents) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return reflect.DeepEqual(left, right)
}

func toUnixUTC(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().Unix()
}

func fromUnixUTC(unix int64) time.Time {
	if unix <= 0 {
		return time.Time{}
	}
	return time.Unix(unix, 0).UTC()
}
