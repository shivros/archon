package daemon

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

const geminiIntegrationEnv = "ARCHON_GEMINI_INTEGRATION"

func requireGeminiIntegration(t *testing.T) {
	t.Helper()
	if integrationEnvDisabled(geminiIntegrationEnv) {
		t.Skipf("%s disables gemini integration tests", geminiIntegrationEnv)
	}
	if _, ok := providers.Lookup("gemini"); !ok {
		t.Fatal("gemini provider not registered")
	}
	cmdName := resolveGeminiCommand()
	if cmdName == "" {
		t.Skip("gemini binary not found in PATH")
	}
}

func resolveGeminiCommand() string {
	if cmd := os.Getenv("ARCHON_GEMINI_CMD"); cmd != "" {
		// Validate the override path exists.
		if _, err := exec.LookPath(cmd); err == nil {
			return cmd
		}
		// Return it anyway — the provider may resolve it differently.
		return cmd
	}
	for _, candidate := range []string{"gemini"} {
		if p, err := exec.LookPath(candidate); err == nil {
			return p
		}
	}
	return ""
}

func geminiIntegrationTimeout() time.Duration {
	if raw := os.Getenv("ARCHON_GEMINI_TIMEOUT"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil {
			return d
		}
	}
	return 3 * time.Minute
}

func geminiIntegrationSetup(t *testing.T) (string, *types.SessionRuntimeOptions) {
	t.Helper()
	repoDir := createGeminiWorkspace(t)
	model := resolveGeminiModel()
	return repoDir, &types.SessionRuntimeOptions{
		Model: model,
	}
}

func createGeminiWorkspace(t *testing.T) string {
	t.Helper()
	repoDir, err := os.MkdirTemp("", "gemini-repo-*")
	if err != nil {
		t.Fatalf("mkdir temp repo: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(repoDir)
	})
	return repoDir
}

func resolveGeminiModel() string {
	if m := os.Getenv("ARCHON_GEMINI_MODEL"); m != "" {
		return m
	}
	return ""
}
