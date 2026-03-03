package transcriptadapters

import (
	"testing"

	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
	"control/internal/types"
)

func TestProviderTranscriptAdapterRegistryDefaultsCapabilities(t *testing.T) {
	registry := NewDefaultProviderTranscriptAdapterRegistry()

	if _, ok := registry.EventAdapterFor("codex"); !ok {
		t.Fatal("expected codex event adapter")
	}
	if _, ok := registry.ItemAdapterFor("codex"); !ok {
		t.Fatal("expected codex item adapter")
	}

	if _, ok := registry.EventAdapterFor("claude"); ok {
		t.Fatal("did not expect claude event adapter")
	}
	if _, ok := registry.ItemAdapterFor("claude"); !ok {
		t.Fatal("expected claude item adapter")
	}

	if _, ok := registry.EventAdapterFor("opencode"); !ok {
		t.Fatal("expected opencode event adapter")
	}
	if _, ok := registry.ItemAdapterFor("opencode"); !ok {
		t.Fatal("expected opencode item adapter")
	}

	if _, ok := registry.EventAdapterFor("kilocode"); !ok {
		t.Fatal("expected kilocode event adapter")
	}
	if _, ok := registry.ItemAdapterFor("kilocode"); !ok {
		t.Fatal("expected kilocode item adapter")
	}
}

func TestProviderTranscriptAdapterRegistryUnknownIsExplicitlyUnsupported(t *testing.T) {
	registry := NewDefaultProviderTranscriptAdapterRegistry()
	if _, ok := registry.EventAdapterFor("unknown-provider"); ok {
		t.Fatal("expected unknown event adapter lookup to be unsupported")
	}
	if _, ok := registry.ItemAdapterFor("unknown-provider"); ok {
		t.Fatal("expected unknown item adapter lookup to be unsupported")
	}
}

func TestBuildProviderTranscriptAdapterRegistryUsesRuntimeFactories(t *testing.T) {
	defs := []providers.Definition{
		{Name: "p-codex", Runtime: providers.RuntimeCodex},
		{Name: "p-claude", Runtime: providers.RuntimeClaude},
		{Name: "p-open", Runtime: providers.RuntimeOpenCodeServer},
	}
	registry := BuildProviderTranscriptAdapterRegistry(defs, map[providers.Runtime]RuntimeAdapterFactory{
		providers.RuntimeCodex: func(provider string) ProviderAdapterBundle {
			adapter := NewCodexTranscriptAdapter(provider)
			return ProviderAdapterBundle{Event: adapter}
		},
		providers.RuntimeClaude: func(provider string) ProviderAdapterBundle {
			adapter := NewClaudeTranscriptAdapter(provider)
			return ProviderAdapterBundle{Item: adapter}
		},
		providers.RuntimeOpenCodeServer: func(provider string) ProviderAdapterBundle {
			adapter := NewOpenCodeTranscriptAdapter(provider)
			return ProviderAdapterBundle{Event: adapter, Item: adapter}
		},
	})

	if _, ok := registry.EventAdapterFor("p-codex"); !ok {
		t.Fatal("expected runtime-based codex event adapter")
	}
	if _, ok := registry.ItemAdapterFor("p-claude"); !ok {
		t.Fatal("expected runtime-based claude item adapter")
	}
	if _, ok := registry.EventAdapterFor("p-open"); !ok {
		t.Fatal("expected runtime-based opencode event adapter")
	}
	if _, ok := registry.ItemAdapterFor("p-open"); !ok {
		t.Fatal("expected runtime-based opencode item adapter")
	}
}

func TestNewProviderTranscriptAdapterRegistryRegistersCapabilities(t *testing.T) {
	registry := NewProviderTranscriptAdapterRegistry(
		NewCodexTranscriptAdapter("custom-codex"),
		NewClaudeTranscriptAdapter("custom-claude"),
		NewOpenCodeTranscriptAdapter("custom-open"),
		nil,
	)

	if _, ok := registry.EventAdapterFor("custom-codex"); !ok {
		t.Fatal("expected custom-codex event adapter")
	}
	if _, ok := registry.ItemAdapterFor("custom-codex"); !ok {
		t.Fatal("expected custom-codex item adapter")
	}
	if _, ok := registry.EventAdapterFor("custom-claude"); ok {
		t.Fatal("did not expect custom-claude event adapter")
	}
	if _, ok := registry.ItemAdapterFor("custom-claude"); !ok {
		t.Fatal("expected custom-claude item adapter")
	}
	if _, ok := registry.EventAdapterFor("custom-open"); !ok {
		t.Fatal("expected custom-open event adapter")
	}
	if _, ok := registry.ItemAdapterFor("custom-open"); !ok {
		t.Fatal("expected custom-open item adapter")
	}
}

func TestProviderTranscriptAdapterRegistryNilReceiverIsUnsupported(t *testing.T) {
	var registry *ProviderTranscriptAdapterRegistry
	if _, ok := registry.EventAdapterFor("codex"); ok {
		t.Fatal("expected nil registry event lookup to be unsupported")
	}
	if _, ok := registry.ItemAdapterFor("codex"); ok {
		t.Fatal("expected nil registry item lookup to be unsupported")
	}
}

func TestBuildProviderTranscriptAdapterRegistrySkipsUnknownRuntime(t *testing.T) {
	defs := []providers.Definition{
		{Name: "p-unknown", Runtime: providers.RuntimeExec},
	}
	registry := BuildProviderTranscriptAdapterRegistry(defs, map[providers.Runtime]RuntimeAdapterFactory{})
	if _, ok := registry.EventAdapterFor("p-unknown"); ok {
		t.Fatal("expected no event adapter for unknown runtime")
	}
	if _, ok := registry.ItemAdapterFor("p-unknown"); ok {
		t.Fatal("expected no item adapter for unknown runtime")
	}
}

func TestBuildProviderTranscriptAdapterRegistrySkipsEmptyProviderName(t *testing.T) {
	defs := []providers.Definition{
		{Name: " ", Runtime: providers.RuntimeCodex},
	}
	registry := BuildProviderTranscriptAdapterRegistry(defs, map[providers.Runtime]RuntimeAdapterFactory{
		providers.RuntimeCodex: func(provider string) ProviderAdapterBundle {
			adapter := NewCodexTranscriptAdapter(provider)
			return ProviderAdapterBundle{Event: adapter, Item: adapter}
		},
	})
	if _, ok := registry.EventAdapterFor(" "); ok {
		t.Fatal("expected no adapter for blank provider")
	}
}

type testEventOnlyAdapter struct{ provider string }

func (a testEventOnlyAdapter) Provider() string { return a.provider }
func (a testEventOnlyAdapter) MapEvent(MappingContext, types.CodexEvent) []transcriptdomain.TranscriptEvent {
	return nil
}

func TestNewProviderTranscriptAdapterRegistryRegistersEventOnlyAdapter(t *testing.T) {
	registry := NewProviderTranscriptAdapterRegistry(testEventOnlyAdapter{provider: "event-only"})
	if _, ok := registry.EventAdapterFor("event-only"); !ok {
		t.Fatal("expected event-only adapter to be registered for events")
	}
	if _, ok := registry.ItemAdapterFor("event-only"); ok {
		t.Fatal("did not expect event-only adapter to be registered for items")
	}
}
