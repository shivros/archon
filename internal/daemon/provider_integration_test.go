package daemon

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
	"control/internal/types"
)

const providerFailurePollInterval = 50 * time.Millisecond

// providerTestCase defines the per-provider setup for table-driven integration tests.
type providerTestCase struct {
	name    string
	require func(t *testing.T)
	setup   func(t *testing.T) (repoDir string, runtimeOpts *types.SessionRuntimeOptions)
	timeout func() time.Duration
}

type providerCapabilitiesResolver interface {
	Capabilities(provider string) providers.Capabilities
}

type defaultProviderCapabilitiesResolver struct{}

func (defaultProviderCapabilitiesResolver) Capabilities(provider string) providers.Capabilities {
	return providers.CapabilitiesFor(provider)
}

type providerAgentReplyWaiter func(
	t *testing.T,
	server *httptest.Server,
	manager *SessionManager,
	sessionID string,
	needle string,
	timeout time.Duration,
)

type providerAgentReplyWaitStrategyRegistry struct {
	resolver       providerCapabilitiesResolver
	waiters        map[string]providerAgentReplyWaiter
	fallbackWaiter providerAgentReplyWaiter
}

func newProviderAgentReplyWaitStrategyRegistry(resolver providerCapabilitiesResolver) providerAgentReplyWaitStrategyRegistry {
	if resolver == nil {
		resolver = defaultProviderCapabilitiesResolver{}
	}
	return providerAgentReplyWaitStrategyRegistry{
		resolver: resolver,
		waiters: map[string]providerAgentReplyWaiter{
			"history": waitForAgentReply,
			"items":   waitForAgentReplyFromItems,
		},
		fallbackWaiter: waitForAgentReply,
	}
}

func (r providerAgentReplyWaitStrategyRegistry) Wait(
	t *testing.T,
	server *httptest.Server,
	manager *SessionManager,
	provider string,
	sessionID string,
	needle string,
	timeout time.Duration,
) {
	t.Helper()
	key := "history"
	if r.resolver != nil && r.resolver.Capabilities(provider).UsesItems {
		key = "items"
	}
	waiter := r.waiters[key]
	if waiter == nil {
		waiter = r.fallbackWaiter
	}
	if waiter == nil {
		waiter = waitForAgentReply
	}
	waiter(t, server, manager, sessionID, needle, timeout)
}

func allProviderTestCases() []providerTestCase {
	return []providerTestCase{
		{
			name:    "codex",
			require: requireCodexIntegration,
			setup:   codexIntegrationSetup,
			timeout: codexIntegrationTimeout,
		},
		{
			name:    "claude",
			require: requireClaudeIntegration,
			setup:   claudeIntegrationSetup,
			timeout: claudeIntegrationTimeout,
		},
		{
			name: "opencode",
			require: func(t *testing.T) {
				t.Helper()
				requireOpenCodeIntegration(t, "opencode")
			},
			setup: func(t *testing.T) (string, *types.SessionRuntimeOptions) {
				t.Helper()
				return openCodeIntegrationSetup(t, "opencode")
			},
			timeout: func() time.Duration { return openCodeIntegrationTimeout("opencode") },
		},
		{
			name: "kilocode",
			require: func(t *testing.T) {
				t.Helper()
				requireOpenCodeIntegration(t, "kilocode")
			},
			setup: func(t *testing.T) (string, *types.SessionRuntimeOptions) {
				t.Helper()
				return openCodeIntegrationSetup(t, "kilocode")
			},
			timeout: func() time.Duration { return openCodeIntegrationTimeout("kilocode") },
		},
	}
}

