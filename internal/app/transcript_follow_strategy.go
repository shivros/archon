package app

import "strings"

type TranscriptFollowOpenRequest struct {
	SessionID     string
	Source        TranscriptAttachmentSource
	AfterRevision string
}

type TranscriptFollowOpenDecision struct {
	Open            bool
	ReconnectSource string
}

type TranscriptFollowStrategy interface {
	Decide(request TranscriptFollowOpenRequest) TranscriptFollowOpenDecision
}

type TranscriptFollowStrategyRegistry interface {
	StrategyFor(source TranscriptAttachmentSource) TranscriptFollowStrategy
}

type staticTranscriptFollowStrategy struct {
	open            bool
	reconnectSource string
}

func (s staticTranscriptFollowStrategy) Decide(_ TranscriptFollowOpenRequest) TranscriptFollowOpenDecision {
	return TranscriptFollowOpenDecision{
		Open:            s.open,
		ReconnectSource: strings.TrimSpace(s.reconnectSource),
	}
}

type defaultTranscriptFollowStrategyRegistry struct {
	bySource map[TranscriptAttachmentSource]TranscriptFollowStrategy
	fallback TranscriptFollowStrategy
}

func NewDefaultTranscriptFollowStrategyRegistry() TranscriptFollowStrategyRegistry {
	return defaultTranscriptFollowStrategyRegistry{
		bySource: map[TranscriptAttachmentSource]TranscriptFollowStrategy{
			transcriptAttachmentSourceSelectionLoad: staticTranscriptFollowStrategy{
				open:            true,
				reconnectSource: transcriptSourceSessionBlocksProject,
			},
			transcriptAttachmentSourceSessionStart: staticTranscriptFollowStrategy{
				open:            true,
				reconnectSource: transcriptSourceSessionBlocksProject,
			},
			transcriptAttachmentSourceRecovery: staticTranscriptFollowStrategy{
				open:            true,
				reconnectSource: transcriptSourceAutoRefreshHistory,
			},
		},
		fallback: staticTranscriptFollowStrategy{},
	}
}

func (r defaultTranscriptFollowStrategyRegistry) StrategyFor(source TranscriptAttachmentSource) TranscriptFollowStrategy {
	source = normalizeTranscriptAttachmentSource(source)
	if strategy, ok := r.bySource[source]; ok && strategy != nil {
		return strategy
	}
	if r.fallback == nil {
		return staticTranscriptFollowStrategy{}
	}
	return r.fallback
}

func (m *Model) transcriptFollowStrategyRegistryOrDefault() TranscriptFollowStrategyRegistry {
	if m == nil || m.transcriptFollowStrategyRegistry == nil {
		return NewDefaultTranscriptFollowStrategyRegistry()
	}
	return m.transcriptFollowStrategyRegistry
}

func WithTranscriptFollowStrategyRegistry(registry TranscriptFollowStrategyRegistry) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if registry == nil {
			m.transcriptFollowStrategyRegistry = NewDefaultTranscriptFollowStrategyRegistry()
			return
		}
		m.transcriptFollowStrategyRegistry = registry
	}
}
