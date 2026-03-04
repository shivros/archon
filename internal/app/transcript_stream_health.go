package app

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type transcriptSignalSummary struct {
	Total   int
	Content int
	Control int
}

type TranscriptSignalClassifier interface {
	Summarize(provider string, signals TranscriptTickSignals) transcriptSignalSummary
}

type defaultTranscriptSignalClassifier struct{}

func (defaultTranscriptSignalClassifier) Summarize(_ string, signals TranscriptTickSignals) transcriptSignalSummary {
	total := signals.Events
	content := signals.ContentEvents
	control := signals.ControlEvents
	if content < 0 {
		content = 0
	}
	if control < 0 {
		control = 0
	}
	if total <= 0 {
		total = content + control
	}
	if control == 0 && total > content {
		control = total - content
	}
	return transcriptSignalSummary{
		Total:   total,
		Content: content,
		Control: control,
	}
}

type StreamHealthObservation struct {
	SessionID            string
	Provider             string
	Now                  time.Time
	LastVisibleAt        time.Time
	RequestActivityAlive bool
	Signals              transcriptSignalSummary
}

type StreamHealthPolicy interface {
	ShouldRecover(observation StreamHealthObservation) bool
}

type defaultStreamHealthPolicy struct {
	minControlOnlyEvents int
	maxNoContentWindow   time.Duration
}

func (p defaultStreamHealthPolicy) ShouldRecover(observation StreamHealthObservation) bool {
	if !observation.RequestActivityAlive {
		return false
	}
	if strings.TrimSpace(observation.SessionID) == "" {
		return false
	}
	if strings.ToLower(strings.TrimSpace(observation.Provider)) != "codex" {
		return false
	}
	if observation.Signals.Total == 0 || observation.Signals.Content > 0 || observation.Signals.Control == 0 {
		return false
	}
	minEvents := p.minControlOnlyEvents
	if minEvents <= 0 {
		minEvents = 6
	}
	if observation.Signals.Control < minEvents {
		return false
	}
	window := p.maxNoContentWindow
	if window <= 0 {
		window = 2 * time.Second
	}
	lastVisible := observation.LastVisibleAt
	if lastVisible.IsZero() {
		lastVisible = observation.Now
	}
	return observation.Now.Sub(lastVisible) >= window
}

type TranscriptRecoveryScheduler interface {
	Schedule(m *Model, sessionID, provider string) tea.Cmd
}

type defaultTranscriptRecoveryScheduler struct{}

func (defaultTranscriptRecoveryScheduler) Schedule(m *Model, sessionID, provider string) tea.Cmd {
	if m == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	cmds := []tea.Cmd{}
	key := strings.TrimSpace(m.pendingSessionKey)
	if key == "" {
		key = strings.TrimSpace(m.selectedKey())
	}
	ctx := m.requestScopeContext(requestScopeSessionLoad)
	if m.sessionTranscriptAPI != nil {
		m.recordReconnectAttempt(sessionID, provider, "transcript", transcriptSourceAutoRefreshHistory)
		cmds = append(cmds, openTranscriptStreamCmd(m.sessionTranscriptAPI, sessionID, m.activeTranscriptRevision()))
		cmds = append(cmds, fetchTranscriptSnapshotCmdWithContext(m.sessionTranscriptAPI, sessionID, key, m.historyFetchLinesInitial(), ctx))
	} else if m.sessionHistoryAPI != nil {
		cmds = append(cmds, fetchHistoryCmdWithContext(m.sessionHistoryAPI, sessionID, key, m.historyFetchLinesInitial(), ctx))
	}
	if decision := m.approvalRefreshDecision(sessionID, provider, transcriptSourceAutoRefreshHistory); decision.ShouldFetch {
		cmds = append(cmds, fetchApprovalsCmdWithContext(m.sessionAPI, sessionID, ctx))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) transcriptSignalClassifierOrDefault() TranscriptSignalClassifier {
	if m == nil || m.transcriptSignalClassifier == nil {
		return defaultTranscriptSignalClassifier{}
	}
	return m.transcriptSignalClassifier
}

func (m *Model) streamHealthPolicyOrDefault() StreamHealthPolicy {
	if m == nil || m.streamHealthPolicy == nil {
		return defaultStreamHealthPolicy{}
	}
	return m.streamHealthPolicy
}

func (m *Model) transcriptRecoverySchedulerOrDefault() TranscriptRecoveryScheduler {
	if m == nil || m.transcriptRecoveryScheduler == nil {
		return defaultTranscriptRecoveryScheduler{}
	}
	return m.transcriptRecoveryScheduler
}

func WithTranscriptSignalClassifier(classifier TranscriptSignalClassifier) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.transcriptSignalClassifier = classifier
	}
}

func WithStreamHealthPolicy(policy StreamHealthPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.streamHealthPolicy = policy
	}
}

func WithTranscriptRecoveryScheduler(scheduler TranscriptRecoveryScheduler) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.transcriptRecoveryScheduler = scheduler
	}
}
