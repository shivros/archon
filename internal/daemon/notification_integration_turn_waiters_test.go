package daemon

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

const providerNotificationPollInterval = 50 * time.Millisecond

type providerTurnCompletionResult struct {
	TurnID string
	Status string
}

type providerTurnCompletionWaiter func(
	t *testing.T,
	env *notificationIntegrationEnvironment,
	sessionID string,
	expectedTurnID string,
	timeout time.Duration,
) providerTurnCompletionResult

// providerTurnCompletionWaitStrategyResolver resolves the provider-specific
// turn completion wait strategy from capability policy.
type providerTurnCompletionWaitStrategyResolver interface {
	Waiter(provider string) providerTurnCompletionWaiter
}

type providerTurnCompletionWaitStrategyRegistry struct {
	resolver       providerCapabilitiesResolver
	waiters        map[string]providerTurnCompletionWaiter
	fallbackWaiter providerTurnCompletionWaiter
}

func newProviderTurnCompletionWaitStrategyRegistry(resolver providerCapabilitiesResolver) providerTurnCompletionWaitStrategyRegistry {
	if resolver == nil {
		resolver = defaultProviderCapabilitiesResolver{}
	}
	return providerTurnCompletionWaitStrategyRegistry{
		resolver: resolver,
		waiters: map[string]providerTurnCompletionWaiter{
			"codex":   waitForProviderTurnCompletionFromLiveState,
			"claude":  waitForProviderTurnCompletionFromLiveState,
			"events":  waitForProviderTurnCompletionFromTranscript,
			"history": waitForProviderTurnCompletionFromHistory,
		},
		fallbackWaiter: waitForProviderTurnCompletionFromHistory,
	}
}

func (r providerTurnCompletionWaitStrategyRegistry) Waiter(provider string) providerTurnCompletionWaiter {
	if waiter := r.waiters[providers.Normalize(provider)]; waiter != nil {
		return waiter
	}
	key := "history"
	if r.resolver != nil && r.resolver.Capabilities(provider).SupportsEvents {
		key = "events"
	}
	waiter := r.waiters[key]
	if waiter == nil {
		waiter = r.fallbackWaiter
	}
	if waiter == nil {
		waiter = waitForProviderTurnCompletionFromHistory
	}
	return waiter
}

func waitForProviderTurnCompletionFromTranscript(
	t *testing.T,
	env *notificationIntegrationEnvironment,
	sessionID string,
	expectedTurnID string,
	timeout time.Duration,
) providerTurnCompletionResult {
	t.Helper()
	if env == nil {
		t.Fatalf("notification integration environment is required")
	}
	if result, ok := findTurnCompletionInHistory(t, env.server, sessionID, expectedTurnID); ok {
		return result
	}

	stream, closeFn := openSSE(t, env.server, "/v1/sessions/"+sessionID+"/transcript/stream?follow=1")
	defer closeFn()

	failures, stopFailures := startSessionTurnFailureMonitor(env.server, sessionID)
	defer stopFailures()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if failure := sessionTerminalFailure(env.server, sessionID); failure != "" {
			t.Fatalf("session entered terminal failure state while waiting for turn completion: %s\n%s", failure, sessionDiagnostics(env.manager, sessionID))
		}
		if result, ok := findTurnCompletionInHistory(t, env.server, sessionID, expectedTurnID); ok {
			return result
		}

		remaining := time.Until(deadline)
		waitWindow := 5 * time.Second
		if remaining < waitWindow {
			waitWindow = remaining
		}
		if waitWindow <= 0 {
			break
		}
		data, failure, ok := waitForSSEDataWithFailure(stream, failures, waitWindow)
		if strings.TrimSpace(failure) != "" {
			t.Fatalf("provider turn failed before completion event: %s\n%s", failure, sessionDiagnostics(env.manager, sessionID))
		}
		if !ok {
			time.Sleep(providerNotificationPollInterval)
			continue
		}
		event, parsed := codexEventFromSSEPayload(data)
		if !parsed || event.Method != "turn/completed" {
			continue
		}
		turn := parseTurnEventFromParams(event.Params)
		turnID := strings.TrimSpace(turn.TurnID)
		if strings.TrimSpace(expectedTurnID) != "" {
			if turnID == "" {
				turnID = strings.TrimSpace(expectedTurnID)
			}
			if turnID != strings.TrimSpace(expectedTurnID) {
				continue
			}
		}
		return providerTurnCompletionResult{
			TurnID: turnID,
			Status: strings.TrimSpace(turn.Status),
		}
	}
	t.Fatalf("timeout waiting for turn completion (expected_turn_id=%q)\n%s", strings.TrimSpace(expectedTurnID), sessionDiagnostics(env.manager, sessionID))
	return providerTurnCompletionResult{}
}

