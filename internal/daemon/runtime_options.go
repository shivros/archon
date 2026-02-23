package daemon

import (
	"fmt"
	"slices"
	"strings"

	"control/internal/providers"
	"control/internal/types"
)

func providerOptionCatalog(provider string) *types.ProviderOptionCatalog {
	name := providers.Normalize(provider)
	switch name {
	case "codex":
		return codexProviderOptionCatalog()
	case "claude":
		return claudeProviderOptionCatalog()
	default:
		return &types.ProviderOptionCatalog{Provider: name}
	}
}

func codexProviderOptionCatalog() *types.ProviderOptionCatalog {
	coreCfg := loadCoreConfigOrDefault()
	models := coreCfg.CodexModels()
	defaultModel := coreCfg.CodexDefaultModel()
	if !slices.Contains(models, defaultModel) {
		models = append([]string{defaultModel}, models...)
	}
	return &types.ProviderOptionCatalog{
		Provider: "codex",
		Models:   models,
		ReasoningLevels: []types.ReasoningLevel{
			types.ReasoningLow,
			types.ReasoningMedium,
			types.ReasoningHigh,
			types.ReasoningExtraHigh,
		},
		ModelReasoningLevels: map[string][]types.ReasoningLevel{
			"gpt-5.1-codex":     {types.ReasoningLow, types.ReasoningMedium, types.ReasoningHigh, types.ReasoningExtraHigh},
			"gpt-5.2-codex":     {types.ReasoningLow, types.ReasoningMedium, types.ReasoningHigh, types.ReasoningExtraHigh},
			"gpt-5.3-codex":     {types.ReasoningLow, types.ReasoningMedium, types.ReasoningHigh, types.ReasoningExtraHigh},
			"gpt-5.1-codex-max": {types.ReasoningLow, types.ReasoningMedium, types.ReasoningHigh, types.ReasoningExtraHigh},
		},
		ModelDefaultReasoning: map[string]types.ReasoningLevel{
			"gpt-5.1-codex":     types.ReasoningMedium,
			"gpt-5.2-codex":     types.ReasoningMedium,
			"gpt-5.3-codex":     types.ReasoningMedium,
			"gpt-5.1-codex-max": types.ReasoningMedium,
		},
		AccessLevels: []types.AccessLevel{
			types.AccessReadOnly,
			types.AccessOnRequest,
			types.AccessFull,
		},
		Defaults: types.SessionRuntimeOptions{
			Model:     defaultModel,
			Reasoning: types.ReasoningMedium,
			Access:    types.AccessOnRequest,
			Version:   1,
		},
	}
}

func codexProviderOptionCatalogFromModels(models []codexModelSummary) *types.ProviderOptionCatalog {
	base := codexProviderOptionCatalog()
	if len(models) == 0 {
		return base
	}
	modelIDs := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	reasoning := []types.ReasoningLevel{}
	reasonSeen := map[types.ReasoningLevel]struct{}{}
	modelReasoning := map[string][]types.ReasoningLevel{}
	modelReasonSeen := map[string]map[types.ReasoningLevel]struct{}{}
	modelDefaultReasoning := map[string]types.ReasoningLevel{}
	defaultModel := strings.TrimSpace(base.Defaults.Model)
	defaultReasoning := base.Defaults.Reasoning

	for _, item := range models {
		model := strings.TrimSpace(item.Model)
		if model == "" {
			model = strings.TrimSpace(item.ID)
		}
		if model != "" {
			if _, ok := seen[model]; !ok {
				modelIDs = append(modelIDs, model)
				seen[model] = struct{}{}
			}
			if item.IsDefault {
				defaultModel = model
			}
			if _, ok := modelReasonSeen[model]; !ok {
				modelReasonSeen[model] = map[types.ReasoningLevel]struct{}{}
			}
		}
		explicitEfforts := 0
		if level, ok := normalizeReasoningLevel(types.ReasoningLevel(item.DefaultReasoningEffort)); ok {
			if _, seenLevel := reasonSeen[level]; !seenLevel {
				reasoning = append(reasoning, level)
				reasonSeen[level] = struct{}{}
			}
			if model != "" {
				if _, seenLevel := modelReasonSeen[model][level]; !seenLevel {
					modelReasoning[model] = append(modelReasoning[model], level)
					modelReasonSeen[model][level] = struct{}{}
				}
				modelDefaultReasoning[model] = level
			}
			if item.IsDefault {
				defaultReasoning = level
			}
		}
		for _, effort := range item.ReasoningEffort {
			explicitEfforts++
			level, ok := normalizeReasoningLevel(types.ReasoningLevel(effort.Effort))
			if !ok {
				continue
			}
			if _, seenLevel := reasonSeen[level]; seenLevel {
				// keep going; we still may need model-specific mapping below.
			} else {
				reasoning = append(reasoning, level)
				reasonSeen[level] = struct{}{}
			}
			if model != "" {
				if _, ok := modelReasonSeen[model]; !ok {
					modelReasonSeen[model] = map[types.ReasoningLevel]struct{}{}
				}
				if _, seenLevel := modelReasonSeen[model][level]; !seenLevel {
					modelReasoning[model] = append(modelReasoning[model], level)
					modelReasonSeen[model][level] = struct{}{}
				}
			}
		}
		if model != "" && (len(modelReasoning[model]) == 0 || (explicitEfforts == 0 && len(modelReasoning[model]) <= 1)) && len(base.ReasoningLevels) > 0 {
			modelReasoning[model] = append([]types.ReasoningLevel{}, base.ReasoningLevels...)
			if _, ok := modelDefaultReasoning[model]; !ok {
				modelDefaultReasoning[model] = base.Defaults.Reasoning
			}
		}
	}

	if len(modelIDs) > 0 {
		base.Models = modelIDs
	}
	if len(reasoning) > 1 {
		base.ReasoningLevels = reasoning
	}
	if len(modelReasoning) > 0 {
		base.ModelReasoningLevels = modelReasoning
	}
	if len(modelDefaultReasoning) > 0 {
		base.ModelDefaultReasoning = modelDefaultReasoning
	}
	if strings.TrimSpace(defaultModel) != "" {
		base.Defaults.Model = defaultModel
	}
	if defaultReasoning != "" {
		base.Defaults.Reasoning = defaultReasoning
	}
	return base
}

