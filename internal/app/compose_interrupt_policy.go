package app

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

type ComposeInterruptEligibilityInput struct {
	SessionID         string
	SessionStatus     types.SessionStatus
	SupportsInterrupt bool
	HasSignal         bool
}

type ComposeInterruptEligibilityPolicy interface {
	CanInterrupt(input ComposeInterruptEligibilityInput) bool
}

type defaultComposeInterruptEligibilityPolicy struct{}

func WithComposeInterruptEligibilityPolicy(policy ComposeInterruptEligibilityPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.composeInterruptEligibilityPolicy = defaultComposeInterruptEligibilityPolicy{}
			return
		}
		m.composeInterruptEligibilityPolicy = policy
	}
}

func (defaultComposeInterruptEligibilityPolicy) CanInterrupt(input ComposeInterruptEligibilityInput) bool {
	if strings.TrimSpace(input.SessionID) == "" {
		return false
	}
	if !isSessionInterruptible(input.SessionStatus) {
		return false
	}
	return input.SupportsInterrupt && input.HasSignal
}

func (m *Model) composeInterruptEligibilityPolicyOrDefault() ComposeInterruptEligibilityPolicy {
	if m == nil || m.composeInterruptEligibilityPolicy == nil {
		return defaultComposeInterruptEligibilityPolicy{}
	}
	return m.composeInterruptEligibilityPolicy
}

type ComposeInterruptSignalInput struct {
	SessionID         string
	InFlightSessionID string
	RequestActivity   requestActivity
	Recents           recentsDomain
}

type ComposeInterruptSignalProbe interface {
	HasSignal(input ComposeInterruptSignalInput) bool
}

type defaultComposeInterruptSignalProbe struct{}

func WithComposeInterruptSignalProbe(probe ComposeInterruptSignalProbe) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if probe == nil {
			m.composeInterruptSignalProbe = defaultComposeInterruptSignalProbe{}
			return
		}
		m.composeInterruptSignalProbe = probe
	}
}

func (defaultComposeInterruptSignalProbe) HasSignal(input ComposeInterruptSignalInput) bool {
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return false
	}
	if strings.TrimSpace(input.InFlightSessionID) == sessionID {
		return true
	}
	if input.RequestActivity.active && strings.TrimSpace(input.RequestActivity.sessionID) == sessionID {
		return true
	}
	return input.Recents != nil && input.Recents.IsRunning(sessionID)
}

func (m *Model) composeInterruptSignalProbeOrDefault() ComposeInterruptSignalProbe {
	if m == nil || m.composeInterruptSignalProbe == nil {
		return defaultComposeInterruptSignalProbe{}
	}
	return m.composeInterruptSignalProbe
}

type ComposeInterruptCapabilityInput struct {
	SessionID    string
	Provider     string
	Capabilities *transcriptdomain.CapabilityEnvelope
	ModeResolver SessionCapabilityModeResolver
}

type ComposeInterruptCapabilityProbe interface {
	SupportsInterrupt(input ComposeInterruptCapabilityInput) bool
}

type defaultComposeInterruptCapabilityProbe struct{}

func WithComposeInterruptCapabilityProbe(probe ComposeInterruptCapabilityProbe) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if probe == nil {
			m.composeInterruptCapabilityProbe = defaultComposeInterruptCapabilityProbe{}
			return
		}
		m.composeInterruptCapabilityProbe = probe
	}
}

func (defaultComposeInterruptCapabilityProbe) SupportsInterrupt(input ComposeInterruptCapabilityInput) bool {
	resolver := input.ModeResolver
	if resolver == nil {
		resolver = defaultSessionCapabilityModeResolver{}
	}
	mode := resolver.ResolveMode(strings.TrimSpace(input.SessionID), strings.TrimSpace(input.Provider), input.Capabilities)
	return mode.SupportsInterrupt
}

func (m *Model) composeInterruptCapabilityProbeOrDefault() ComposeInterruptCapabilityProbe {
	if m == nil || m.composeInterruptCapabilityProbe == nil {
		return defaultComposeInterruptCapabilityProbe{}
	}
	return m.composeInterruptCapabilityProbe
}