func waitForProviderTurnCompletionFromHistory(
	t *testing.T,
	env *notificationIntegrationEnvironment,
	sessionID string,
	expectedTurnID string,
	timeout time.Duration,
) providerTurnCompletionResult {
	t.Helper()
	if env == nil {
		t.Fatalf("notification integration environment is required")
	}
	failures, stopFailures := startSessionTurnFailureMonitor(env.server, sessionID)
	defer stopFailures()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if failure := sessionTerminalFailure(env.server, sessionID); failure != "" {
			t.Fatalf("session entered terminal failure state while waiting for turn completion: %s\n%s", failure, sessionDiagnostics(env.manager, sessionID))
		}
		select {
		case failure, ok := <-failures:
			if ok && strings.TrimSpace(failure) != "" {
				t.Fatalf("provider turn failed before completion event: %s\n%s", failure, sessionDiagnostics(env.manager, sessionID))
			}
		default:
		}
		if result, ok := findTurnCompletionInHistory(t, env.server, sessionID, expectedTurnID); ok {
			return result
		}
		time.Sleep(providerNotificationPollInterval)
	}
	t.Fatalf("timeout waiting for turn completion (expected_turn_id=%q)\n%s", strings.TrimSpace(expectedTurnID), sessionDiagnostics(env.manager, sessionID))
	return providerTurnCompletionResult{}
}

func waitForProviderTurnCompletionFromLiveState(
	t *testing.T,
	env *notificationIntegrationEnvironment,
	sessionID string,
	expectedTurnID string,
	timeout time.Duration,
) providerTurnCompletionResult {
	t.Helper()
	if env == nil {
		t.Fatalf("notification integration environment is required")
	}

	deadline := time.Now().Add(timeout)
	sawExpectedActive := false
	for time.Now().Before(deadline) {
		if failure := sessionTerminalFailure(env.server, sessionID); failure != "" {
			t.Fatalf("session entered terminal failure state while waiting for live turn completion: %s\n%s", failure, sessionDiagnostics(env.manager, sessionID))
		}
		if result, ok := findTurnCompletionInHistory(t, env.server, sessionID, expectedTurnID); ok {
			return result
		}
		activeTurnID, ok := activeTurnIDFromLiveSession(env, sessionID)
		if ok {
			if strings.TrimSpace(activeTurnID) == strings.TrimSpace(expectedTurnID) {
				sawExpectedActive = true
			}
			if strings.TrimSpace(activeTurnID) == "" || (sawExpectedActive && strings.TrimSpace(activeTurnID) != strings.TrimSpace(expectedTurnID)) {
				return providerTurnCompletionResult{TurnID: strings.TrimSpace(expectedTurnID)}
			}
		}
		time.Sleep(providerNotificationPollInterval)
	}

	t.Fatalf("timeout waiting for live turn completion (expected_turn_id=%q)\n%s", strings.TrimSpace(expectedTurnID), sessionDiagnostics(env.manager, sessionID))
	return providerTurnCompletionResult{}
}

func activeTurnIDFromLiveSession(env *notificationIntegrationEnvironment, sessionID string) (string, bool) {
	if env == nil || env.manager == nil || env.live == nil || strings.TrimSpace(sessionID) == "" {
		return "", false
	}
	session, ok := env.manager.GetSession(sessionID)
	if !ok || session == nil {
		return "", false
	}
	var meta *types.SessionMeta
	if env.stores != nil && env.stores.SessionMeta != nil {
		if stored, found, err := env.stores.SessionMeta.Get(context.Background(), sessionID); err == nil && found {
			meta = stored
		}
	}
	ls, err := env.live.ensure(context.Background(), session, meta)
	if err != nil || ls == nil {
		return "", false
	}
	return strings.TrimSpace(ls.ActiveTurnID()), true
}

func findTurnCompletionInHistory(
	t *testing.T,
	server *httptest.Server,
	sessionID string,
	expectedTurnID string,
) (providerTurnCompletionResult, bool) {
	t.Helper()
	history := historySession(t, server, sessionID)
	return findTurnCompletionInHistoryItems(history.Items, expectedTurnID)
}

func findTurnCompletionInHistoryItems(items []map[string]any, expectedTurnID string) (providerTurnCompletionResult, bool) {
	if len(items) == 0 {
		return providerTurnCompletionResult{}, false
	}
	expectedTurnID = strings.TrimSpace(expectedTurnID)
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if !strings.EqualFold(strings.TrimSpace(asString(item["type"])), "turnCompletion") {
			continue
		}
		turnID := strings.TrimSpace(asString(item["turn_id"]))
		if expectedTurnID != "" {
			if turnID == "" {
				turnID = expectedTurnID
			}
			if turnID != expectedTurnID {
				continue
			}
		}
		status := strings.TrimSpace(asString(item["turn_status"]))
		if status == "" {
			status = strings.TrimSpace(asString(item["status"]))
		}
		return providerTurnCompletionResult{
			TurnID: turnID,
			Status: status,
		}, true
	}
	return providerTurnCompletionResult{}, false
}

func normalizeProviderTurnStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "success", "succeeded", "done":
		return "completed"
	case "cancelled", "canceled", "aborted", "stopped":
		return "interrupted"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}
