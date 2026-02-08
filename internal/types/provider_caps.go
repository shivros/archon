package types

import "control/internal/providers"

type ProviderCapabilities = providers.Capabilities

func Capabilities(provider string) ProviderCapabilities {
	return providers.CapabilitiesFor(provider)
}
