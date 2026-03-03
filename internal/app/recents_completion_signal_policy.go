package app

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
)

// RecentsCompletionSignalPolicy decides whether a transcript event should
// complete a recents run and which turn ID should be applied.
type RecentsCompletionSignalPolicy interface {
	CompletionFromTranscriptEvent(event transcriptdomain.TranscriptEvent) (turnID string, matched bool)
}

type transcriptEventRecentsCompletionSignalPolicy struct{}

func WithRecentsCompletionSignalPolicy(policy RecentsCompletionSignalPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.recentsCompletionSignalPolicy = transcriptEventRecentsCompletionSignalPolicy{}
			return
		}
		m.recentsCompletionSignalPolicy = policy
	}
}

func (transcriptEventRecentsCompletionSignalPolicy) CompletionFromTranscriptEvent(event transcriptdomain.TranscriptEvent) (string, bool) {
	switch event.Kind {
	case transcriptdomain.TranscriptEventTurnCompleted, transcriptdomain.TranscriptEventTurnFailed:
		if event.Turn == nil {
			return "", true
		}
		return strings.TrimSpace(event.Turn.TurnID), true
	default:
		return "", false
	}
}

func (m *Model) recentsCompletionSignalPolicyOrDefault() RecentsCompletionSignalPolicy {
	if m == nil || m.recentsCompletionSignalPolicy == nil {
		return transcriptEventRecentsCompletionSignalPolicy{}
	}
	return m.recentsCompletionSignalPolicy
}
