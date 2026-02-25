package app

import (
	"strings"

	"control/internal/types"
)

type runtimeCatalogLookup interface {
	providerOptionCatalog(provider string) *types.ProviderOptionCatalog
}

type runtimeDefaultsLookup interface {
	composeDefaultsForProvider(provider string) *types.SessionRuntimeOptions
}

type runtimeOptionResolver struct {
	catalogs runtimeCatalogLookup
	defaults runtimeDefaultsLookup
}

func newRuntimeOptionResolver(catalogs runtimeCatalogLookup, defaults runtimeDefaultsLookup) runtimeOptionResolver {
	return runtimeOptionResolver{
		catalogs: catalogs,
		defaults: defaults,
	}
}

func (r runtimeOptionResolver) resolve(provider string, current *types.SessionRuntimeOptions) *types.SessionRuntimeOptions {
	provider = strings.ToLower(strings.TrimSpace(provider))
	out := types.CloneRuntimeOptions(current)
	if out == nil {
		out = &types.SessionRuntimeOptions{}
	}
	if provider == "" || r.catalogs == nil {
		return out
	}
	catalog := r.catalogs.providerOptionCatalog(provider)
	if catalog == nil {
		return out
	}
	effectiveDefaults := types.CloneRuntimeOptions(&catalog.Defaults)
	if r.defaults != nil {
		effectiveDefaults = types.MergeRuntimeOptions(effectiveDefaults, r.defaults.composeDefaultsForProvider(provider))
	}
	if effectiveDefaults == nil {
		effectiveDefaults = &types.SessionRuntimeOptions{}
	}

	model := strings.TrimSpace(out.Model)
	if len(catalog.Models) > 0 {
		if !runtimeModelAllowed(model, catalog.Models) {
			model = strings.TrimSpace(effectiveDefaults.Model)
		}
		if !runtimeModelAllowed(model, catalog.Models) {
			model = strings.TrimSpace(catalog.Models[0])
		}
	} else if model == "" {
		model = strings.TrimSpace(effectiveDefaults.Model)
	}
	out.Model = model

	if len(catalog.AccessLevels) > 0 {
		if !runtimeAccessAllowed(out.Access, catalog.AccessLevels) {
			out.Access = effectiveDefaults.Access
		}
		if !runtimeAccessAllowed(out.Access, catalog.AccessLevels) {
			out.Access = catalog.AccessLevels[0]
		}
	} else if out.Access == "" {
		out.Access = effectiveDefaults.Access
	}

	allowedReasoning := runtimeReasoningLevelsForModel(catalog, out.Model)
	if len(allowedReasoning) == 0 {
		out.Reasoning = ""
		return out
	}
	if !runtimeReasoningAllowed(out.Reasoning, allowedReasoning) {
		out.Reasoning = effectiveDefaults.Reasoning
	}
	if out.Reasoning == "" || !runtimeReasoningAllowed(out.Reasoning, allowedReasoning) {
		out.Reasoning = runtimeDefaultReasoningForModel(catalog, out.Model)
	}
	if out.Reasoning == "" || !runtimeReasoningAllowed(out.Reasoning, allowedReasoning) {
		out.Reasoning = allowedReasoning[0]
	}

	return out
}

func runtimeReasoningLevelsForModel(catalog *types.ProviderOptionCatalog, model string) []types.ReasoningLevel {
	if catalog == nil {
		return nil
	}
	model = strings.TrimSpace(model)
	if model != "" && len(catalog.ModelReasoningLevels) > 0 {
		for key, levels := range catalog.ModelReasoningLevels {
			if strings.EqualFold(strings.TrimSpace(key), model) {
				return append([]types.ReasoningLevel{}, levels...)
			}
		}
	}
	return append([]types.ReasoningLevel{}, catalog.ReasoningLevels...)
}

func runtimeDefaultReasoningForModel(catalog *types.ProviderOptionCatalog, model string) types.ReasoningLevel {
	if catalog == nil {
		return ""
	}
	model = strings.TrimSpace(model)
	if model != "" && len(catalog.ModelDefaultReasoning) > 0 {
		for key, level := range catalog.ModelDefaultReasoning {
			if strings.EqualFold(strings.TrimSpace(key), model) {
				return level
			}
		}
	}
	return catalog.Defaults.Reasoning
}

func runtimeReasoningAllowed(level types.ReasoningLevel, allowed []types.ReasoningLevel) bool {
	if level == "" || len(allowed) == 0 {
		return true
	}
	for _, entry := range allowed {
		if entry == level {
			return true
		}
	}
	return false
}

func runtimeModelAllowed(model string, allowed []string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, entry := range allowed {
		if strings.EqualFold(strings.TrimSpace(entry), model) {
			return true
		}
	}
	return false
}

func runtimeAccessAllowed(access types.AccessLevel, allowed []types.AccessLevel) bool {
	if access == "" {
		return false
	}
	for _, entry := range allowed {
		if entry == access {
			return true
		}
	}
	return false
}
