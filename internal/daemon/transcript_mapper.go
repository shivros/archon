package daemon

import (
	"strings"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
	"control/internal/types"
)

type TranscriptMapper interface {
	MapItem(provider string, ctx transcriptadapters.MappingContext, item map[string]any) []transcriptdomain.TranscriptEvent
	MapEvent(provider string, ctx transcriptadapters.MappingContext, event types.CodexEvent) []transcriptdomain.TranscriptEvent
}

type defaultTranscriptMapper struct {
	registry *transcriptadapters.ProviderTranscriptAdapterRegistry
}

func NewDefaultTranscriptMapper(registry *transcriptadapters.ProviderTranscriptAdapterRegistry) TranscriptMapper {
	if registry == nil {
		registry = transcriptadapters.NewDefaultProviderTranscriptAdapterRegistry()
	}
	return &defaultTranscriptMapper{registry: registry}
}

func (m *defaultTranscriptMapper) MapItem(
	provider string,
	ctx transcriptadapters.MappingContext,
	item map[string]any,
) []transcriptdomain.TranscriptEvent {
	if m == nil || m.registry == nil {
		return nil
	}
	provider = normalizeTranscriptProvider(provider)
	if adapter, ok := m.registry.ItemAdapterFor(provider); ok {
		return adapter.MapItem(ctx, item)
	}
	event, ok := transcriptadapters.DeltaEventFromItem(ctx.SessionID, provider, ctx.Revision, item)
	if !ok {
		return nil
	}
	if err := transcriptdomain.ValidateEvent(event); err != nil {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{event}
}

func (m *defaultTranscriptMapper) MapEvent(
	provider string,
	ctx transcriptadapters.MappingContext,
	event types.CodexEvent,
) []transcriptdomain.TranscriptEvent {
	if m == nil || m.registry == nil {
		return nil
	}
	provider = normalizeTranscriptProvider(provider)
	adapter, ok := m.registry.EventAdapterFor(provider)
	if !ok {
		// Explicitly unsupported instead of codex-biased fallback mapping.
		return nil
	}
	return adapter.MapEvent(ctx, event)
}

func normalizeTranscriptProvider(provider string) string {
	normalized := providers.Normalize(provider)
	if normalized != "" {
		return normalized
	}
	return strings.TrimSpace(provider)
}
