package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"control/internal/config"
)

func TestOpenRouterTitleGeneratorIntegration(t *testing.T) {
	apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}
	candidates := []string{}
	appendCandidate := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range candidates {
			if strings.EqualFold(existing, value) {
				return
			}
		}
		candidates = append(candidates, value)
	}
	appendCandidate(os.Getenv("ARCHON_TITLE_GENERATION_INTEGRATION_MODEL"))
	appendCandidate(os.Getenv("ARCHON_OPENCODE_MODEL"))
	appendCandidate("openrouter/google/gemini-2.5-flash")
	appendCandidate("openrouter/openai/gpt-4o-mini")
	appendCandidate("openrouter/auto")

	var attempts []string
	for _, model := range candidates {
		cfg := config.DefaultCoreConfig()
		cfg.TitleGeneration.Provider = "openrouter"
		cfg.TitleGeneration.Model = model
		cfg.TitleGeneration.TimeoutSeconds = 20
		cfg.TitleGeneration.OpenRouter.APIKeyEnv = "OPENROUTER_API_KEY"
		generator, err := newTitleGeneratorFromCoreConfig(cfg, nil)
		if err != nil {
			t.Fatalf("newTitleGeneratorFromCoreConfig(%q): %v", model, err)
		}
		if generator == nil {
			t.Fatal("expected configured title generator")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		title, err := generator.GenerateTitle(ctx, "Investigate flaky OpenCode provider integration tests and improve retry observability")
		cancel()
		if err == nil {
			if strings.TrimSpace(title) == "" {
				t.Fatalf("expected non-empty generated title for model %q", model)
			}
			t.Logf("title generation succeeded with model %q", model)
			return
		}
		var providerErr *titleProviderError
		if errors.As(err, &providerErr) {
			attempts = append(attempts, fmt.Sprintf("%s (%s)", model, providerErr.Error()))
			continue
		}
		t.Fatalf("GenerateTitle(%q): %v", model, err)
	}
	t.Skipf("no OpenRouter model succeeded for title generation in this environment: %s", strings.Join(attempts, "; "))
}
