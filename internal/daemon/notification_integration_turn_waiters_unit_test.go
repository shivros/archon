package daemon

import (
	"encoding/json"
	"net/http"
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
			"events": func(*testing.T, *notificationIntegrationEnvironment, string, string, time.Duration) providerTurnCompletionResult {
				eventsCalls++
				return providerTurnCompletionResult{}
			},
			"history": func(*testing.T, *notificationIntegrationEnvironment, string, string, time.Duration) providerTurnCompletionResult {
				historyCalls++
				return providerTurnCompletionResult{}
			},
		},
	}

	registry.Waiter("opencode")(t, nil, "sess-1", "turn-1", time.Second)
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
			"events": func(*testing.T, *notificationIntegrationEnvironment, string, string, time.Duration) providerTurnCompletionResult {
				eventsCalls++
				return providerTurnCompletionResult{}
			},
			"history": func(*testing.T, *notificationIntegrationEnvironment, string, string, time.Duration) providerTurnCompletionResult {
				historyCalls++
				return providerTurnCompletionResult{}
			},
		},
	}

	registry.Waiter("claude")(t, nil, "sess-1", "turn-1", time.Second)
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
		fallbackWaiter: func(*testing.T, *notificationIntegrationEnvironment, string, string, time.Duration) providerTurnCompletionResult {
			fallbackCalls++
			return providerTurnCompletionResult{}
		},
	}

	registry.Waiter("opencode")(t, nil, "sess-1", "turn-1", time.Second)
	if fallbackCalls != 1 {
		t.Fatalf("expected fallback waiter call, got %d", fallbackCalls)
	}
}

func TestNewProviderTurnCompletionWaitStrategyRegistryDefaultsResolver(t *testing.T) {
	var codexCalls int
	registry := newProviderTurnCompletionWaitStrategyRegistry(nil)
	registry.waiters["codex"] = func(*testing.T, *notificationIntegrationEnvironment, string, string, time.Duration) providerTurnCompletionResult {
		codexCalls++
		return providerTurnCompletionResult{}
	}
	registry.waiters["history"] = func(*testing.T, *notificationIntegrationEnvironment, string, string, time.Duration) providerTurnCompletionResult {
		return providerTurnCompletionResult{}
	}
	registry.fallbackWaiter = func(*testing.T, *notificationIntegrationEnvironment, string, string, time.Duration) providerTurnCompletionResult {
		return providerTurnCompletionResult{}
	}

	registry.Waiter("codex")(t, nil, "sess-1", "turn-1", time.Second)
	if codexCalls != 1 {
		t.Fatalf("expected default resolver to route codex to provider-specific waiter, got %d", codexCalls)
	}
}

func TestHistorySessionAllowingPendingReturnsFalseWhenHistoryPending(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"transcript_history_pending","error":"transcript history pending"}`))
	}))
	defer server.Close()

	_, ok := historySessionAllowingPending(t, server, "sess-pending")
	if ok {
		t.Fatalf("expected pending history response to return ok=false")
	}
}

func TestHistorySessionAllowingPendingReturnsPayloadOnSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %q", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(itemsResponse{
			Items: []map[string]any{
				{
					"type":        "turnCompletion",
					"turn_id":     "turn-1",
					"turn_status": "completed",
				},
			},
		})
	}))
	defer server.Close()

	payload, ok := historySessionAllowingPending(t, server, "sess-ok")
	if !ok {
		t.Fatalf("expected successful history response")
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected one history item, got %d", len(payload.Items))
	}
	if payload.Items[0]["turn_id"] != "turn-1" {
		t.Fatalf("unexpected history payload: %#v", payload.Items[0])
	}
}
