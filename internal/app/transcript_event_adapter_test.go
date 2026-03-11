package app

import (
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestDefaultTranscriptEventAdapterNormalizesDeltaAndFinalizedCategories(t *testing.T) {
	registry := NewDefaultTranscriptEventAdapterRegistry()
	adapter := registry.AdapterForProvider("codex")

	delta := adapter.Normalize(transcriptdomain.TranscriptEvent{
		Kind: transcriptdomain.TranscriptEventDelta,
		Delta: []transcriptdomain.Block{
			{Kind: "assistant_delta", Role: "assistant", Text: "part"},
		},
	})
	if delta.Category != transcriptEventCategoryDelta {
		t.Fatalf("expected delta category, got %#v", delta)
	}
	if len(delta.FinalizedDeltaBlockIndexes) != 0 {
		t.Fatalf("expected no finalized delta indexes, got %#v", delta.FinalizedDeltaBlockIndexes)
	}

	finalized := adapter.Normalize(transcriptdomain.TranscriptEvent{
		Kind: transcriptdomain.TranscriptEventDelta,
		Delta: []transcriptdomain.Block{
			{Kind: "assistant_message", Role: "assistant", Text: "full answer"},
		},
	})
	if finalized.Category != transcriptEventCategoryFinalizedMessage {
		t.Fatalf("expected finalized category, got %#v", finalized)
	}
	if len(finalized.FinalizedDeltaBlockIndexes) != 1 {
		t.Fatalf("expected one finalized index, got %#v", finalized.FinalizedDeltaBlockIndexes)
	}
}

func TestDefaultTranscriptEventAdapterTreatsMetaFinalAsFinalized(t *testing.T) {
	registry := NewDefaultTranscriptEventAdapterRegistry()
	adapter := registry.AdapterForProvider("codex")

	finalized := adapter.Normalize(transcriptdomain.TranscriptEvent{
		Kind: transcriptdomain.TranscriptEventDelta,
		Delta: []transcriptdomain.Block{
			{
				Kind: "assistant_chunk",
				Role: "assistant",
				Text: "done",
				Meta: map[string]any{"final": true},
			},
		},
	})
	if finalized.Category != transcriptEventCategoryFinalizedMessage {
		t.Fatalf("expected finalized category from meta.final, got %#v", finalized)
	}
	if len(finalized.FinalizedDeltaBlockIndexes) != 1 {
		t.Fatalf("expected finalized index from meta.final, got %#v", finalized.FinalizedDeltaBlockIndexes)
	}
}

func TestTranscriptEventAdapterRegistryProviderFallbacks(t *testing.T) {
	registry := defaultTranscriptEventAdapterRegistry{
		byProvider: map[string]TranscriptEventAdapter{
			"codex": newDefaultTranscriptEventAdapter(defaultTranscriptBlockKindClassifier{}),
		},
	}
	if adapter := registry.AdapterForProvider(" codex "); adapter == nil {
		t.Fatalf("expected registered provider adapter")
	}
	if adapter := registry.AdapterForProvider("unknown"); adapter == nil {
		t.Fatalf("expected unknown provider to return non-nil fallback adapter")
	}
}

func TestNewDefaultTranscriptEventAdapterNilClassifierFallsBack(t *testing.T) {
	adapter := newDefaultTranscriptEventAdapter(nil)
	normalized := adapter.Normalize(transcriptdomain.TranscriptEvent{
		Kind: transcriptdomain.TranscriptEventDelta,
		Delta: []transcriptdomain.Block{
			{Kind: "assistant_message", Role: "assistant", Text: "final"},
		},
	})
	if normalized.Category != transcriptEventCategoryFinalizedMessage {
		t.Fatalf("expected nil classifier fallback to classify finalized message, got %#v", normalized)
	}
}
