package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/providers"
)

const (
	opencodeIntegrationEnv = "ARCHON_OPENCODE_INTEGRATION"
	kilocodeIntegrationEnv = "ARCHON_KILOCODE_INTEGRATION"
)

func TestAPIOpenCodeSessionFlow(t *testing.T) {
	for _, provider := range integrationOpenCodeProviders() {
		t.Run(provider, func(t *testing.T) {
			requireOpenCodeIntegration(t, provider)

			repoDir := createOpenCodeWorkspace(t, provider)
			server, manager, _ := newCodexIntegrationServer(t)
			defer server.Close()

			ws := createWorkspace(t, server, repoDir)
			session := startSession(t, server, StartSessionRequest{
				Provider:    provider,
				WorkspaceID: ws.ID,
				Text:        "Say \"ok\" and nothing else.",
			})
			if session.ID == "" {
				t.Fatalf("session id missing")
			}

			waitForHistoryItemsClaude(t, server, manager, session.ID, openCodeIntegrationTimeout(provider))
			sendOpenCodeMessage(t, server, session.ID, "Say \"ok\" again.")
			waitForHistoryItemsClaude(t, server, manager, session.ID, openCodeIntegrationTimeout(provider))

			history := historySession(t, server, session.ID)
			if !historyHasAgentText(history.Items, "ok") {
				t.Fatalf("agent reply missing\n%s", sessionDiagnostics(manager, session.ID))
			}
		})
	}
}

func TestOpenCodeItemsStream(t *testing.T) {
	for _, provider := range integrationOpenCodeProviders() {
		t.Run(provider, func(t *testing.T) {
			requireOpenCodeIntegration(t, provider)

			repoDir := createOpenCodeWorkspace(t, provider)
			server, manager, _ := newCodexIntegrationServer(t)
			defer server.Close()

			ws := createWorkspace(t, server, repoDir)
			session := startSession(t, server, StartSessionRequest{
				Provider:    provider,
				WorkspaceID: ws.ID,
				Text:        "Say \"ok\" and nothing else.",
			})

			stream, closeFn := openSSE(t, server, "/v1/sessions/"+session.ID+"/items?follow=1&lines=100")
			defer closeFn()

			data, ok := waitForSSEData(stream, 30*time.Second)
			if !ok {
				t.Fatalf("timeout waiting for items stream event\n%s", sessionDiagnostics(manager, session.ID))
			}

			var item map[string]any
			if err := json.Unmarshal([]byte(data), &item); err != nil {
				t.Fatalf("decode item: %v", err)
			}
			if typ, _ := item["type"].(string); typ == "" {
				t.Fatalf("expected item type to be set")
			}

			sendOpenCodeMessage(t, server, session.ID, "Say \"ok\" again.")
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
			t.Fatalf("timeout waiting for agent reply on items stream\n%s", sessionDiagnostics(manager, session.ID))
		})
	}
}

func TestOpenCodeEventsStream(t *testing.T) {
	for _, provider := range integrationOpenCodeProviders() {
		t.Run(provider, func(t *testing.T) {
			requireOpenCodeIntegration(t, provider)

			repoDir := createOpenCodeWorkspace(t, provider)
			server, manager, _ := newCodexIntegrationServer(t)
			defer server.Close()

			ws := createWorkspace(t, server, repoDir)
			session := startSession(t, server, StartSessionRequest{
				Provider:    provider,
				WorkspaceID: ws.ID,
				Text:        "Say \"ok\" and nothing else.",
			})

			stream, closeFn := openSSE(t, server, "/v1/sessions/"+session.ID+"/events?follow=1")
			defer closeFn()

			sendOpenCodeMessage(t, server, session.ID, "Say \"ok\" again.")
			events := collectEvents(stream, 45*time.Second)
			if len(events) == 0 {
				t.Fatalf("expected events from SSE stream\n%s", sessionDiagnostics(manager, session.ID))
			}

			found := false
			for _, event := range events {
				switch event.Method {
				case "turn/started", "item/agentMessage/delta", "turn/completed", "error":
					found = true
				}
			}
			if !found {
				methods := make([]string, 0, len(events))
				for _, event := range events {
					methods = append(methods, event.Method)
				}
				t.Fatalf("expected mapped open code event methods, got=%v\n%s", methods, sessionDiagnostics(manager, session.ID))
			}
		})
	}
}

func integrationOpenCodeProviders() []string {
	return []string{"opencode", "kilocode"}
}

func requireOpenCodeIntegration(t *testing.T, provider string) {
	t.Helper()
	var enabledEnv string
	switch providers.Normalize(provider) {
	case "kilocode":
		enabledEnv = kilocodeIntegrationEnv
	default:
		enabledEnv = opencodeIntegrationEnv
	}
	if integrationEnvDisabled(enabledEnv) {
		t.Skipf("%s disables %s integration tests", enabledEnv, provider)
	}
	if _, ok := providers.Lookup(provider); !ok {
		t.Fatalf("%s provider not registered", provider)
	}
	cfg := resolveOpenCodeClientConfig(provider, loadCoreConfigOrDefault())
	if _, err := newOpenCodeClient(cfg); err != nil {
		t.Fatalf("%s client not configured: %v (set %s=disabled to skip)", provider, err, enabledEnv)
	}
}

func openCodeIntegrationTimeout(provider string) time.Duration {
	env := "ARCHON_OPENCODE_TIMEOUT"
	if providers.Normalize(provider) == "kilocode" {
		env = "ARCHON_KILOCODE_TIMEOUT"
	}
	if raw := strings.TrimSpace(os.Getenv(env)); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			return parsed
		}
	}
	return 2 * time.Minute
}

func createOpenCodeWorkspace(t *testing.T, provider string) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), provider+"-repo")
	if err := os.MkdirAll(repoDir, 0o700); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	return repoDir
}

func sendOpenCodeMessage(t *testing.T, server *httptest.Server, sessionID, text string) {
	t.Helper()
	status, body, _ := sendMessageOnce(server, sessionID, text)
	if status != http.StatusOK {
		t.Fatalf("send failed status=%d body=%s", status, body)
	}
}
