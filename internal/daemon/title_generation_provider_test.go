package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type stubTitleGenerationConfig struct {
	provider   string
	model      string
	timeout    int
	apiKey     string
	apiKeyEnv  string
	openRouter string
}

func (s stubTitleGenerationConfig) TitleGenerationProvider() string            { return s.provider }
func (s stubTitleGenerationConfig) TitleGenerationModel() string               { return s.model }
func (s stubTitleGenerationConfig) TitleGenerationTimeoutSeconds() int         { return s.timeout }
func (s stubTitleGenerationConfig) TitleGenerationOpenRouterAPIKey() string    { return s.apiKey }
func (s stubTitleGenerationConfig) TitleGenerationOpenRouterAPIKeyEnv() string { return s.apiKeyEnv }
func (s stubTitleGenerationConfig) TitleGenerationOpenRouterBaseURL() string   { return s.openRouter }

func TestTitleProviderBridgeBuildOpenRouterFromEnvKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	bridge := newTitleProviderBridge(nil)
	generator, ok, err := bridge.Build(stubTitleGenerationConfig{
		provider:   "openrouter",
		model:      "openrouter/auto",
		timeout:    7,
		apiKeyEnv:  "OPENROUTER_API_KEY",
		openRouter: "https://openrouter.ai/api/v1",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !ok {
		t.Fatalf("expected provider to be configured")
	}
	impl, ok := generator.(*openRouterTitleGenerator)
	if !ok {
		t.Fatalf("expected openRouterTitleGenerator, got %T", generator)
	}
	if impl.apiKey != "test-key" {
		t.Fatalf("expected API key from env, got %q", impl.apiKey)
	}
	if impl.client == nil || impl.client.Timeout != 7*time.Second {
		t.Fatalf("expected timeout 7s, got %#v", impl.client)
	}
}

func TestTitleProviderBridgeBuildMissingKeyDisables(t *testing.T) {
	bridge := newTitleProviderBridge(nil)
	generator, ok, err := bridge.Build(stubTitleGenerationConfig{provider: "openrouter"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ok {
		t.Fatalf("expected missing key to disable provider, got generator %T", generator)
	}
	if generator != nil {
		t.Fatalf("expected nil generator when provider disabled")
	}
}

func TestTitleProviderBridgeBuildRejectsUnsupportedProvider(t *testing.T) {
	bridge := newTitleProviderBridge(nil)
	_, ok, err := bridge.Build(stubTitleGenerationConfig{provider: "unsupported"})
	if ok {
		t.Fatalf("expected unsupported provider to be disabled")
	}
	if err == nil {
		t.Fatalf("expected error for unsupported provider")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unsupported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenRouterTitleGeneratorGenerateTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}
		if got := strings.TrimSpace(r.URL.Path); got != "/chat/completions" {
			t.Fatalf("expected chat completions path, got %q", got)
		}
		if got := strings.TrimSpace(r.Header.Get("Authorization")); got != "Bearer test-key" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Fix Guided Workflow Retry Loop"}}]}`))
	}))
	defer server.Close()

	generator := &openRouterTitleGenerator{
		baseURL: server.URL,
		apiKey:  "test-key",
		model:   "openrouter/auto",
		client:  &http.Client{Timeout: 3 * time.Second},
	}
	title, err := generator.GenerateTitle(context.Background(), "Prompt body")
	if err != nil {
		t.Fatalf("GenerateTitle: %v", err)
	}
	if title != "Fix Guided Workflow Retry Loop" {
		t.Fatalf("unexpected generated title: %q", title)
	}
}

func TestOpenRouterTitleGeneratorRejectsReasoningOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null,"reasoning":"Internal thoughts here"}}]}`))
	}))
	defer server.Close()
	generator := &openRouterTitleGenerator{
		baseURL: server.URL,
		apiKey:  "test-key",
		model:   "openrouter/auto",
		client:  &http.Client{Timeout: 3 * time.Second},
	}
	_, err := generator.GenerateTitle(context.Background(), "Prompt body")
	if err == nil {
		t.Fatalf("expected error for reasoning-only response")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "empty title") {
		t.Fatalf("expected empty-title error, got %q", err.Error())
	}
}

func TestOpenRouterTitleGeneratorRedactsErrorBody(t *testing.T) {
	const secret = "top-secret-response-fragment"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(secret))
	}))
	defer server.Close()
	generator := &openRouterTitleGenerator{
		baseURL: server.URL,
		apiKey:  "test-key",
		model:   "openrouter/auto",
		client:  &http.Client{Timeout: 3 * time.Second},
	}
	_, err := generator.GenerateTitle(context.Background(), "Prompt body")
	if err == nil {
		t.Fatalf("expected request failure")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked response body content: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected status in error, got %q", err.Error())
	}
}

func TestOpenRouterTitleGeneratorInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{not-json`))
	}))
	defer server.Close()
	generator := &openRouterTitleGenerator{
		baseURL: server.URL,
		apiKey:  "test-key",
		model:   "openrouter/auto",
		client:  &http.Client{Timeout: 3 * time.Second},
	}
	_, err := generator.GenerateTitle(context.Background(), "Prompt body")
	if err == nil {
		t.Fatalf("expected invalid response error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractOpenRouterContentValueVariants(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name:  "string",
			input: "  title  ",
			want:  "title",
		},
		{
			name: "array of maps",
			input: []any{
				map[string]any{"text": "first"},
				map[string]any{"content": "second"},
			},
			want: "first second",
		},
		{
			name: "nested value key",
			input: map[string]any{
				"value": map[string]any{"text": "inner"},
			},
			want: "inner",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractOpenRouterContentValue(tc.input); got != tc.want {
				t.Fatalf("extractOpenRouterContentValue() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildTitleGeneratorFromCoreConfig(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	cfg := loadCoreConfigOrDefault()
	cfg.TitleGeneration.Provider = "openrouter"
	cfg.TitleGeneration.OpenRouter.APIKeyEnv = "OPENROUTER_API_KEY"
	generator, err := newTitleGeneratorFromCoreConfig(cfg, nil)
	if err != nil {
		t.Fatalf("newTitleGeneratorFromCoreConfig: %v", err)
	}
	if generator == nil {
		t.Fatalf("expected non-nil generator")
	}
}

func TestBuildTitleGeneratorFromCoreConfigDisabled(t *testing.T) {
	_ = os.Unsetenv("OPENROUTER_API_KEY")
	cfg := loadCoreConfigOrDefault()
	generator, err := newTitleGeneratorFromCoreConfig(cfg, nil)
	if err != nil {
		t.Fatalf("newTitleGeneratorFromCoreConfig: %v", err)
	}
	if generator != nil {
		t.Fatalf("expected nil generator when feature is disabled")
	}
}
