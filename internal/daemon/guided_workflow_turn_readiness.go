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

func newDefaultTurnProgressionReadinessRegistry() turnProgressionReadinessRegistry {
	fallback := allowAllTurnProgressionReadinessPolicy{}
	return providerTurnProgressionReadinessRegistry{
		byProvider: map[string]turnProgressionReadinessPolicy{
			"opencode": openCodeTurnProgressionReadinessPolicy{},
			"kilocode": openCodeTurnProgressionReadinessPolicy{},
		},
		fallback: fallback,
	}
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
