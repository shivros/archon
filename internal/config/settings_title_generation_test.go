package config

import "testing"

func TestCoreConfigTitleGenerationDefaults(t *testing.T) {
	cfg := DefaultCoreConfig()
	if got := cfg.TitleGenerationProvider(); got != "" {
		t.Fatalf("expected empty provider by default, got %q", got)
	}
	if got := cfg.TitleGenerationModel(); got != "openrouter/auto" {
		t.Fatalf("expected default model openrouter/auto, got %q", got)
	}
	if got := cfg.TitleGenerationTimeoutSeconds(); got != 10 {
		t.Fatalf("expected default timeout seconds 10, got %d", got)
	}
	if got := cfg.TitleGenerationOpenRouterAPIKeyEnv(); got != "OPENROUTER_API_KEY" {
		t.Fatalf("expected default api_key_env OPENROUTER_API_KEY, got %q", got)
	}
	if got := cfg.TitleGenerationOpenRouterBaseURL(); got != "https://openrouter.ai/api/v1" {
		t.Fatalf("expected default OpenRouter base URL, got %q", got)
	}
}

func TestCoreConfigTitleGenerationNormalization(t *testing.T) {
	cfg := DefaultCoreConfig()
	cfg.TitleGeneration.Provider = " OpenRouter "
	cfg.TitleGeneration.Model = " openrouter/google/gemini-2.5-flash "
	cfg.TitleGeneration.TimeoutSeconds = 21
	cfg.TitleGeneration.OpenRouter.APIKey = " abc "
	cfg.TitleGeneration.OpenRouter.APIKeyEnv = " ARCHON_OPENROUTER_KEY "
	cfg.TitleGeneration.OpenRouter.BaseURL = " https://example.com/router/ "

	if got := cfg.TitleGenerationProvider(); got != "openrouter" {
		t.Fatalf("expected normalized provider openrouter, got %q", got)
	}
	if got := cfg.TitleGenerationModel(); got != "openrouter/google/gemini-2.5-flash" {
		t.Fatalf("expected trimmed model, got %q", got)
	}
	if got := cfg.TitleGenerationTimeoutSeconds(); got != 21 {
		t.Fatalf("expected configured timeout, got %d", got)
	}
	if got := cfg.TitleGenerationOpenRouterAPIKey(); got != "abc" {
		t.Fatalf("expected trimmed API key, got %q", got)
	}
	if got := cfg.TitleGenerationOpenRouterAPIKeyEnv(); got != "ARCHON_OPENROUTER_KEY" {
		t.Fatalf("expected trimmed api key env, got %q", got)
	}
	if got := cfg.TitleGenerationOpenRouterBaseURL(); got != "https://example.com/router" {
		t.Fatalf("expected trimmed base URL, got %q", got)
	}
}

func TestCoreConfigTitleGenerationUnsupportedProviderDisabled(t *testing.T) {
	cfg := DefaultCoreConfig()
	cfg.TitleGeneration.Provider = "not-a-provider"
	if got := cfg.TitleGenerationProvider(); got != "" {
		t.Fatalf("expected unsupported provider to be disabled, got %q", got)
	}
}
