package app

import (
	"testing"

	"control/internal/types"
)

type stubRuntimeCatalogLookup struct {
	catalogs map[string]*types.ProviderOptionCatalog
}

func (s stubRuntimeCatalogLookup) providerOptionCatalog(provider string) *types.ProviderOptionCatalog {
	if s.catalogs == nil {
		return nil
	}
	return s.catalogs[provider]
}

type stubRuntimeDefaultsLookup struct {
	defaults map[string]*types.SessionRuntimeOptions
}

func (s stubRuntimeDefaultsLookup) composeDefaultsForProvider(provider string) *types.SessionRuntimeOptions {
	if s.defaults == nil {
		return nil
	}
	return types.CloneRuntimeOptions(s.defaults[provider])
}

func TestRuntimeOptionResolverFallbacksIncompatibleSelections(t *testing.T) {
	resolver := newRuntimeOptionResolver(
		stubRuntimeCatalogLookup{
			catalogs: map[string]*types.ProviderOptionCatalog{
				"claude": {
					Provider:     "claude",
					Models:       []string{"sonnet", "opus"},
					AccessLevels: []types.AccessLevel{types.AccessReadOnly, types.AccessOnRequest},
					Defaults: types.SessionRuntimeOptions{
						Model:  "sonnet",
						Access: types.AccessOnRequest,
					},
				},
			},
		},
		nil,
	)

	got := resolver.resolve("claude", &types.SessionRuntimeOptions{
		Model:     "gpt-5.3-codex",
		Reasoning: types.ReasoningHigh,
		Access:    types.AccessFull,
	})
	if got.Model != "sonnet" {
		t.Fatalf("expected model fallback to provider default, got %q", got.Model)
	}
	if got.Access != types.AccessOnRequest {
		t.Fatalf("expected access fallback to provider default, got %q", got.Access)
	}
	if got.Reasoning != "" {
		t.Fatalf("expected reasoning cleared when provider has no reasoning levels, got %q", got.Reasoning)
	}
}

func TestRuntimeOptionResolverPreservesCompatibleSelections(t *testing.T) {
	resolver := newRuntimeOptionResolver(
		stubRuntimeCatalogLookup{
			catalogs: map[string]*types.ProviderOptionCatalog{
				"opencode": {
					Provider:        "opencode",
					Models:          []string{"shared-model", "other"},
					ReasoningLevels: []types.ReasoningLevel{types.ReasoningLow, types.ReasoningHigh},
					AccessLevels:    []types.AccessLevel{types.AccessOnRequest, types.AccessFull},
					Defaults: types.SessionRuntimeOptions{
						Model:     "other",
						Reasoning: types.ReasoningLow,
						Access:    types.AccessOnRequest,
					},
				},
			},
		},
		nil,
	)

	got := resolver.resolve("opencode", &types.SessionRuntimeOptions{
		Model:     "shared-model",
		Reasoning: types.ReasoningHigh,
		Access:    types.AccessFull,
	})
	if got.Model != "shared-model" || got.Reasoning != types.ReasoningHigh || got.Access != types.AccessFull {
		t.Fatalf("expected compatible selections preserved, got %#v", got)
	}
}

func TestRuntimeOptionResolverUsesPersistedProviderDefaults(t *testing.T) {
	resolver := newRuntimeOptionResolver(
		stubRuntimeCatalogLookup{
			catalogs: map[string]*types.ProviderOptionCatalog{
				"codex": {
					Provider:        "codex",
					Models:          []string{"gpt-5.1-codex", "gpt-5.3-codex"},
					ReasoningLevels: []types.ReasoningLevel{types.ReasoningLow, types.ReasoningMedium, types.ReasoningHigh},
					AccessLevels:    []types.AccessLevel{types.AccessReadOnly, types.AccessOnRequest, types.AccessFull},
					Defaults: types.SessionRuntimeOptions{
						Model:     "gpt-5.1-codex",
						Reasoning: types.ReasoningMedium,
						Access:    types.AccessOnRequest,
					},
				},
			},
		},
		stubRuntimeDefaultsLookup{
			defaults: map[string]*types.SessionRuntimeOptions{
				"codex": {
					Model:     "gpt-5.3-codex",
					Reasoning: types.ReasoningHigh,
					Access:    types.AccessFull,
				},
			},
		},
	)

	got := resolver.resolve("codex", &types.SessionRuntimeOptions{
		Model:     "invalid-model",
		Reasoning: types.ReasoningExtraHigh,
		Access:    types.AccessLevel("invalid"),
	})
	if got.Model != "gpt-5.3-codex" {
		t.Fatalf("expected persisted default model fallback, got %q", got.Model)
	}
	if got.Access != types.AccessFull {
		t.Fatalf("expected persisted default access fallback, got %q", got.Access)
	}
	if got.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected persisted default reasoning fallback, got %q", got.Reasoning)
	}
}
