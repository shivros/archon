package app

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
)

type approvalRefreshPolicyDecision struct {
	ShouldFetch bool
	Reason      string
}

type SessionApprovalRefreshPolicy interface {
	ShouldFetchApprovals(sessionID, provider string, capabilities *transcriptdomain.CapabilityEnvelope) approvalRefreshPolicyDecision
}

type defaultSessionApprovalRefreshPolicy struct{}

func WithSessionApprovalRefreshPolicy(policy SessionApprovalRefreshPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sessionApprovalRefreshPolicy = defaultSessionApprovalRefreshPolicy{}
			return
		}
		m.sessionApprovalRefreshPolicy = policy
	}
}

func (defaultSessionApprovalRefreshPolicy) ShouldFetchApprovals(
	sessionID, provider string,
	capabilities *transcriptdomain.CapabilityEnvelope,
) approvalRefreshPolicyDecision {
	_ = sessionID
	if capabilities != nil {
		if capabilities.SupportsApprovals {
			return approvalRefreshPolicyDecision{
				ShouldFetch: true,
				Reason:      transcriptReasonApprovalRefreshCapabilitySupported,
			}
		}
		return approvalRefreshPolicyDecision{
			ShouldFetch: false,
			Reason:      transcriptReasonApprovalRefreshCapabilityUnsupported,
		}
	}
	if providerSupportsApprovals(provider) {
		return approvalRefreshPolicyDecision{
			ShouldFetch: true,
			Reason:      transcriptReasonApprovalRefreshProviderFallbackSupported,
		}
	}
	return approvalRefreshPolicyDecision{
		ShouldFetch: false,
		Reason:      transcriptReasonApprovalRefreshProviderFallbackUnsupported,
	}
}

func (m *Model) sessionApprovalRefreshPolicyOrDefault() SessionApprovalRefreshPolicy {
	if m == nil || m.sessionApprovalRefreshPolicy == nil {
		return defaultSessionApprovalRefreshPolicy{}
	}
	return m.sessionApprovalRefreshPolicy
}

func (m *Model) setSessionTranscriptCapabilities(sessionID string, capabilities transcriptdomain.CapabilityEnvelope) {
	if m == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if m.sessionTranscriptCapabilities == nil {
		m.sessionTranscriptCapabilities = map[string]transcriptdomain.CapabilityEnvelope{}
	}
	m.sessionTranscriptCapabilities[sessionID] = capabilities
}

func (m *Model) sessionTranscriptCapabilitiesForSession(sessionID string) (*transcriptdomain.CapabilityEnvelope, bool) {
	if m == nil || m.sessionTranscriptCapabilities == nil {
		return nil, false
	}
	capabilities, ok := m.sessionTranscriptCapabilities[strings.TrimSpace(sessionID)]
	if !ok {
		return nil, false
	}
	capabilitiesCopy := capabilities
	return &capabilitiesCopy, true
}

func (m *Model) approvalRefreshDecision(sessionID, provider, source string) approvalRefreshPolicyDecision {
	var capabilities *transcriptdomain.CapabilityEnvelope
	if value, ok := m.sessionTranscriptCapabilitiesForSession(sessionID); ok {
		capabilities = value
	}
	decision := m.sessionApprovalRefreshPolicyOrDefault().ShouldFetchApprovals(sessionID, provider, capabilities)
	outcome := transcriptOutcomeSkipped
	if decision.ShouldFetch {
		outcome = transcriptOutcomeSuccess
	}
	m.recordTranscriptBoundaryMetric(newApprovalRefreshMetric(
		decision.Reason,
		outcome,
		source,
		sessionID,
		provider,
	))
	return decision
}
