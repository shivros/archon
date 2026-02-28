package daemon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/providers"
)

const claudeIntegrationEnv = "ARCHON_CLAUDE_INTEGRATION"

func sendClaudeMessage(t *testing.T, server *httptest.Server, sessionID, text string) {
	t.Helper()
	status, body, _ := sendMessageOnce(server, sessionID, text)
	if status != http.StatusOK {
		t.Fatalf("send failed status=%d body=%s", status, body)
	}
}

func requireClaudeIntegration(t *testing.T) {
	t.Helper()
	if integrationEnvDisabled(claudeIntegrationEnv) {
		t.Skipf("%s disables Claude integration tests", claudeIntegrationEnv)
	}
	def, ok := providers.Lookup("claude")
	if !ok {
		t.Fatalf("claude provider not registered")
	}
	if _, err := resolveProviderCommandName(def, ""); err != nil {
		t.Fatalf("claude command not found: %v (set %s=disabled to skip)", err, claudeIntegrationEnv)
	}
}

func claudeIntegrationTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("ARCHON_CLAUDE_TIMEOUT")); raw != "" {
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

func waitForAgentReply(t *testing.T, server *httptest.Server, manager *SessionManager, sessionID, needle string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		history := historySession(t, server, sessionID)
		if historyHasAgentText(history.Items, needle) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for agent reply containing %q\n%s", needle, sessionDiagnostics(manager, sessionID))
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

func historyHasAgentText(items []map[string]any, needle string) bool {
	if len(items) == 0 || needle == "" {
		return false
	}
	needle = strings.ToLower(needle)
	for _, item := range items {
		if item == nil {
			continue
		}
		typ, _ := item["type"].(string)
		if typ != "agentMessage" && typ != "agentMessageDelta" && typ != "assistant" {
			continue
		}
		if text := extractHistoryText(item); text != "" {
			if strings.Contains(strings.ToLower(text), needle) {
				return true
			}
		}
	}
	return false
}

func extractHistoryText(item map[string]any) string {
	if item == nil {
		return ""
	}
	if text, _ := item["text"].(string); strings.TrimSpace(text) != "" {
		return text
	}
	content := []any{}
	if direct, ok := item["content"].([]any); ok {
		content = direct
	} else if message, ok := item["message"].(map[string]any); ok {
		if nested, ok := message["content"].([]any); ok {
			content = nested
		}
	}
	if len(content) == 0 {
		return ""
	}
	var parts []string
	for _, entry := range content {
		block, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if text, _ := block["text"].(string); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}