// TestProviderSessionFlow verifies the core session lifecycle for every provider:
// start a session with initial text, wait for the agent to reply, send a follow-up,
// and wait for the agent to reply again. Using the same test body for all providers
// ensures consistent assertion strength and prevents regressions like the initial-
// message silent drop that affected OpenCode/KiloCode.
func TestProviderSessionFlow(t *testing.T) {
	waitStrategy := newProviderAgentReplyWaitStrategyRegistry(defaultProviderCapabilitiesResolver{})
	for _, tc := range allProviderTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.require(t)

			repoDir, runtimeOpts := tc.setup(t)
			server, manager, _ := newCodexIntegrationServer(t)
			defer server.Close()

			ws := createWorkspace(t, server, repoDir)
			session := startSession(t, server, StartSessionRequest{
				Provider:       tc.name,
				WorkspaceID:    ws.ID,
				Text:           "Say \"ok\" and nothing else.",
				RuntimeOptions: runtimeOpts,
			})
			if session.ID == "" {
				t.Fatalf("session id missing")
			}

			list := listSessions(t, server)
			if len(list.Sessions) == 0 {
				t.Fatalf("expected sessions list to be non-empty")
			}

			// Wait for the initial agent reply (strong assertion for all providers).
			timeout := tc.timeout()
			waitStrategy.Wait(t, server, manager, tc.name, session.ID, "ok", timeout)

			// Send a follow-up and wait for reply.
			turnID := sendMessageWithRetry(t, server, session.ID, "Say \"ok\" again.", timeout)
			if turnID == "" {
				t.Fatalf("turn id missing from send")
			}
			waitStrategy.Wait(t, server, manager, tc.name, session.ID, "ok", timeout)
		})
	}
}

func TestProviderInterruptParity(t *testing.T) {
	for _, tc := range allProviderTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.require(t)

			repoDir, runtimeOpts := tc.setup(t)
			server, manager, _ := newCodexIntegrationServer(t)
			defer server.Close()

			ws := createWorkspace(t, server, repoDir)
			session := startSession(t, server, StartSessionRequest{
				Provider:       tc.name,
				WorkspaceID:    ws.ID,
				RuntimeOptions: runtimeOpts,
			})

			stream, closeFn := openSSE(t, server, "/v1/sessions/"+session.ID+"/transcript/stream?follow=1")
			defer closeFn()

			finalToken := "interrupt-final-token-" + tc.name
			turnID := sendMessageWithRetry(t, server, session.ID, providerInterruptPrompt(finalToken), tc.timeout())
			if strings.TrimSpace(turnID) == "" {
				t.Fatalf("expected turn id from send")
			}

			if !waitForProviderInterruptReadiness(stream, 2*time.Second) {
				time.Sleep(1500 * time.Millisecond)
			}

			status, body := interruptSession(server, session.ID)
			if status != 200 {
				t.Fatalf("interrupt failed status=%d body=%s", status, body)
			}

			waitForProviderInterruptConfirmation(t, server, manager, session.ID, stream, finalToken, 45*time.Second)
		})
	}
}

func providerInterruptPrompt(finalToken string) string {
	return "Write a very detailed response with at least 18 numbered sections about distributed systems, reliability, scheduling, retries, and failure handling. " +
		"Do not stop early. Put the exact final line `" + strings.TrimSpace(finalToken) + "` only after the full response is completely finished."
}

func waitForProviderInterruptReadiness(stream <-chan string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-stream:
			if !ok {
				return false
			}
			var event transcriptdomain.TranscriptEvent
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				if event.Kind == transcriptdomain.TranscriptEventTurnStarted || event.Kind == transcriptdomain.TranscriptEventDelta {
					return true
				}
			}
			if codexEvent, ok := codexEventFromSSEPayload(data); ok {
				if codexEvent.Method == "turn/started" {
					return true
				}
			}
		case <-time.After(providerFailurePollInterval):
		}
	}
	return false
}

func waitForProviderInterruptConfirmation(
	t *testing.T,
	server *httptest.Server,
	manager *SessionManager,
	sessionID string,
	stream <-chan string,
	finalToken string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		history := historySession(t, server, sessionID)
		if historyHasAgentText(history.Items, finalToken) {
			t.Fatalf("provider completed after interrupt and emitted final token %q\n%s", finalToken, sessionDiagnostics(manager, sessionID))
		}
		if historyHasInterruptedTurn(history.Items) {
			return
		}

		if failure := sessionTerminalFailure(server, sessionID); failure != "" {
			t.Fatalf("session entered terminal state %q while waiting for interrupt confirmation\n%s", failure, sessionDiagnostics(manager, sessionID))
		}

		data, ok := waitForSSEData(stream, 2*time.Second)
		if !ok {
			continue
		}
		if transcriptPayloadHasAgentText(data, finalToken) {
			t.Fatalf("provider streamed final token %q after interrupt\n%s", finalToken, sessionDiagnostics(manager, sessionID))
		}
		if transcriptPayloadIndicatesInterrupted(data) {
			return
		}
	}
	t.Fatalf("timeout waiting for interrupted turn confirmation\n%s", sessionDiagnostics(manager, sessionID))
}

