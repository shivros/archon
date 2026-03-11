package app

import (
	"strings"
	"time"
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

type TranscriptRecoveryRequest struct {
	SessionID string
	Provider  string
}

type TranscriptRecoveryPlan struct {
	FetchTranscriptSnapshot bool
	FetchHistory            bool
	FetchApprovals          bool
	SnapshotSource          TranscriptAttachmentSource
	AuthoritativeSnapshot   bool
}

type TranscriptRecoveryScheduler interface {
	Plan(request TranscriptRecoveryRequest) TranscriptRecoveryPlan
}

type defaultTranscriptRecoveryScheduler struct{}

func (defaultTranscriptRecoveryScheduler) Plan(request TranscriptRecoveryRequest) TranscriptRecoveryPlan {
	if strings.TrimSpace(request.SessionID) == "" {
		return TranscriptRecoveryPlan{}
	}
	return TranscriptRecoveryPlan{
		FetchTranscriptSnapshot: true,
		FetchHistory:            true,
		FetchApprovals:          true,
		SnapshotSource:          transcriptAttachmentSourceRecovery,
		AuthoritativeSnapshot:   true,
	}
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
