package daemon

import (
	"strings"

	"control/internal/guidedworkflows"
	"control/internal/providers"
	"control/internal/types"
)

type turnProgressionReadinessPolicy interface {
	AllowProgression(event types.NotificationEvent, status string, errMsg string, terminal bool, output string) bool
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

func (allowAllTurnProgressionReadinessPolicy) AllowProgression(types.NotificationEvent, string, string, bool, string) bool {
	return true
}

type terminalTurnProgressionReadinessPolicy struct{}

func (terminalTurnProgressionReadinessPolicy) AllowProgression(
	_ types.NotificationEvent,
	_ string,
	_ string,
	terminal bool,
	_ string,
) bool {
	return terminal
}

type openCodeTurnProgressionReadinessPolicy struct{}

func (openCodeTurnProgressionReadinessPolicy) AllowProgression(
	event types.NotificationEvent,
	status string,
	errMsg string,
	terminal bool,
	output string,
) bool {
	if terminal && (strings.TrimSpace(errMsg) != "" || guidedworkflows.IsFailedTurnStatus(status)) {
		return true
	}
	if !terminal {
		return false
	}
	if strings.TrimSpace(output) != "" {
		return true
	}
	if notificationPayloadBool(event.Payload, "artifacts_persisted") {
		return true
	}
	return notificationPayloadInt(event.Payload, "assistant_artifact_count") > 0
}