func historyHasInterruptedTurn(items []map[string]any) bool {
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(asString(item["type"])), "turnCompletion") {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(asString(item["turn_status"])))
		errMsg := strings.ToLower(strings.TrimSpace(asString(item["turn_error"])))
		switch status {
		case "interrupted", "aborted", "cancelled", "canceled", "stopped":
			return true
		}
		if strings.Contains(errMsg, "interrupt") {
			return true
		}
	}
	return false
}

func transcriptPayloadHasAgentText(data string, needle string) bool {
	var event transcriptdomain.TranscriptEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return false
	}
	return transcriptEventHasAgentText(event, needle)
}

func transcriptPayloadIndicatesInterrupted(data string) bool {
	event, ok := codexEventFromSSEPayload(data)
	if !ok || event.Method != "turn/completed" {
		return false
	}
	turn := parseTurnEventFromParams(event.Params)
	status := strings.ToLower(strings.TrimSpace(turn.Status))
	switch status {
	case "interrupted", "aborted", "cancelled", "canceled", "stopped":
		return true
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(turn.Error)), "interrupt")
}

func waitForAgentReplyFromItems(t *testing.T, server *httptest.Server, manager *SessionManager, sessionID, needle string, timeout time.Duration) {
	t.Helper()
	stream, closeFn := openSSE(t, server, "/v1/sessions/"+sessionID+"/transcript/stream?follow=1")
	defer closeFn()

	failures, stopFailures := startSessionTurnFailureMonitor(server, sessionID)
	defer stopFailures()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		history := historySession(t, server, sessionID)
		if historyHasAgentText(history.Items, needle) {
			return
		}
		if failure := sessionTerminalFailure(server, sessionID); failure != "" {
			t.Fatalf("session entered terminal failure state before agent reply containing %q: %s\n%s", needle, failure, sessionDiagnostics(manager, sessionID))
		}
		data, failure, ok := waitForSSEDataWithFailure(stream, failures, 5*time.Second)
		if strings.TrimSpace(failure) != "" {
			t.Fatalf("provider turn failed before agent reply containing %q: %s\n%s", needle, failure, sessionDiagnostics(manager, sessionID))
		}
		if !ok {
			continue
		}
		var event transcriptdomain.TranscriptEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if transcriptEventHasAgentText(event, needle) {
			return
		}
	}
	t.Fatalf("timeout waiting for agent reply containing %q\n%s", needle, sessionDiagnostics(manager, sessionID))
}

func transcriptEventHasAgentText(event transcriptdomain.TranscriptEvent, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	if event.Kind != transcriptdomain.TranscriptEventDelta {
		return false
	}
	for _, block := range event.Delta {
		role := strings.ToLower(strings.TrimSpace(block.Role))
		kind := strings.ToLower(strings.TrimSpace(block.Kind))
		if role != "assistant" && role != "agent" && role != "model" &&
			kind != "assistant" && kind != "agent" && kind != "model" {
			continue
		}
		if strings.Contains(strings.ToLower(block.Text), needle) {
			return true
		}
	}
	return false
}

// TestProviderMissingModelFailsFast ensures OpenCode-family providers do not
// attempt upstream work when no runtime model is provided.
func TestProviderMissingModelFailsFast(t *testing.T) {
	for _, provider := range integrationOpenCodeProviders() {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			requireOpenCodeIntegration(t, provider)

			repoDir := createOpenCodeWorkspace(t, provider)
			server, manager, _ := newCodexIntegrationServer(t)
			defer server.Close()

			ws := createWorkspace(t, server, repoDir)
			session := startSession(t, server, StartSessionRequest{
				Provider:    provider,
				WorkspaceID: ws.ID,
				Text:        "Say \"ok\" and nothing else.",
				// Intentionally omitted to verify fail-fast behavior.
				RuntimeOptions: nil,
			})

			failures, stopFailures := startSessionTurnFailureMonitor(server, session.ID)
			defer stopFailures()
			assertSessionFailsFastWithModelRequired(t, server, manager, session.ID, failures, 15*time.Second)
		})
	}
}

