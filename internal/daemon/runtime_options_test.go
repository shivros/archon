package daemon

import (
	"testing"

	"control/internal/types"
)

func TestCodexProviderOptionCatalogFromModels(t *testing.T) {
	t.Parallel()

	catalog := codexProviderOptionCatalogFromModels([]codexModelSummary{
		{
			ID:                     "gpt-5.2-codex",
			Model:                  "gpt-5.2-codex",
			DefaultReasoningEffort: "medium",
			IsDefault:              true,
			ReasoningEffort: []codexReasoningEffortDef{
				{Effort: "low"},
				{Effort: "high"},
			},
		},
		{
			ID:                     "gpt-5.3-codex",
			Model:                  "gpt-5.3-codex",
			DefaultReasoningEffort: "high",
			ReasoningEffort: []codexReasoningEffortDef{
				{Effort: "extra-high"},
			},
		},
	})
	if catalog == nil {
		t.Fatalf("expected catalog")
	}
	if len(catalog.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(catalog.Models))
	}
	if catalog.Models[0] != "gpt-5.2-codex" {
		t.Fatalf("expected default model first, got %q", catalog.Models[0])
	}
	if catalog.Defaults.Model != "gpt-5.2-codex" {
		t.Fatalf("expected default model from dynamic list, got %q", catalog.Defaults.Model)
	}
	if catalog.Defaults.Reasoning != types.ReasoningMedium {
		t.Fatalf("expected default reasoning medium, got %q", catalog.Defaults.Reasoning)
	}
	if len(catalog.ReasoningLevels) < 3 {
		t.Fatalf("expected merged reasoning levels, got %v", catalog.ReasoningLevels)
	}
	levels, ok := modelReasoningLevelsFor(catalog, "gpt-5.2-codex")
	if !ok || len(levels) == 0 {
		t.Fatalf("expected model-specific reasoning levels for gpt-5.2-codex")
	}
	defaultLevel, ok := modelDefaultReasoningFor(catalog, "gpt-5.2-codex")
	if !ok || defaultLevel != types.ReasoningMedium {
		t.Fatalf("expected model default reasoning medium, got %q", defaultLevel)
	}
}

func TestResolveRuntimeOptionsAllowsUnknownCodexModel(t *testing.T) {
	t.Parallel()

	options, err := resolveRuntimeOptions("codex", nil, &types.SessionRuntimeOptions{
		Model: "gpt-9-codex-preview",
	}, false)
	if err != nil {
		t.Fatalf("resolveRuntimeOptions codex: %v", err)
	}
	if options == nil || options.Model != "gpt-9-codex-preview" {
		t.Fatalf("expected unknown codex model to be accepted")
	}
}

func TestResolveRuntimeOptionsValidatesReasoningByModel(t *testing.T) {
	t.Parallel()

	options, err := resolveRuntimeOptions("codex", nil, &types.SessionRuntimeOptions{
		Model:     "gpt-5.2-codex",
		Reasoning: types.ReasoningHigh,
	}, true)
	if err != nil {
		t.Fatalf("resolveRuntimeOptions valid model reasoning: %v", err)
	}
	if options == nil || options.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected high reasoning to pass for model")
	}

	_, err = resolveRuntimeOptions("codex", nil, &types.SessionRuntimeOptions{
		Model:     "gpt-5.2-codex",
		Reasoning: "turbo",
	}, true)
	if err == nil {
		t.Fatalf("expected invalid reasoning for model to fail")
	}
}

func TestCodexProviderOptionCatalogFromModelsFallsBackWhenOnlyDefaultEffortProvided(t *testing.T) {
	t.Parallel()

	catalog := codexProviderOptionCatalogFromModels([]codexModelSummary{
		{
			ID:                     "gpt-5.2-codex",
			Model:                  "gpt-5.2-codex",
			DefaultReasoningEffort: "medium",
			IsDefault:              true,
		},
	})
	if catalog == nil {
		t.Fatalf("expected catalog")
	}
	if len(catalog.ReasoningLevels) < 3 {
		t.Fatalf("expected fallback provider reasoning levels, got %v", catalog.ReasoningLevels)
	}
	levels, ok := modelReasoningLevelsFor(catalog, "gpt-5.2-codex")
	if !ok || len(levels) < 3 {
		t.Fatalf("expected fallback model reasoning levels, got %v", levels)
	}
}

func TestClaudeProviderOptionCatalogIncludesModelAndAccess(t *testing.T) {
	t.Parallel()

	catalog := providerOptionCatalog("claude")
	if catalog == nil {
		t.Fatalf("expected catalog")
	}
	if catalog.Provider != "claude" {
		t.Fatalf("expected claude provider, got %q", catalog.Provider)
	}
	if len(catalog.Models) == 0 {
		t.Fatalf("expected non-empty claude model list")
	}
	if len(catalog.AccessLevels) == 0 {
		t.Fatalf("expected non-empty claude access levels")
	}
	if catalog.Defaults.Access != types.AccessOnRequest {
		t.Fatalf("expected on_request access default, got %q", catalog.Defaults.Access)
	}
}

func TestResolveRuntimeOptionsAllowsUnknownClaudeModel(t *testing.T) {
	t.Parallel()

	options, err := resolveRuntimeOptions("claude", nil, &types.SessionRuntimeOptions{
		Model: "claude-sonnet-5-20260101",
	}, false)
	if err != nil {
		t.Fatalf("resolveRuntimeOptions claude: %v", err)
	}
	if options == nil || options.Model != "claude-sonnet-5-20260101" {
		t.Fatalf("expected unknown claude model to be accepted")
	}
}

func TestResolveRuntimeOptionsValidatesClaudeAccess(t *testing.T) {
	t.Parallel()

	options, err := resolveRuntimeOptions("claude", nil, &types.SessionRuntimeOptions{
		Access: types.AccessFull,
	}, true)
	if err != nil {
		t.Fatalf("resolveRuntimeOptions claude access: %v", err)
	}
	if options == nil || options.Access != types.AccessFull {
		t.Fatalf("expected full access to pass for claude")
	}

	_, err = resolveRuntimeOptions("claude", nil, &types.SessionRuntimeOptions{
		Access: "always",
	}, true)
	if err == nil {
		t.Fatalf("expected invalid access level to fail")
	}
}