func claudeProviderOptionCatalog() *types.ProviderOptionCatalog {
	coreCfg := loadCoreConfigOrDefault()
	models := coreCfg.ClaudeModels()
	defaultModel := coreCfg.ClaudeDefaultModel()
	if !slices.Contains(models, defaultModel) {
		models = append([]string{defaultModel}, models...)
	}
	return &types.ProviderOptionCatalog{
		Provider: "claude",
		Models:   models,
		AccessLevels: []types.AccessLevel{
			types.AccessReadOnly,
			types.AccessOnRequest,
			types.AccessFull,
		},
		Defaults: types.SessionRuntimeOptions{
			Model:   defaultModel,
			Access:  types.AccessOnRequest,
			Version: 1,
		},
	}
}

func resolveRuntimeOptions(provider string, base, patch *types.SessionRuntimeOptions, applyDefaults bool) (*types.SessionRuntimeOptions, error) {
	merged := types.MergeRuntimeOptions(base, patch)
	catalog := providerOptionCatalog(provider)
	if merged == nil {
		if !applyDefaults {
			return nil, nil
		}
		merged = &types.SessionRuntimeOptions{}
	}
	merged.Model = strings.TrimSpace(merged.Model)
	if merged.Reasoning != "" {
		normalized, ok := normalizeReasoningLevel(merged.Reasoning)
		if !ok {
			return nil, fmt.Errorf("invalid reasoning level: %s", merged.Reasoning)
		}
		merged.Reasoning = normalized
	}
	if merged.Access != "" {
		normalized, ok := normalizeAccessLevel(merged.Access)
		if !ok {
			return nil, fmt.Errorf("invalid access level: %s", merged.Access)
		}
		merged.Access = normalized
	}
	if applyDefaults {
		if merged.Model == "" {
			merged.Model = strings.TrimSpace(catalog.Defaults.Model)
		}
		if merged.Reasoning == "" {
			merged.Reasoning = catalog.Defaults.Reasoning
			if merged.Model != "" {
				if level, ok := modelDefaultReasoningFor(catalog, merged.Model); ok {
					merged.Reasoning = level
				}
			}
		}
		if merged.Access == "" {
			merged.Access = catalog.Defaults.Access
		}
	}
	normalizedProvider := providers.Normalize(provider)
	skipModelValidation := normalizedProvider == "codex" || normalizedProvider == "claude"
	if merged.Model != "" && len(catalog.Models) > 0 && !containsFolded(catalog.Models, merged.Model) && !skipModelValidation {
		return nil, fmt.Errorf("invalid model: %s", merged.Model)
	}
	if merged.Reasoning != "" {
		reasoningLevels := catalog.ReasoningLevels
		if merged.Model != "" {
			if modelLevels, ok := modelReasoningLevelsFor(catalog, merged.Model); ok && len(modelLevels) > 0 {
				reasoningLevels = modelLevels
			}
		}
		if len(reasoningLevels) > 0 && !slices.Contains(reasoningLevels, merged.Reasoning) {
			return nil, fmt.Errorf("invalid reasoning level for model %q: %s", merged.Model, merged.Reasoning)
		}
	}
	if merged.Access != "" && len(catalog.AccessLevels) > 0 && !slices.Contains(catalog.AccessLevels, merged.Access) {
		return nil, fmt.Errorf("invalid access level: %s", merged.Access)
	}
	if merged.Provider != nil && len(merged.Provider) == 0 {
		merged.Provider = nil
	}
	if merged.Version == 0 {
		merged.Version = 1
	}
	if !applyDefaults && merged.Model == "" && merged.Reasoning == "" && merged.Access == "" && len(merged.Provider) == 0 {
		return nil, nil
	}
	return merged, nil
}

func normalizeReasoningLevel(raw types.ReasoningLevel) (types.ReasoningLevel, bool) {
	return types.NormalizeReasoningLevel(raw)
}

func normalizeAccessLevel(raw types.AccessLevel) (types.AccessLevel, bool) {
	return types.NormalizeAccessLevel(raw)
}

func containsFolded(values []string, target string) bool {
	if strings.TrimSpace(target) == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func modelReasoningLevelsFor(catalog *types.ProviderOptionCatalog, model string) ([]types.ReasoningLevel, bool) {
	if catalog == nil || len(catalog.ModelReasoningLevels) == 0 || strings.TrimSpace(model) == "" {
		return nil, false
	}
	for key, levels := range catalog.ModelReasoningLevels {
		if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(model)) {
			return append([]types.ReasoningLevel{}, levels...), true
		}
	}
	return nil, false
}

func modelDefaultReasoningFor(catalog *types.ProviderOptionCatalog, model string) (types.ReasoningLevel, bool) {
	if catalog == nil || len(catalog.ModelDefaultReasoning) == 0 || strings.TrimSpace(model) == "" {
		return "", false
	}
	for key, level := range catalog.ModelDefaultReasoning {
		if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(model)) {
			return level, true
		}
	}
	return "", false
}
