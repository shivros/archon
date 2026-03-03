package app

import (
	"testing"

	"control/internal/daemon/transcriptdomain"
)

type approvalRefreshPolicyStub struct {
	decision approvalRefreshPolicyDecision
}

func (s approvalRefreshPolicyStub) ShouldFetchApprovals(string, string, *transcriptdomain.CapabilityEnvelope) approvalRefreshPolicyDecision {
	return s.decision
}

func TestWithSessionApprovalRefreshPolicyOption(t *testing.T) {
	custom := approvalRefreshPolicyStub{
		decision: approvalRefreshPolicyDecision{
			ShouldFetch: false,
			Reason:      "custom",
		},
	}
	m := NewModel(nil, WithSessionApprovalRefreshPolicy(custom))
	decision := m.sessionApprovalRefreshPolicyOrDefault().ShouldFetchApprovals("s1", "codex", nil)
	if decision.Reason != "custom" {
		t.Fatalf("expected custom policy to be used, got %#v", decision)
	}

	m2 := NewModel(nil, WithSessionApprovalRefreshPolicy(nil))
	decision = m2.sessionApprovalRefreshPolicyOrDefault().ShouldFetchApprovals("s1", "custom", nil)
	if decision.Reason != transcriptReasonApprovalRefreshProviderFallbackUnsupported {
		t.Fatalf("expected default fallback policy, got %#v", decision)
	}
}

func TestSessionTranscriptCapabilitiesSetGet(t *testing.T) {
	m := NewModel(nil)
	if got, ok := m.sessionTranscriptCapabilitiesForSession("missing"); ok || got != nil {
		t.Fatalf("expected missing capabilities to return nil,false")
	}
	caps := transcriptdomain.CapabilityEnvelope{
		SupportsApprovals: true,
		SupportsEvents:    true,
	}
	m.setSessionTranscriptCapabilities("s1", caps)
	got, ok := m.sessionTranscriptCapabilitiesForSession("s1")
	if !ok || got == nil || !got.SupportsApprovals || !got.SupportsEvents {
		t.Fatalf("expected stored capabilities, got %#v ok=%v", got, ok)
	}

	m.setSessionTranscriptCapabilities(" ", caps)
	if _, ok := m.sessionTranscriptCapabilitiesForSession(" "); ok {
		t.Fatalf("expected blank session id to be ignored")
	}
}
