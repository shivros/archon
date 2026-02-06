package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	claudeIntegrationEnv  = "CONTROL_CLAUDE_INTEGRATION"
	claudeIntegrationSkip = "CONTROL_CLAUDE_SKIP"
)

// These tests require the real Claude CLI to be installed and authenticated.

func TestAPIClaudeSessionFlow(t *testing.T) {
	requireClaudeIntegration(t)

	repoDir := createClaudeWorkspace(t)
	server, manager, _ := newCodexIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)
	session := startSession(t, server, StartSessionRequest{
		Provider:    "claude",
		WorkspaceID: ws.ID,
		Text:        "Say \"ok\" and nothing else.",
	})
	if session.ID == "" {
		t.Fatalf("session id missing")
	}

	waitForHistoryItemsClaude(t, server, manager, session.ID, claudeIntegrationTimeout())

	sendClaudeMessage(t, server, session.ID, "Say \"ok\" again.")
	waitForHistoryItemsClaude(t, server, manager, session.ID, claudeIntegrationTimeout())
}

func TestClaudeItemsStream(t *testing.T) {
	requireClaudeIntegration(t)

	repoDir := createClaudeWorkspace(t)
	server, manager, _ := newCodexIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)
	session := startSession(t, server, StartSessionRequest{
		Provider:    "claude",
		WorkspaceID: ws.ID,
		Text:        "Say \"ok\" and nothing else.",
	})

	stream, closeFn := openSSE(t, server, "/v1/sessions/"+session.ID+"/items?follow=1&lines=50")
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
}

func sendClaudeMessage(t *testing.T, server *httptest.Server, sessionID, text string) {
	t.Helper()
	status, body, _ := sendMessageOnce(server, sessionID, text)
	if status != http.StatusOK {
		t.Fatalf("send failed status=%d body=%s", status, body)
	}
}

func requireClaudeIntegration(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(claudeIntegrationSkip)) != "" {
		t.Skipf("%s set", claudeIntegrationSkip)
	}
	if os.Getenv(claudeIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run Claude integration tests", claudeIntegrationEnv)
	}
	if _, err := findCommand("CONTROL_CLAUDE_CMD", "claude"); err != nil {
		t.Fatalf("claude command not found: %v", err)
	}
}

func claudeIntegrationTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("CONTROL_CLAUDE_TIMEOUT")); raw != "" {
		if secs, err := time.ParseDuration(raw); err == nil {
			return secs
		}
	}
	return 2 * time.Minute
}

func createClaudeWorkspace(t *testing.T) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o700); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	return repoDir
}

func waitForHistoryItemsClaude(t *testing.T, server *httptest.Server, manager *SessionManager, sessionID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		history := historySession(t, server, sessionID)
		if len(history.Items) > 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for history items\n%s", sessionDiagnostics(manager, sessionID))
}

func sessionDiagnostics(manager *SessionManager, sessionID string) string {
	if manager == nil || sessionID == "" {
		return "no diagnostics available"
	}
	sessionDir := filepath.Join(manager.baseDir, sessionID)
	itemsPath := filepath.Join(sessionDir, "items.jsonl")
	stdoutPath := filepath.Join(sessionDir, "stdout.log")
	stderrPath := filepath.Join(sessionDir, "stderr.log")
	itemsLines, _, _ := tailLines(itemsPath, 40)
	stdoutLines, _, _ := tailLines(stdoutPath, 40)
	stderrLines, _, _ := tailLines(stderrPath, 40)
	return fmt.Sprintf("items.jsonl:\n%s\n\nstdout.log:\n%s\n\nstderr.log:\n%s\n",
		strings.Join(itemsLines, "\n"),
		strings.Join(stdoutLines, "\n"),
		strings.Join(stderrLines, "\n"),
	)
}
