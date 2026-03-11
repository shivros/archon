package app

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
)

type TranscriptNormalizedEvent struct {
	Event                      transcriptdomain.TranscriptEvent
	Category                   TranscriptEventCategory
	FinalizedDeltaBlockIndexes map[int]struct{}
}

type TranscriptEventAdapter interface {
	Normalize(event transcriptdomain.TranscriptEvent) TranscriptNormalizedEvent
}

type TranscriptEventAdapterRegistry interface {
	AdapterForProvider(provider string) TranscriptEventAdapter
}

type defaultTranscriptEventAdapterRegistry struct {
	byProvider map[string]TranscriptEventAdapter
	fallback   TranscriptEventAdapter
}

func NewDefaultTranscriptEventAdapterRegistry() TranscriptEventAdapterRegistry {
	defaultAdapter := newDefaultTranscriptEventAdapter(defaultTranscriptBlockKindClassifier{})
	return defaultTranscriptEventAdapterRegistry{
		byProvider: map[string]TranscriptEventAdapter{},
		fallback:   defaultAdapter,
	}
}

func (r defaultTranscriptEventAdapterRegistry) AdapterForProvider(provider string) TranscriptEventAdapter {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if adapter, ok := r.byProvider[provider]; ok && adapter != nil {
		return adapter
	}
	if r.fallback == nil {
		return newDefaultTranscriptEventAdapter(defaultTranscriptBlockKindClassifier{})
	}
	return r.fallback
}

type transcriptDeltaBlockKindClassifier interface {
	IsFinalized(block transcriptdomain.Block) bool
}

type defaultTranscriptBlockKindClassifier struct{}

func (defaultTranscriptBlockKindClassifier) IsFinalized(block transcriptdomain.Block) bool {
	if block.Meta != nil {
		switch typed := block.Meta["final"].(type) {
		case bool:
			if typed {
				return true
			}
		case string:
			if strings.EqualFold(strings.TrimSpace(typed), "true") {
				return true
			}
		}
	}
	kind := strings.ToLower(strings.TrimSpace(block.Kind))
	if kind == "" {
		return false
	}
	switch kind {
	case "assistant_message",
		"agent_message",
		"reasoning_message",
		"user_message",
		"message",
		"assistant_response",
		"model_response",
		"turn_output":
		return true
	}
	if strings.HasSuffix(kind, ".message") || strings.HasSuffix(kind, "_message") {
		return true
	}
	return false
}

type defaultTranscriptEventAdapter struct {
	classifier transcriptDeltaBlockKindClassifier
}

func newDefaultTranscriptEventAdapter(classifier transcriptDeltaBlockKindClassifier) TranscriptEventAdapter {
	if classifier == nil {
		classifier = defaultTranscriptBlockKindClassifier{}
	}
	return defaultTranscriptEventAdapter{classifier: classifier}
}

func (a defaultTranscriptEventAdapter) Normalize(event transcriptdomain.TranscriptEvent) TranscriptNormalizedEvent {
	normalized := TranscriptNormalizedEvent{
		Event:    event,
		Category: transcriptEventCategoryControl,
	}
	switch event.Kind {
	case transcriptdomain.TranscriptEventReplace:
		normalized.Category = transcriptEventCategorySnapshotReplace
	case transcriptdomain.TranscriptEventDelta:
		normalized.Category = transcriptEventCategoryDelta
		finalized := map[int]struct{}{}
		for i, block := range event.Delta {
			if a.classifier.IsFinalized(block) {
				finalized[i] = struct{}{}
			}
		}
		if len(finalized) > 0 {
			normalized.Category = transcriptEventCategoryFinalizedMessage
			normalized.FinalizedDeltaBlockIndexes = finalized
		}
	}
	return normalized
}
