package app

import (
	"testing"

	"control/internal/types"
)

func TestProviderCapabilitiesRecentsCompletionPolicySignals(t *testing.T) {
	policy := providerCapabilitiesRecentsCompletionPolicy{}
	cases := []struct {
		name         string
		provider     string
		watch        bool
		metaFallback bool
	}{
		{name: "event provider", provider: "codex", watch: true, metaFallback: false},
		{name: "non-event provider", provider: "claude", watch: false, metaFallback: true},
		{name: "custom provider", provider: "custom", watch: false, metaFallback: true},
		{name: "unknown provider", provider: "my-provider", watch: false, metaFallback: false},
		{name: "empty provider", provider: "", watch: false, metaFallback: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := policy.ShouldWatchCompletion(tc.provider); got != tc.watch {
				t.Fatalf("watch mismatch for %q: got=%v want=%v", tc.provider, got, tc.watch)
			}
			if got := policy.ShouldUseMetaFallback(tc.provider); got != tc.metaFallback {
				t.Fatalf("meta fallback mismatch for %q: got=%v want=%v", tc.provider, got, tc.metaFallback)
			}
		})
	}
}

func TestProviderCapabilitiesRecentsCompletionPolicyTurnResolution(t *testing.T) {
	policy := providerCapabilitiesRecentsCompletionPolicy{}
	meta := &types.SessionMeta{LastTurnID: "turn-meta"}

	if got := policy.RunBaselineTurnID("turn-send", meta); got != "turn-send" {
		t.Fatalf("expected send turn baseline, got %q", got)
	}
	if got := policy.RunBaselineTurnID("", meta); got != "turn-meta" {
		t.Fatalf("expected meta turn baseline, got %q", got)
	}
	if got := policy.CompletionTurnID("turn-event", meta); got != "turn-event" {
		t.Fatalf("expected event completion turn, got %q", got)
	}
	if got := policy.CompletionTurnID("", meta); got != "turn-meta" {
		t.Fatalf("expected meta completion turn fallback, got %q", got)
	}
}

func TestRecentsMetaFallbackMapIncludesOnlyConfiguredFallbackProviders(t *testing.T) {
	m := NewModel(nil)
	m.sessions = []*types.Session{
		{ID: "s-codex", Provider: "codex"},
		{ID: "s-claude", Provider: "claude"},
		{ID: "s-custom", Provider: "custom"},
		{ID: "s-unknown", Provider: "my-provider"},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s-codex":   {SessionID: "s-codex", LastTurnID: "turn-1"},
		"s-claude":  {SessionID: "s-claude", LastTurnID: "turn-2"},
		"s-custom":  {SessionID: "s-custom", LastTurnID: "turn-3"},
		"s-unknown": {SessionID: "s-unknown", LastTurnID: "turn-4"},
	}

	filtered := m.recentsMetaFallbackMap()
	if _, ok := filtered["s-codex"]; ok {
		t.Fatalf("did not expect codex (event provider) in metadata fallback map")
	}
	if _, ok := filtered["s-claude"]; !ok {
		t.Fatalf("expected claude in metadata fallback map")
	}
	if _, ok := filtered["s-custom"]; !ok {
		t.Fatalf("expected custom in metadata fallback map")
	}
	if _, ok := filtered["s-unknown"]; ok {
		t.Fatalf("did not expect unknown provider in metadata fallback map")
	}
}
