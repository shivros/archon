package types

import "strings"

type ProviderCapabilities struct {
	UsesItems         bool
	SupportsEvents    bool
	SupportsApprovals bool
	SupportsInterrupt bool
	NoProcess         bool
}

func Capabilities(provider string) ProviderCapabilities {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		return ProviderCapabilities{
			SupportsEvents:    true,
			SupportsApprovals: true,
			SupportsInterrupt: true,
		}
	case "claude":
		return ProviderCapabilities{
			UsesItems: true,
			NoProcess: true,
		}
	default:
		return ProviderCapabilities{}
	}
}
