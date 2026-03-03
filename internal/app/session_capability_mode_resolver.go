package app

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
)

type SessionCapabilityModeResolver interface {
	ResolveMode(sessionID, provider string, capabilities *transcriptdomain.CapabilityEnvelope) sessionSemanticMode
}

type defaultSessionCapabilityModeResolver struct{}

func WithSessionCapabilityModeResolver(resolver SessionCapabilityModeResolver) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if resolver == nil {
			m.sessionCapabilityModeResolver = defaultSessionCapabilityModeResolver{}
			return
		}
		m.sessionCapabilityModeResolver = resolver
	}
}

func (defaultSessionCapabilityModeResolver) ResolveMode(
	sessionID, provider string,
	capabilities *transcriptdomain.CapabilityEnvelope,
) sessionSemanticMode {
	_ = strings.TrimSpace(sessionID)
	provider = strings.TrimSpace(provider)
	if capabilities != nil {
		return sessionSemanticMode{
			SupportsApprovals: capabilities.SupportsApprovals,
			SupportsEvents:    capabilities.SupportsEvents,
			UsesItems:         capabilities.UsesItems,
			SupportsInterrupt: capabilities.SupportsInterrupt,
			NoProcess:         capabilities.NoProcess,
		}
	}
	fallback := providers.CapabilitiesFor(provider)
	return sessionSemanticMode{
		SupportsApprovals: fallback.SupportsApprovals,
		SupportsEvents:    fallback.SupportsEvents,
		UsesItems:         fallback.UsesItems,
		SupportsInterrupt: fallback.SupportsInterrupt,
		NoProcess:         fallback.NoProcess,
	}
}

func (m *Model) sessionCapabilityModeResolverOrDefault() SessionCapabilityModeResolver {
	if m == nil || m.sessionCapabilityModeResolver == nil {
		return defaultSessionCapabilityModeResolver{}
	}
	return m.sessionCapabilityModeResolver
}