func assertSessionFailsFastWithModelRequired(
	t *testing.T,
	server *httptest.Server,
	manager *SessionManager,
	sessionID string,
	failures <-chan string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if historyHasModelRequiredTurnFailure(t, server, sessionID) {
			return
		}
		if failure := sessionTerminalFailure(server, sessionID); failure != "" {
			select {
			case msg, ok := <-failures:
				if ok && isModelRequiredFailure(msg) {
					return
				}
			case <-time.After(providerFailurePollInterval):
			}
			t.Fatalf("session entered terminal state %q without model-required error\n%s", failure, sessionDiagnostics(manager, sessionID))
		}
		select {
		case msg, ok := <-failures:
			if !ok {
				break
			}
			if isModelRequiredFailure(msg) {
				return
			}
			t.Fatalf("expected model-required failure, got %q\n%s", msg, sessionDiagnostics(manager, sessionID))
		case <-time.After(providerFailurePollInterval):
		}
	}
	t.Fatalf("timeout waiting for model-required failure\n%s", sessionDiagnostics(manager, sessionID))
}

func isModelRequiredFailure(msg string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(msg)), "model is required")
}

func historyTurnFailureMessage(items []map[string]any) string {
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(asString(item["type"])), "turnCompletion") {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(asString(item["turn_status"])))
		errMsg := strings.TrimSpace(asString(item["turn_error"]))
		if errMsg == "" {
			errMsg = strings.TrimSpace(asString(item["error"]))
		}
		switch status {
		case "failed", "error", "abandoned":
			if errMsg != "" {
				return "turn " + status + ": " + errMsg
			}
			return "turn " + status
		}
	}
	return ""
}

func historyHasModelRequiredTurnFailure(t *testing.T, server *httptest.Server, sessionID string) bool {
	t.Helper()
	history := historySession(t, server, sessionID)
	for _, item := range history.Items {
		if !strings.EqualFold(strings.TrimSpace(asString(item["type"])), "turnCompletion") {
			continue
		}
		if isModelRequiredFailure(asString(item["turn_error"])) ||
			isModelRequiredFailure(asString(item["error"])) {
			return true
		}
	}
	return false
}

type stubProviderCapabilitiesResolver struct {
	capabilities map[string]providers.Capabilities
}

func (s stubProviderCapabilitiesResolver) Capabilities(provider string) providers.Capabilities {
	if s.capabilities == nil {
		return providers.Capabilities{}
	}
	return s.capabilities[provider]
}

func TestProviderAgentReplyWaitStrategyRegistryWaitSelectsItemsWaiter(t *testing.T) {
	var historyCalls int
	var itemCalls int
	registry := providerAgentReplyWaitStrategyRegistry{
		resolver: stubProviderCapabilitiesResolver{
			capabilities: map[string]providers.Capabilities{
				"opencode": {UsesItems: true},
			},
		},
		waiters: map[string]providerAgentReplyWaiter{
			"history": func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) {
				historyCalls++
			},
			"items": func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) {
				itemCalls++
			},
		},
	}

	registry.Wait(t, nil, nil, "opencode", "sess-1", "ok", time.Second)
	if itemCalls != 1 {
		t.Fatalf("expected items waiter call, got %d", itemCalls)
	}
	if historyCalls != 0 {
		t.Fatalf("expected no history waiter call, got %d", historyCalls)
	}
}

func TestProviderAgentReplyWaitStrategyRegistryWaitSelectsHistoryWaiter(t *testing.T) {
	var historyCalls int
	var itemCalls int
	registry := providerAgentReplyWaitStrategyRegistry{
		resolver: stubProviderCapabilitiesResolver{
			capabilities: map[string]providers.Capabilities{
				"codex": {UsesItems: false},
			},
		},
		waiters: map[string]providerAgentReplyWaiter{
			"history": func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) {
				historyCalls++
			},
			"items": func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) {
				itemCalls++
			},
		},
	}

	registry.Wait(t, nil, nil, "codex", "sess-1", "ok", time.Second)
	if historyCalls != 1 {
		t.Fatalf("expected history waiter call, got %d", historyCalls)
	}
	if itemCalls != 0 {
		t.Fatalf("expected no items waiter call, got %d", itemCalls)
	}
}

func TestProviderAgentReplyWaitStrategyRegistryWaitUsesFallbackWhenStrategyMissing(t *testing.T) {
	var fallbackCalls int
	registry := providerAgentReplyWaitStrategyRegistry{
		resolver: stubProviderCapabilitiesResolver{
			capabilities: map[string]providers.Capabilities{
				"opencode": {UsesItems: true},
			},
		},
		waiters: map[string]providerAgentReplyWaiter{
			"history": func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) {},
			"items":   nil,
		},
		fallbackWaiter: func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) {
			fallbackCalls++
		},
	}

	registry.Wait(t, nil, nil, "opencode", "sess-1", "ok", time.Second)
	if fallbackCalls != 1 {
		t.Fatalf("expected fallback waiter call, got %d", fallbackCalls)
	}
}

