package daemon

import (
	"net/http/httptest"
	"testing"
	"time"

	"control/internal/providers"
)

type stubTurnWaitProviderCapabilitiesResolver struct {
	capabilities map[string]providers.Capabilities
}

func (s stubTurnWaitProviderCapabilitiesResolver) Capabilities(provider string) providers.Capabilities {
	if s.capabilities == nil {
		return providers.Capabilities{}
	}
	return s.capabilities[provider]
}

func TestProviderTurnCompletionWaitStrategyRegistrySelectsEventsWaiter(t *testing.T) {
	var eventsCalls int
	var historyCalls int
	registry := providerTurnCompletionWaitStrategyRegistry{
		resolver: stubTurnWaitProviderCapabilitiesResolver{
			capabilities: map[string]providers.Capabilities{
				"opencode": {SupportsEvents: true},
			},
		},
		waiters: map[string]providerTurnCompletionWaiter{
			"events": func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) providerTurnCompletionResult {
				eventsCalls++
				return providerTurnCompletionResult{}
			},
			"history": func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) providerTurnCompletionResult {
				historyCalls++
				return providerTurnCompletionResult{}
			},
		},
	}

	registry.Waiter("opencode")(t, nil, nil, "sess-1", "turn-1", time.Second)
	if eventsCalls != 1 {
		t.Fatalf("expected events waiter call, got %d", eventsCalls)
	}
	if historyCalls != 0 {
		t.Fatalf("expected no history waiter call, got %d", historyCalls)
	}
}

func TestProviderTurnCompletionWaitStrategyRegistrySelectsHistoryWaiter(t *testing.T) {
	var eventsCalls int
	var historyCalls int
	registry := providerTurnCompletionWaitStrategyRegistry{
		resolver: stubTurnWaitProviderCapabilitiesResolver{
			capabilities: map[string]providers.Capabilities{
				"claude": {SupportsEvents: false},
			},
		},
		waiters: map[string]providerTurnCompletionWaiter{
			"events": func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) providerTurnCompletionResult {
				eventsCalls++
				return providerTurnCompletionResult{}
			},
			"history": func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) providerTurnCompletionResult {
				historyCalls++
				return providerTurnCompletionResult{}
			},
		},
	}

	registry.Waiter("claude")(t, nil, nil, "sess-1", "turn-1", time.Second)
	if historyCalls != 1 {
		t.Fatalf("expected history waiter call, got %d", historyCalls)
	}
	if eventsCalls != 0 {
		t.Fatalf("expected no events waiter call, got %d", eventsCalls)
	}
}

func TestProviderTurnCompletionWaitStrategyRegistryUsesFallbackWhenEventsWaiterMissing(t *testing.T) {
	var fallbackCalls int
	registry := providerTurnCompletionWaitStrategyRegistry{
		resolver: stubTurnWaitProviderCapabilitiesResolver{
			capabilities: map[string]providers.Capabilities{
				"opencode": {SupportsEvents: true},
			},
		},
		waiters: map[string]providerTurnCompletionWaiter{
			"events": nil,
		},
		fallbackWaiter: func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) providerTurnCompletionResult {
			fallbackCalls++
			return providerTurnCompletionResult{}
		},
	}

	registry.Waiter("opencode")(t, nil, nil, "sess-1", "turn-1", time.Second)
	if fallbackCalls != 1 {
		t.Fatalf("expected fallback waiter call, got %d", fallbackCalls)
	}
}

func TestNewProviderTurnCompletionWaitStrategyRegistryDefaultsResolver(t *testing.T) {
	var eventsCalls int
	registry := newProviderTurnCompletionWaitStrategyRegistry(nil)
	registry.waiters["events"] = func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) providerTurnCompletionResult {
		eventsCalls++
		return providerTurnCompletionResult{}
	}
	registry.waiters["history"] = func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) providerTurnCompletionResult {
		return providerTurnCompletionResult{}
	}
	registry.fallbackWaiter = func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) providerTurnCompletionResult {
		return providerTurnCompletionResult{}
	}

	registry.Waiter("codex")(t, nil, nil, "sess-1", "turn-1", time.Second)
	if eventsCalls != 1 {
		t.Fatalf("expected default resolver to route codex to events waiter, got %d", eventsCalls)
	}
}
