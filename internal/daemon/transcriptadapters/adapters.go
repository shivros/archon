package transcriptadapters

import (
	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
	"control/internal/types"
)

// MappingContext carries provider-agnostic metadata for native->canonical mapping.
type MappingContext struct {
	SessionID    string
	Revision     transcriptdomain.RevisionToken
	ActiveTurnID string
}

// ProviderAdapter identifies the provider an adapter serves.
type ProviderAdapter interface {
	Provider() string
}

// TranscriptEventAdapter maps provider-native event transport to canonical events.
type TranscriptEventAdapter interface {
	ProviderAdapter
	MapEvent(ctx MappingContext, event types.CodexEvent) []transcriptdomain.TranscriptEvent
}

// TranscriptItemAdapter maps provider-native item transport to canonical events.
type TranscriptItemAdapter interface {
	ProviderAdapter
	MapItem(ctx MappingContext, item map[string]any) []transcriptdomain.TranscriptEvent
}

type ProviderTranscriptAdapterRegistry struct {
	eventAdapters map[string]TranscriptEventAdapter
	itemAdapters  map[string]TranscriptItemAdapter
}

type ProviderAdapterBundle struct {
	Event TranscriptEventAdapter
	Item  TranscriptItemAdapter
}

type RuntimeAdapterFactory func(providerName string) ProviderAdapterBundle

func NewDefaultProviderTranscriptAdapterRegistry() *ProviderTranscriptAdapterRegistry {
	factories := map[providers.Runtime]RuntimeAdapterFactory{
		providers.RuntimeCodex: func(providerName string) ProviderAdapterBundle {
			adapter := NewCodexTranscriptAdapter(providerName)
			return ProviderAdapterBundle{Event: adapter, Item: adapter}
		},
		providers.RuntimeClaude: func(providerName string) ProviderAdapterBundle {
			adapter := NewClaudeTranscriptAdapter(providerName)
			return ProviderAdapterBundle{Item: adapter}
		},
		providers.RuntimeOpenCodeServer: func(providerName string) ProviderAdapterBundle {
			adapter := NewOpenCodeTranscriptAdapter(providerName)
			return ProviderAdapterBundle{Event: adapter, Item: adapter}
		},
	}
	return BuildProviderTranscriptAdapterRegistry(providers.All(), factories)
}

func BuildProviderTranscriptAdapterRegistry(
	defs []providers.Definition,
	factories map[providers.Runtime]RuntimeAdapterFactory,
) *ProviderTranscriptAdapterRegistry {
	registry := &ProviderTranscriptAdapterRegistry{
		eventAdapters: map[string]TranscriptEventAdapter{},
		itemAdapters:  map[string]TranscriptItemAdapter{},
	}
	for _, def := range defs {
		providerName := providers.Normalize(def.Name)
		if providerName == "" {
			continue
		}
		factory, ok := factories[def.Runtime]
		if !ok || factory == nil {
			continue
		}
		bundle := factory(providerName)
		if bundle.Event != nil {
			registry.eventAdapters[providerName] = bundle.Event
		}
		if bundle.Item != nil {
			registry.itemAdapters[providerName] = bundle.Item
		}
	}
	return registry
}

func NewProviderTranscriptAdapterRegistry(adapters ...ProviderAdapter) *ProviderTranscriptAdapterRegistry {
	registry := &ProviderTranscriptAdapterRegistry{
		eventAdapters: map[string]TranscriptEventAdapter{},
		itemAdapters:  map[string]TranscriptItemAdapter{},
	}
	for _, adapter := range adapters {
		registry.register(adapter)
	}
	return registry
}

func (r *ProviderTranscriptAdapterRegistry) register(adapter ProviderAdapter) {
	if r == nil || adapter == nil {
		return
	}
	provider := providers.Normalize(adapter.Provider())
	if provider == "" {
		return
	}
	if eventAdapter, ok := adapter.(TranscriptEventAdapter); ok {
		r.eventAdapters[provider] = eventAdapter
	}
	if itemAdapter, ok := adapter.(TranscriptItemAdapter); ok {
		r.itemAdapters[provider] = itemAdapter
	}
}

func (r *ProviderTranscriptAdapterRegistry) EventAdapterFor(provider string) (TranscriptEventAdapter, bool) {
	if r == nil {
		return nil, false
	}
	adapter, ok := r.eventAdapters[providers.Normalize(provider)]
	if !ok || adapter == nil {
		return nil, false
	}
	return adapter, true
}

func (r *ProviderTranscriptAdapterRegistry) ItemAdapterFor(provider string) (TranscriptItemAdapter, bool) {
	if r == nil {
		return nil, false
	}
	adapter, ok := r.itemAdapters[providers.Normalize(provider)]
	if !ok || adapter == nil {
		return nil, false
	}
	return adapter, true
}
