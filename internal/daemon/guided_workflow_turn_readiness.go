package daemon

import (
	"strings"

	"control/internal/guidedworkflows"
	"control/internal/providers"
	"control/internal/types"
)

type turnProgressionReadinessPolicy interface {
	AllowProgression(event types.NotificationEvent, evidence TurnProgressionEvidence) bool
}

type turnProgressionReadinessRegistry interface {
	ForProvider(provider string) turnProgressionReadinessPolicy
}

type providerTurnProgressionReadinessRegistry struct {
	byProvider map[string]turnProgressionReadinessPolicy
	fallback   turnProgressionReadinessPolicy
}

type turnProgressionReadinessRegistryOption func(*providerTurnProgressionReadinessRegistry)

func withTurnProgressionProviderReadiness(provider string, policy turnProgressionReadinessPolicy) turnProgressionReadinessRegistryOption {
	return func(registry *providerTurnProgressionReadinessRegistry) {
		if registry == nil {
			return
		}
		normalized := providers.Normalize(provider)
		if normalized == "" || policy == nil {
			return
		}
		if registry.byProvider == nil {
			registry.byProvider = map[string]turnProgressionReadinessPolicy{}
		}
		registry.byProvider[normalized] = policy
	}
}

func withTurnProgressionFallbackReadiness(policy turnProgressionReadinessPolicy) turnProgressionReadinessRegistryOption {
	return func(registry *providerTurnProgressionReadinessRegistry) {
		if registry == nil || policy == nil {
			return
		}
		registry.fallback = policy
	}
}

func newTurnProgressionReadinessRegistry(opts ...turnProgressionReadinessRegistryOption) turnProgressionReadinessRegistry {
	registry := providerTurnProgressionReadinessRegistry{
		byProvider: map[string]turnProgressionReadinessPolicy{
			"codex":    terminalTurnProgressionReadinessPolicy{},
			"claude":   terminalTurnProgressionReadinessPolicy{},
			"opencode": openCodeTurnProgressionReadinessPolicy{},
			"kilocode": openCodeTurnProgressionReadinessPolicy{},
		},
		fallback: terminalTurnProgressionReadinessPolicy{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&registry)
		}
	}
	return registry
}

func newDefaultTurnProgressionReadinessRegistry() turnProgressionReadinessRegistry {
	return newTurnProgressionReadinessRegistry()
}

func (r providerTurnProgressionReadinessRegistry) ForProvider(provider string) turnProgressionReadinessPolicy {
	normalized := providers.Normalize(provider)
	if policy, ok := r.byProvider[normalized]; ok && policy != nil {
		return policy
	}
	if r.fallback != nil {
		return r.fallback
	}
	return allowAllTurnProgressionReadinessPolicy{}
}

type allowAllTurnProgressionReadinessPolicy struct{}

func (allowAllTurnProgressionReadinessPolicy) AllowProgression(types.NotificationEvent, TurnProgressionEvidence) bool {
	return true
}

type terminalTurnProgressionReadinessPolicy struct{}

func (terminalTurnProgressionReadinessPolicy) AllowProgression(
	_ types.NotificationEvent,
	evidence TurnProgressionEvidence,
) bool {
	return evidence.Terminal
}

type openCodeTurnProgressionReadinessPolicy struct{}

func (openCodeTurnProgressionReadinessPolicy) AllowProgression(
	_ types.NotificationEvent,
	evidence TurnProgressionEvidence,
) bool {
	if evidence.Terminal && (strings.TrimSpace(evidence.Error) != "" || guidedworkflows.IsFailedTurnStatus(evidence.Status)) {
		return true
	}
	if !evidence.Terminal {
		return false
	}
	if evidence.FreshOutput {
		return true
	}
	// Legacy fallback: no explicit freshness signal, but output exists.
	// Prefer false negatives over false positives for guided progression.
	return strings.TrimSpace(evidence.Output) != ""
}