func TestNewProviderAgentReplyWaitStrategyRegistryDefaultsResolver(t *testing.T) {
	var itemCalls int
	registry := newProviderAgentReplyWaitStrategyRegistry(nil)
	registry.waiters["items"] = func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) {
		itemCalls++
	}
	registry.waiters["history"] = func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) {}
	registry.fallbackWaiter = func(*testing.T, *httptest.Server, *SessionManager, string, string, time.Duration) {}

	registry.Wait(t, nil, nil, "opencode", "sess-1", "ok", time.Second)
	if itemCalls != 1 {
		t.Fatalf("expected default resolver to route opencode to items waiter, got %d", itemCalls)
	}
}

func TestIsModelRequiredFailure(t *testing.T) {
	if !isModelRequiredFailure("  MODEL is REQUIRED  ") {
		t.Fatalf("expected case-insensitive match")
	}
	if isModelRequiredFailure("model unsupported") {
		t.Fatalf("expected non-match for unrelated error")
	}
}

// TestProviderItemsStream verifies the SSE /items endpoint for providers that
// use item-based history (UsesItems capability). Codex uses /tail instead and
// is tested separately in TestCodexTailStream.
func TestProviderItemsStream(t *testing.T) {
	for _, tc := range allProviderTestCases() {
		if !providers.CapabilitiesFor(tc.name).UsesItems {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.require(t)

			repoDir, runtimeOpts := tc.setup(t)
			server, manager, _ := newCodexIntegrationServer(t)
			defer server.Close()

			ws := createWorkspace(t, server, repoDir)
			session := startSession(t, server, StartSessionRequest{
				Provider:       tc.name,
				WorkspaceID:    ws.ID,
				Text:           "Say \"ok\" and nothing else.",
				RuntimeOptions: runtimeOpts,
			})

			streamPath := "/v1/sessions/" + session.ID + "/transcript/stream?follow=1"
			stream, closeFn := openSSE(t, server, streamPath)
			defer func() {
				if closeFn != nil {
					closeFn()
				}
			}()
			lastRevision := ""
			failures, stopFailures := startSessionTurnFailureMonitor(server, session.ID)
			defer stopFailures()

			data, failure, ok := waitForSSEDataWithFailure(stream, failures, 30*time.Second)
			if strings.TrimSpace(failure) != "" {
				t.Fatalf("items stream aborted by provider failure: %s\n%s",
					failure, sessionDiagnostics(manager, session.ID))
			}
			if failure := sessionTerminalFailure(server, session.ID); failure != "" {
				t.Fatalf("session entered terminal failure state before items stream event: %s\n%s",
					failure, sessionDiagnostics(manager, session.ID))
			}
			if !ok {
				diagnostics := sessionDiagnostics(manager, session.ID)
				if isGuidedWorkflowProviderRuntimeUnavailableFailure(tc.name, diagnostics) {
					t.Skipf("skipping provider %q items stream due runtime/auth unavailability (%s)", tc.name, strings.TrimSpace(diagnostics))
				}
				t.Fatalf("timeout waiting for items stream event\n%s", diagnostics)
			}

			var firstEvent transcriptdomain.TranscriptEvent
			if err := json.Unmarshal([]byte(data), &firstEvent); err != nil {
				t.Fatalf("decode item: %v", err)
			}
			if strings.TrimSpace(string(firstEvent.Kind)) == "" {
				t.Fatalf("expected transcript event kind to be set")
			}
			lastRevision = strings.TrimSpace(firstEvent.Revision.String())

			sendMessageWithRetry(t, server, session.ID, "Say \"ok\" again.", tc.timeout())
			deadline := time.Now().Add(45 * time.Second)
			for time.Now().Before(deadline) {
				history := historySession(t, server, session.ID)
				if historyHasAgentText(history.Items, "ok") {
					return
				}
				if failureMsg := historyTurnFailureMessage(history.Items); strings.TrimSpace(failureMsg) != "" {
					if isGuidedWorkflowProviderRuntimeUnavailableFailure(tc.name, failureMsg) {
						t.Skipf("skipping provider %q items stream due runtime/auth unavailability (%s)", tc.name, strings.TrimSpace(failureMsg))
					}
					t.Fatalf("session produced failed turn while waiting for items stream reply: %s\n%s",
						failureMsg, sessionDiagnostics(manager, session.ID))
				}
				data, failure, ok = waitForSSEDataWithFailure(stream, failures, 5*time.Second)
				if strings.TrimSpace(failure) != "" {
					if isGuidedWorkflowProviderRuntimeUnavailableFailure(tc.name, failure) {
						t.Skipf("skipping provider %q items stream due runtime/auth unavailability (%s)", tc.name, strings.TrimSpace(failure))
					}
					t.Fatalf("items stream aborted by provider failure: %s\n%s",
						failure, sessionDiagnostics(manager, session.ID))
				}
				if failure := sessionTerminalFailure(server, session.ID); failure != "" {
					t.Fatalf("session entered terminal failure state while waiting for items stream reply: %s\n%s",
						failure, sessionDiagnostics(manager, session.ID))
				}
				if !ok {
					if closeFn != nil {
						closeFn()
					}
					reconnectPath := streamPath
					if lastRevision != "" {
						reconnectPath += "&after_revision=" + url.QueryEscape(lastRevision)
					}
					stream, closeFn = openSSE(t, server, reconnectPath)
					continue
				}
				var event transcriptdomain.TranscriptEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue
				}
				if rev := strings.TrimSpace(event.Revision.String()); rev != "" {
					lastRevision = rev
				}
				if transcriptEventHasAgentText(event, "ok") {
					return
				}
			}
			diagnostics := sessionDiagnostics(manager, session.ID)
			if isGuidedWorkflowProviderRuntimeUnavailableFailure(tc.name, diagnostics) {
				t.Skipf("skipping provider %q items stream due runtime/auth unavailability (%s)", tc.name, strings.TrimSpace(diagnostics))
			}
			t.Fatalf("timeout waiting for agent reply on items stream\n%s", diagnostics)
		})
	}
}

