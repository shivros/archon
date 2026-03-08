package app

import (
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
)

func TestDefaultSessionReloadDecisionPolicyIgnoresVolatileMetadata(t *testing.T) {
	policy := defaultSessionReloadDecisionPolicy{}
	prev := sessionSelectionSnapshot{
		isSession: true,
		sessionID: "s1",
		key:       "sess:s1",
		revision:  "stable",
		mode: sessionSemanticMode{
			SupportsApprovals: true,
			SupportsEvents:    true,
			UsesItems:         false,
			SupportsInterrupt: true,
			NoProcess:         false,
		},
	}
	next := prev
	decision := policy.DecideReload(prev, next)
	if decision.Reload {
		t.Fatalf("expected no reload for volatile metadata churn")
	}
	if decision.Reason != transcriptReasonReloadVolatileMetadataIgnored {
		t.Fatalf("expected volatile metadata reason, got %q", decision.Reason)
	}
}

func TestDefaultSessionReloadDecisionPolicyReloadsOnCapabilityModeChange(t *testing.T) {
	policy := defaultSessionReloadDecisionPolicy{}
	prev := sessionSelectionSnapshot{
		isSession: true,
		sessionID: "s1",
		key:       "sess:s1",
		revision:  "stable",
		mode:      sessionSemanticMode{UsesItems: true, NoProcess: true},
	}
	next := sessionSelectionSnapshot{
		isSession: true,
		sessionID: "s1",
		key:       "sess:s1",
		revision:  "stable",
		mode:      sessionSemanticMode{SupportsApprovals: true, SupportsEvents: true, SupportsInterrupt: true},
	}
	decision := policy.DecideReload(prev, next)
	if !decision.Reload {
		t.Fatalf("expected reload for capability mode change")
	}
	if decision.Reason != transcriptReasonReloadSemanticCapabilityChanged {
		t.Fatalf("expected semantic capability reason, got %q", decision.Reason)
	}
}

func TestDefaultSessionReloadCoalescerCoalescesVolatileNoopBursts(t *testing.T) {
	coalescer := NewDefaultSessionReloadCoalescer(250 * time.Millisecond)
	now := time.Now().UTC()
	decision := sessionReloadDecision{
		Reload: false,
		Reason: transcriptReasonReloadVolatileMetadataIgnored,
	}
	next := sessionSelectionSnapshot{
		isSession: true,
		sessionID: "s1",
		key:       "sess:s1",
		revision:  "stable",
		mode:      sessionSemanticMode{SupportsApprovals: true},
	}
	if got := coalescer.NoopReason(decision, next, now); got != transcriptReasonReloadVolatileMetadataIgnored {
		t.Fatalf("expected first noop reason to remain volatile metadata reason, got %q", got)
	}
	if got := coalescer.NoopReason(decision, next, now.Add(100*time.Millisecond)); got != transcriptReasonReloadCoalescedMetadataUpdate {
		t.Fatalf("expected second noop reason to coalesce, got %q", got)
	}
	if got := coalescer.NoopReason(decision, next, now.Add(2*time.Second)); got != transcriptReasonReloadVolatileMetadataIgnored {
		t.Fatalf("expected coalescing window to expire, got %q", got)
	}
}

func TestDefaultSessionCapabilityModeResolverUsesEnvelopeWhenPresent(t *testing.T) {
	resolver := defaultSessionCapabilityModeResolver{}
	mode := resolver.ResolveMode("s1", "claude", &transcriptdomain.CapabilityEnvelope{
		SupportsApprovals: true,
		SupportsEvents:    true,
		UsesItems:         false,
		SupportsInterrupt: true,
		NoProcess:         false,
	})
	expected := sessionSemanticMode{
		SupportsApprovals: true,
		SupportsEvents:    true,
		UsesItems:         false,
		SupportsInterrupt: true,
		NoProcess:         false,
	}
	if !mode.Equal(expected) {
		t.Fatalf("expected mode %#v, got %#v", expected, mode)
	}
}

func TestDefaultSessionCapabilityModeResolverFallsBackToProviderCaps(t *testing.T) {
	resolver := defaultSessionCapabilityModeResolver{}
	mode := resolver.ResolveMode("s1", "claude", nil)
	if !mode.UsesItems || !mode.SupportsApprovals {
		t.Fatalf("expected claude fallback mode to use items with approvals, got %#v", mode)
	}
}

func TestWithSessionReloadDecisionPolicyOption(t *testing.T) {
	m := NewModel(nil)
	custom := reloadPolicyStub{}
	WithSessionReloadDecisionPolicy(custom)(&m)
	if _, ok := m.sessionReloadPolicy.(reloadPolicyStub); !ok {
		t.Fatalf("expected custom reload policy to be applied")
	}
	WithSessionReloadDecisionPolicy(nil)(&m)
	if _, ok := m.sessionReloadPolicy.(defaultSessionReloadDecisionPolicy); !ok {
		t.Fatalf("expected nil option to restore default reload policy")
	}
}

