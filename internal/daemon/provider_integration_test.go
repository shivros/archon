package daemon

import (
	"encoding/json"
	"testing"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

// providerTestCase defines the per-provider setup for table-driven integration tests.
type providerTestCase struct {
	name    string
	require func(t *testing.T)
	setup   func(t *testing.T) (repoDir string, runtimeOpts *types.SessionRuntimeOptions)
	timeout func() time.Duration
}

func allProviderTestCases() []providerTestCase {
	return []providerTestCase{
		{
			name:    "codex",
			require: requireCodexIntegration,
			setup: func(t *testing.T) (string, *types.SessionRuntimeOptions) {
				t.Helper()
				repoDir, codexHome := createCodexWorkspace(t)
				model := resolveCodexIntegrationModelForWorkspace(t, repoDir, codexHome)
				return repoDir, &types.SessionRuntimeOptions{Model: model}
			},
			timeout: codexIntegrationTimeout,
		},
		{
			name:    "claude",
			require: requireClaudeIntegration,
			setup: func(t *testing.T) (string, *types.SessionRuntimeOptions) {
				t.Helper()
				return createClaudeWorkspace(t), nil
			},
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
				return createOpenCodeWorkspace(t, "opencode"), nil
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
				return createOpenCodeWorkspace(t, "kilocode"), nil
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
	for _, tc := range allProviderTestCases() {
		t.Run(tc.name, func(t *testing.T) {
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
			waitForAgentReply(t, server, manager, session.ID, "ok", timeout)

			// Send a follow-up and wait for reply.
			turnID := sendMessageWithRetry(t, server, session.ID, "Say \"ok\" again.", timeout)
			if turnID == "" {
				t.Fatalf("turn id missing from send")
			}
			waitForAgentReply(t, server, manager, session.ID, "ok", timeout)
		})
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
				"/v1/sessions/"+session.ID+"/items?follow=1&lines=100")
			defer closeFn()

			data, ok := waitForSSEData(stream, 30*time.Second)
			if !ok {
				t.Fatalf("timeout waiting for items stream event\n%s",
					sessionDiagnostics(manager, session.ID))
			}

			var item map[string]any
			if err := json.Unmarshal([]byte(data), &item); err != nil {
				t.Fatalf("decode item: %v", err)
			}
			if typ, _ := item["type"].(string); typ == "" {
				t.Fatalf("expected item type to be set")
			}

			sendMessageWithRetry(t, server, session.ID, "Say \"ok\" again.", tc.timeout())
			deadline := time.Now().Add(45 * time.Second)
			for time.Now().Before(deadline) {
				data, ok = waitForSSEData(stream, 5*time.Second)
				if !ok {
					continue
				}
				if err := json.Unmarshal([]byte(data), &item); err != nil {
					continue
				}
				if historyHasAgentText([]map[string]any{item}, "ok") {
					return
				}
			}
			t.Fatalf("timeout waiting for agent reply on items stream\n%s",
				sessionDiagnostics(manager, session.ID))
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
				"/v1/sessions/"+session.ID+"/events?follow=1")
			defer closeFn()

			_ = sendMessageWithRetry(t, server, session.ID, "Say \"ok\" again.", tc.timeout())

			events := collectEvents(stream, 45*time.Second)
			if len(events) == 0 {
				t.Fatalf("expected events from SSE stream\n%s",
					sessionDiagnostics(manager, session.ID))
			}
			found := false
			for _, event := range events {
				if event.Method != "" {
					found = true
					break
				}
			}
			if !found {
				methods := make([]string, 0, len(events))
				for _, event := range events {
					methods = append(methods, event.Method)
				}
				t.Fatalf("expected at least one event with Method set, got=%v\n%s",
					methods, sessionDiagnostics(manager, session.ID))
			}
		})
	}
}