// TestProviderEventsStream verifies the SSE /events endpoint for providers that
// support event streaming (SupportsEvents capability). Claude does not support
// this endpoint.
func TestProviderEventsStream(t *testing.T) {
	for _, tc := range allProviderTestCases() {
		if !providers.CapabilitiesFor(tc.name).SupportsEvents {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.require(t)

			repoDir, runtimeOpts := tc.setup(t)
			server, manager, _ := newCodexIntegrationServer(t)
			defer server.Close()

			ws := createWorkspace(t, server, repoDir)
			session := startSession(t, server, StartSessionRequest{
				Provider:       tc.name,
				WorkspaceID:    ws.ID,
				Text:           "Say \"ok\" and nothing else.",
				RuntimeOptions: runtimeOpts,
			})

			stream, closeFn := openSSE(t, server,
				"/v1/sessions/"+session.ID+"/transcript/stream?follow=1")
			defer closeFn()

			_ = sendMessageWithRetry(t, server, session.ID, "Say \"ok\" again.", tc.timeout())

			events := collectTranscriptEventsFromSSE(stream, 45*time.Second)
			if len(events) == 0 {
				t.Fatalf("expected events from SSE stream\n%s",
					sessionDiagnostics(manager, session.ID))
			}
			found := false
			for _, event := range events {
				if strings.TrimSpace(string(event.Kind)) != "" {
					found = true
					break
				}
			}
			if !found {
				methods := make([]string, 0, len(events))
				for _, event := range events {
					methods = append(methods, string(event.Kind))
				}
				t.Fatalf("expected at least one event with Kind set, got=%v\n%s",
					methods, sessionDiagnostics(manager, session.ID))
			}
		})
	}
}

func collectTranscriptEventsFromSSE(ch <-chan string, timeout time.Duration) []transcriptdomain.TranscriptEvent {
	deadline := time.Now().Add(timeout)
	out := make([]transcriptdomain.TranscriptEvent, 0)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return out
			}
			var event transcriptdomain.TranscriptEvent
			if err := json.Unmarshal([]byte(data), &event); err == nil && strings.TrimSpace(string(event.Kind)) != "" {
				out = append(out, event)
			}
		case <-time.After(integrationSSEPollInterval):
		}
	}
	return out
}
