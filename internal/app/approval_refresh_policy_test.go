package app

import (
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestDefaultSessionApprovalRefreshPolicyUsesCapabilitiesFirst(t *testing.T) {
	policy := defaultSessionApprovalRefreshPolicy{}
	supported := transcriptdomain.CapabilityEnvelope{SupportsApprovals: true}
	unsupported := transcriptdomain.CapabilityEnvelope{SupportsApprovals: false}

	if decision := policy.ShouldFetchApprovals("s1", "custom", &supported); !decision.ShouldFetch || decision.Reason != transcriptReasonApprovalRefreshCapabilitySupported {
		t.Fatalf("expected capability-supported decision, got %#v", decision)
	}
	if decision := policy.ShouldFetchApprovals("s1", "codex", &unsupported); decision.ShouldFetch || decision.Reason != transcriptReasonApprovalRefreshCapabilityUnsupported {
		t.Fatalf("expected capability-unsupported decision, got %#v", decision)
	}
}

func TestDefaultSessionApprovalRefreshPolicyFallsBackToProviderCapabilities(t *testing.T) {
	policy := defaultSessionApprovalRefreshPolicy{}

	if decision := policy.ShouldFetchApprovals("s1", "codex", nil); !decision.ShouldFetch || decision.Reason != transcriptReasonApprovalRefreshProviderFallbackSupported {
		t.Fatalf("expected codex fallback support, got %#v", decision)
	}
	if decision := policy.ShouldFetchApprovals("s1", "custom", nil); decision.ShouldFetch || decision.Reason != transcriptReasonApprovalRefreshProviderFallbackUnsupported {
		t.Fatalf("expected custom fallback unsupported, got %#v", decision)
	}
}

func TestApprovalRefreshDecisionRecordsMetric(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("codex")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	m.setSessionTranscriptCapabilities("s1", transcriptdomain.CapabilityEnvelope{SupportsApprovals: true})

	decision := m.approvalRefreshDecision("s1", "codex", transcriptSourceSendMsg)
	if !decision.ShouldFetch {
		t.Fatalf("expected approvals fetch decision")
	}
	metrics := sink.Snapshot()
	if len(metrics) == 0 {
		t.Fatalf("expected approval refresh metric")
	}
	last := metrics[len(metrics)-1]
	if last.Name != transcriptMetricApprovalRefresh || last.Reason != transcriptReasonApprovalRefreshCapabilitySupported {
		t.Fatalf("unexpected approval refresh metric: %#v", last)
	}
}
