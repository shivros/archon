package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

type openCodeModelCatalogReader interface {
	ListModels(ctx context.Context) (*openCodeModelCatalog, error)
}

type openCodeCatalogReaderFactory func(provider string) (openCodeModelCatalogReader, error)

type openCodeIntegrationModelSelector struct {
	configuredModel  func(provider string) string
	catalogReaderFor openCodeCatalogReaderFactory
	catalogTimeout   time.Duration
}

func newOpenCodeIntegrationModelSelector() openCodeIntegrationModelSelector {
	return openCodeIntegrationModelSelector{
		configuredModel:  openCodeConfiguredIntegrationModel,
		catalogReaderFor: defaultOpenCodeCatalogReaderFactory,
		catalogTimeout:   8 * time.Second,
	}
}

func defaultOpenCodeCatalogReaderFactory(provider string) (openCodeModelCatalogReader, error) {
	cfg := resolveOpenCodeClientConfig(provider, loadCoreConfigOrDefault())
	client, err := newOpenCodeClient(cfg)
	if err != nil {
		return nil, err
	}
	// Integration setup should be resilient when the local OpenCode/KiloCode
	// server is not already running. Start it on-demand so model discovery can
	// select a valid explicit runtime model.
	startedBaseURL, startErr := maybeAutoStartOpenCodeServer(provider, client.baseURL, client.token, nil)
	if startErr == nil && strings.TrimSpace(startedBaseURL) != "" && !strings.EqualFold(strings.TrimSpace(startedBaseURL), strings.TrimSpace(client.baseURL)) {
		if switched, switchErr := cloneOpenCodeClientWithBaseURL(client, startedBaseURL); switchErr == nil {
			client = switched
		}
	}
	return client, nil
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

func openCodeConfiguredIntegrationModel(provider string) string {
	normalized := providers.Normalize(provider)
	switch normalized {
	case "kilocode":
		if model := strings.TrimSpace(os.Getenv("ARCHON_KILOCODE_MODEL")); model != "" {
			return model
		}
	case "opencode":
		if model := strings.TrimSpace(os.Getenv("ARCHON_OPENCODE_MODEL")); model != "" {
			return model
		}
	}
	if fallback := strings.TrimSpace(openCodeIntegrationFallbackModels[normalized]); fallback != "" {
		return fallback
	}
	return ""
}

func openCodeIntegrationModel(t *testing.T, provider string) string {
	t.Helper()
	return newOpenCodeIntegrationModelSelector().SelectModel(t, provider)
}

func (s openCodeIntegrationModelSelector) SelectModel(t *testing.T, provider string) string {
	t.Helper()

	preferred := ""
	if s.configuredModel != nil {
		preferred = strings.TrimSpace(s.configuredModel(provider))
	}
	if s.catalogReaderFor == nil {
		return preferred
	}

	reader, err := s.catalogReaderFor(provider)
	if err != nil {
		return preferred
	}

	timeout := s.catalogTimeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	catalog, err := reader.ListModels(ctx)
	if err != nil || catalog == nil {
		return preferred
	}

	if preferred != "" && containsFolded(catalog.Models, preferred) {
		return preferred
	}
	if preferred != "" && strings.EqualFold(strings.TrimSpace(catalog.DefaultModel), preferred) {
		return preferred
	}
	if model := strings.TrimSpace(catalog.DefaultModel); model != "" {
		return model
	}
	for _, candidate := range catalog.Models {
		if model := strings.TrimSpace(candidate); model != "" {
			return model
		}
	}
	return preferred
}

type stubOpenCodeModelCatalogReader struct {
	catalog *openCodeModelCatalog
	err     error
	called  bool
}

func (s *stubOpenCodeModelCatalogReader) ListModels(_ context.Context) (*openCodeModelCatalog, error) {
	s.called = true
	return s.catalog, s.err
}

func TestOpenCodeIntegrationModelSelectorSelectModel(t *testing.T) {
	makeFactory := func(reader openCodeModelCatalogReader, err error) openCodeCatalogReaderFactory {
		return func(string) (openCodeModelCatalogReader, error) {
			if err != nil {
				return nil, err
			}
			return reader, nil
		}
	}

	t.Run("returns configured when reader factory missing", func(t *testing.T) {
		selector := openCodeIntegrationModelSelector{
			configuredModel: func(string) string { return "openrouter/google/gemini-2.5-flash" },
		}
		got := selector.SelectModel(t, "opencode")
		if got != "openrouter/google/gemini-2.5-flash" {
			t.Fatalf("expected configured model, got %q", got)
		}
	})

	t.Run("uses deterministic provider fallback when env/config are empty", func(t *testing.T) {
		if got := openCodeConfiguredIntegrationModel("opencode"); got != "openrouter/google/gemini-2.5-flash" {
			t.Fatalf("expected opencode fallback model, got %q", got)
		}
		if got := openCodeConfiguredIntegrationModel("kilocode"); got != "moonshotai/kimi-k2.5" {
			t.Fatalf("expected kilocode fallback model, got %q", got)
		}
	})

	t.Run("returns configured when factory errors", func(t *testing.T) {
		selector := openCodeIntegrationModelSelector{
			configuredModel:  func(string) string { return "openrouter/google/gemini-2.5-flash" },
			catalogReaderFor: makeFactory(nil, context.DeadlineExceeded),
		}
		got := selector.SelectModel(t, "opencode")
		if got != "openrouter/google/gemini-2.5-flash" {
			t.Fatalf("expected configured model, got %q", got)
		}
	})

	t.Run("returns configured when catalog lookup fails", func(t *testing.T) {
		reader := &stubOpenCodeModelCatalogReader{err: context.DeadlineExceeded}
		selector := openCodeIntegrationModelSelector{
			configuredModel:  func(string) string { return "openrouter/google/gemini-2.5-flash" },
			catalogReaderFor: makeFactory(reader, nil),
		}
		got := selector.SelectModel(t, "opencode")
		if !reader.called {
			t.Fatalf("expected catalog reader to be called")
		}
		if got != "openrouter/google/gemini-2.5-flash" {
			t.Fatalf("expected configured model, got %q", got)
		}
	})

	t.Run("keeps configured when present in catalog models", func(t *testing.T) {
		reader := &stubOpenCodeModelCatalogReader{
			catalog: &openCodeModelCatalog{
				Models: []string{
					"openrouter/google/gemini-2.5-flash",
					"openrouter/anthropic/claude-sonnet-4",
				},
				DefaultModel: "openrouter/anthropic/claude-sonnet-4",
			},
		}
		selector := openCodeIntegrationModelSelector{
			configuredModel:  func(string) string { return "openrouter/google/gemini-2.5-flash" },
			catalogReaderFor: makeFactory(reader, nil),
		}
		got := selector.SelectModel(t, "opencode")
		if got != "openrouter/google/gemini-2.5-flash" {
			t.Fatalf("expected configured model, got %q", got)
		}
	})

	t.Run("uses catalog default when configured missing from catalog", func(t *testing.T) {
		reader := &stubOpenCodeModelCatalogReader{
			catalog: &openCodeModelCatalog{
				Models:       []string{"openrouter/anthropic/claude-sonnet-4"},
				DefaultModel: "openrouter/anthropic/claude-sonnet-4",
			},
		}
		selector := openCodeIntegrationModelSelector{
			configuredModel:  func(string) string { return "openrouter/google/gemini-2.5-flash" },
			catalogReaderFor: makeFactory(reader, nil),
		}
		got := selector.SelectModel(t, "opencode")
		if got != "openrouter/anthropic/claude-sonnet-4" {
			t.Fatalf("expected catalog default model, got %q", got)
		}
	})

	t.Run("uses first catalog model when default missing", func(t *testing.T) {
		reader := &stubOpenCodeModelCatalogReader{
			catalog: &openCodeModelCatalog{
				Models: []string{"openrouter/anthropic/claude-sonnet-4"},
			},
		}
		selector := openCodeIntegrationModelSelector{
			configuredModel:  func(string) string { return "" },
			catalogReaderFor: makeFactory(reader, nil),
			catalogTimeout:   0, // Exercise default-timeout branch.
		}
		got := selector.SelectModel(t, "opencode")
		if got != "openrouter/anthropic/claude-sonnet-4" {
			t.Fatalf("expected first catalog model, got %q", got)
		}
	})

	t.Run("returns configured when catalog empty", func(t *testing.T) {
		reader := &stubOpenCodeModelCatalogReader{
			catalog: &openCodeModelCatalog{},
		}
		selector := openCodeIntegrationModelSelector{
			configuredModel:  func(string) string { return "openrouter/google/gemini-2.5-flash" },
			catalogReaderFor: makeFactory(reader, nil),
		}
		got := selector.SelectModel(t, "opencode")
		if got != "openrouter/google/gemini-2.5-flash" {
			t.Fatalf("expected configured model, got %q", got)
		}
	})
}

func openCodeIntegrationSetup(t *testing.T, provider string) (string, *types.SessionRuntimeOptions) {
	t.Helper()
	model := openCodeIntegrationModel(t, provider)
	if model == "" {
		switch providers.Normalize(provider) {
		case "kilocode":
			t.Fatalf("no model configured for %s integration test (set ARCHON_KILOCODE_MODEL, provider default model in config, or expose provider catalog defaults)", provider)
		default:
			t.Fatalf("no model configured for %s integration test (set ARCHON_OPENCODE_MODEL, provider default model in config, or expose provider catalog defaults)", provider)
		}
	}
	opts := &types.SessionRuntimeOptions{Model: model}
	return createOpenCodeWorkspace(t, provider), opts
}

func createOpenCodeWorkspace(t *testing.T, provider string) string {
	t.Helper()
	repoDir, err := os.MkdirTemp("", provider+"-repo-*")
	if err != nil {
		t.Fatalf("mkdir temp repo: %v", err)
	}
	t.Cleanup(func() {
		const attempts = 5
		for i := 0; i < attempts; i++ {
			if err := os.RemoveAll(repoDir); err == nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		// OpenCode/Kilo integration can leave transient open handles during teardown.
		// Avoid failing tests purely due best-effort temp cleanup races.
		t.Logf("warning: best-effort cleanup could not remove %s", repoDir)
	})
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
