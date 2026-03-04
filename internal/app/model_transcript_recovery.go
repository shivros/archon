package app

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type transcriptStreamHealthState struct {
	controlOnlyBatches int
	controlOnlyEvents  int
	lastRecoveryAt     time.Time
}

func (m *Model) clearTranscriptHealthState(sessionID string) {
	if m == nil || m.transcriptHealthBySession == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	delete(m.transcriptHealthBySession, sessionID)
}

func (m *Model) maybeRecoverTranscriptFromControlOnlySignals(
	now time.Time,
	sessionID string,
	provider string,
	signals TranscriptTickSignals,
) tea.Cmd {
	if m == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	if m.transcriptHealthBySession == nil {
		m.transcriptHealthBySession = map[string]transcriptStreamHealthState{}
	}
	summary := m.transcriptSignalClassifierOrDefault().Summarize(provider, signals)
	if summary.Total == 0 {
		return nil
	}
	state := m.transcriptHealthBySession[sessionID]
	if summary.Content > 0 {
		state.controlOnlyBatches = 0
		state.controlOnlyEvents = 0
		m.transcriptHealthBySession[sessionID] = state
		return nil
	}
	if summary.Control == 0 {
		return nil
	}
	state.controlOnlyBatches++
	state.controlOnlyEvents += summary.Control
	m.transcriptHealthBySession[sessionID] = state

	lastVisible := m.requestActivity.lastVisibleAt
	requestActive := m.requestActivity.active && strings.TrimSpace(m.requestActivity.sessionID) == sessionID
	observation := StreamHealthObservation{
		SessionID:            sessionID,
		Provider:             provider,
		Now:                  now,
		LastVisibleAt:        lastVisible,
		RequestActivityAlive: requestActive,
		Signals: transcriptSignalSummary{
			Total:   summary.Total,
			Content: summary.Content,
			Control: max(summary.Control, state.controlOnlyEvents),
		},
	}
	if !m.streamHealthPolicyOrDefault().ShouldRecover(observation) {
		return nil
	}
	if state.controlOnlyBatches < 2 {
		return nil
	}
	if !state.lastRecoveryAt.IsZero() && now.Sub(state.lastRecoveryAt) < requestRefreshCooldown {
		return nil
	}
	state.lastRecoveryAt = now
	state.controlOnlyBatches = 0
	state.controlOnlyEvents = 0
	m.transcriptHealthBySession[sessionID] = state

	if m.requestActivity.active && strings.TrimSpace(m.requestActivity.sessionID) == sessionID {
		m.requestActivity.lastHistoryRefreshAt = now
		m.requestActivity.refreshCount++
	}
	m.setBackgroundStatus("transcript stream stale; recovering")
	return m.transcriptRecoverySchedulerOrDefault().Schedule(m, sessionID, provider)
}
