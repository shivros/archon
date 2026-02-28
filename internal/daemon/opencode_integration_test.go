package daemon

import (
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