func TestWithSessionReloadCoalescerOption(t *testing.T) {
	m := NewModel(nil)
	custom := &reloadCoalescerStub{}
	WithSessionReloadCoalescer(custom)(&m)
	if _, ok := m.sessionReloadCoalescer.(*reloadCoalescerStub); !ok {
		t.Fatalf("expected custom coalescer to be applied")
	}
	WithSessionReloadCoalescer(nil)(&m)
	if _, ok := m.sessionReloadCoalescer.(*defaultSessionReloadCoalescer); !ok {
		t.Fatalf("expected nil option to restore default coalescer")
	}
}

func TestWithSessionCapabilityModeResolverOption(t *testing.T) {
	m := NewModel(nil)
	custom := resolverStub{}
	WithSessionCapabilityModeResolver(custom)(&m)
	if _, ok := m.sessionCapabilityModeResolver.(resolverStub); !ok {
		t.Fatalf("expected custom resolver to be applied")
	}
	WithSessionCapabilityModeResolver(nil)(&m)
	if _, ok := m.sessionCapabilityModeResolver.(defaultSessionCapabilityModeResolver); !ok {
		t.Fatalf("expected nil option to restore default resolver")
	}
}

func TestReloadPolicyDefaultResolversHandleNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.sessionReloadPolicyOrDefault().(defaultSessionReloadDecisionPolicy); !ok {
		t.Fatalf("expected nil model to return default reload policy")
	}
	if _, ok := m.sessionReloadCoalescerOrDefault().(*defaultSessionReloadCoalescer); !ok {
		t.Fatalf("expected nil model to return default coalescer")
	}
	if _, ok := m.sessionCapabilityModeResolverOrDefault().(defaultSessionCapabilityModeResolver); !ok {
		t.Fatalf("expected nil model to return default capability resolver")
	}
}

func TestCoalesceKeyForSelectionBranches(t *testing.T) {
	if got := coalesceKeyForSelection(sessionSelectionSnapshot{}); got != "" {
		t.Fatalf("expected non-session snapshot to return empty key, got %q", got)
	}
	if got := coalesceKeyForSelection(sessionSelectionSnapshot{isSession: true}); got != "" {
		t.Fatalf("expected empty session id snapshot to return empty key, got %q", got)
	}
}

func TestDefaultSessionReloadCoalescerSkipsNonVolatileReasonsAndReset(t *testing.T) {
	coalescer := NewDefaultSessionReloadCoalescer(250 * time.Millisecond)
	now := time.Now().UTC()
	next := sessionSelectionSnapshot{
		isSession: true,
		sessionID: "s1",
		key:       "sess:s1",
		revision:  "stable",
		mode:      sessionSemanticMode{SupportsApprovals: true},
	}
	if got := coalescer.NoopReason(sessionReloadDecision{Reason: transcriptReasonSelectedRevisionUnchanged}, next, now); got != transcriptReasonSelectedRevisionUnchanged {
		t.Fatalf("expected non-volatile noop reason passthrough, got %q", got)
	}
	if got := coalescer.NoopReason(sessionReloadDecision{Reason: transcriptReasonReloadVolatileMetadataIgnored}, sessionSelectionSnapshot{}, now); got != transcriptReasonReloadVolatileMetadataIgnored {
		t.Fatalf("expected empty coalesce key to skip coalescing, got %q", got)
	}
	_ = coalescer.NoopReason(sessionReloadDecision{Reason: transcriptReasonReloadVolatileMetadataIgnored}, next, now)
	coalescer.Reset()
	if got := coalescer.NoopReason(sessionReloadDecision{Reason: transcriptReasonReloadVolatileMetadataIgnored}, next, now.Add(100*time.Millisecond)); got != transcriptReasonReloadVolatileMetadataIgnored {
		t.Fatalf("expected reset to clear coalescer state, got %q", got)
	}
}

type reloadPolicyStub struct{}

func (reloadPolicyStub) DecideReload(previous, next sessionSelectionSnapshot) sessionReloadDecision {
	_ = previous
	_ = next
	return sessionReloadDecision{}
}

type reloadCoalescerStub struct{}

func (*reloadCoalescerStub) NoopReason(decision sessionReloadDecision, next sessionSelectionSnapshot, now time.Time) string {
	_ = decision
	_ = next
	_ = now
	return "noop"
}

func (*reloadCoalescerStub) Reset() {}

type resolverStub struct{}

func (resolverStub) ResolveMode(sessionID, provider string, capabilities *transcriptdomain.CapabilityEnvelope) sessionSemanticMode {
	_ = sessionID
	_ = provider
	_ = capabilities
	return sessionSemanticMode{}
}
