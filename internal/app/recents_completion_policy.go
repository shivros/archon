package app

import (
	"strings"

	"control/internal/providers"
	"control/internal/types"
)

// RecentsCompletionPolicy decides which completion signals are trusted and how
// turn IDs are resolved for recents transitions.
type RecentsCompletionPolicy interface {
	ShouldWatchCompletion(provider string) bool
	ShouldUseMetaFallback(provider string) bool
	RunBaselineTurnID(sendTurnID string, meta *types.SessionMeta) string
	CompletionTurnID(eventTurnID string, meta *types.SessionMeta) string
}

type providerCapabilitiesRecentsCompletionPolicy struct{}

func WithRecentsCompletionPolicy(policy RecentsCompletionPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.recentsCompletionPolicy = providerCapabilitiesRecentsCompletionPolicy{}
			return
		}
		m.recentsCompletionPolicy = policy
	}
}

func (providerCapabilitiesRecentsCompletionPolicy) ShouldWatchCompletion(provider string) bool {
	normalized, known := normalizeKnownProvider(provider)
	if !known {
		return false
	}
	return providers.CapabilitiesFor(normalized).SupportsEvents
}

func (providerCapabilitiesRecentsCompletionPolicy) ShouldUseMetaFallback(provider string) bool {
	normalized, known := normalizeKnownProvider(provider)
	if !known {
		// Unknown providers are treated conservatively: we avoid metadata-only
		// completion to prevent false running->ready transitions.
		return false
	}
	return !providers.CapabilitiesFor(normalized).SupportsEvents
}

func (providerCapabilitiesRecentsCompletionPolicy) RunBaselineTurnID(sendTurnID string, meta *types.SessionMeta) string {
	if sendTurnID := strings.TrimSpace(sendTurnID); sendTurnID != "" {
		return sendTurnID
	}
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(meta.LastTurnID)
}

func (providerCapabilitiesRecentsCompletionPolicy) CompletionTurnID(eventTurnID string, meta *types.SessionMeta) string {
	if turnID := strings.TrimSpace(eventTurnID); turnID != "" {
		return turnID
	}
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(meta.LastTurnID)
}

func normalizeKnownProvider(provider string) (string, bool) {
	normalized := providers.Normalize(provider)
	if normalized == "" {
		return "", false
	}
	_, known := providers.Lookup(normalized)
	return normalized, known
}

func (m *Model) recentsCompletionPolicyOrDefault() RecentsCompletionPolicy {
	if m == nil || m.recentsCompletionPolicy == nil {
		return providerCapabilitiesRecentsCompletionPolicy{}
	}
	return m.recentsCompletionPolicy
}
