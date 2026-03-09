package app

import (
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

type stubComposeInterruptEligibilityPolicy struct {
	canInterrupt bool
	input        ComposeInterruptEligibilityInput
	called       bool
}

func (s *stubComposeInterruptEligibilityPolicy) CanInterrupt(input ComposeInterruptEligibilityInput) bool {
	s.called = true
	s.input = input
	return s.canInterrupt
}

type stubComposeInterruptSignalProbe struct {
	hasSignal bool
	input     ComposeInterruptSignalInput
	called    bool
}

func (s *stubComposeInterruptSignalProbe) HasSignal(input ComposeInterruptSignalInput) bool {
	s.called = true
	s.input = input
	return s.hasSignal
}

type stubComposeInterruptCapabilityProbe struct {
	supports bool
	input    ComposeInterruptCapabilityInput
	called   bool
}

func (s *stubComposeInterruptCapabilityProbe) SupportsInterrupt(input ComposeInterruptCapabilityInput) bool {
	s.called = true
	s.input = input
	return s.supports
}

func TestDefaultComposeInterruptEligibilityPolicy(t *testing.T) {
	policy := defaultComposeInterruptEligibilityPolicy{}
	if policy.CanInterrupt(ComposeInterruptEligibilityInput{}) {
		t.Fatalf("expected blank session to be non-interruptible")
	}
	if policy.CanInterrupt(ComposeInterruptEligibilityInput{
		SessionID:         "s1",
		SessionStatus:     types.SessionStatusExited,
		SupportsInterrupt: true,
		HasSignal:         true,
	}) {
		t.Fatalf("expected completed session to be non-interruptible")
	}
	if !policy.CanInterrupt(ComposeInterruptEligibilityInput{
		SessionID:         "s1",
		SessionStatus:     types.SessionStatusRunning,
		SupportsInterrupt: true,
		HasSignal:         true,
	}) {
		t.Fatalf("expected running session with support and signal to be interruptible")
	}
}

func TestWithComposeInterruptEligibilityPolicyConfiguresAndResetsDefault(t *testing.T) {
	custom := &stubComposeInterruptEligibilityPolicy{canInterrupt: true}
	m := NewModel(nil, WithComposeInterruptEligibilityPolicy(custom))
	if m.composeInterruptEligibilityPolicyOrDefault() != custom {
		t.Fatalf("expected custom policy to be configured")
	}
	WithComposeInterruptEligibilityPolicy(nil)(&m)
	if _, ok := m.composeInterruptEligibilityPolicyOrDefault().(defaultComposeInterruptEligibilityPolicy); !ok {
		t.Fatalf("expected default policy after reset, got %T", m.composeInterruptEligibilityPolicyOrDefault())
	}
}

func TestWithComposeInterruptEligibilityPolicyHandlesNilModel(t *testing.T) {
	WithComposeInterruptEligibilityPolicy(&stubComposeInterruptEligibilityPolicy{canInterrupt: true})(nil)
}

func TestComposeInterruptEligibilityPolicyOrDefaultHandlesNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.composeInterruptEligibilityPolicyOrDefault().(defaultComposeInterruptEligibilityPolicy); !ok {
		t.Fatalf("expected default eligibility policy for nil model")
	}
}

func TestDefaultComposeInterruptSignalProbe(t *testing.T) {
	probe := defaultComposeInterruptSignalProbe{}
	if probe.HasSignal(ComposeInterruptSignalInput{SessionID: " "}) {
		t.Fatalf("expected blank session to have no interrupt signal")
	}
	if !probe.HasSignal(ComposeInterruptSignalInput{SessionID: "s1", InFlightSessionID: "s1"}) {
		t.Fatalf("expected in-flight interrupt signal")
	}
	if !probe.HasSignal(ComposeInterruptSignalInput{
		SessionID:       "s1",
		RequestActivity: requestActivity{active: true, sessionID: "s1"},
	}) {
		t.Fatalf("expected request activity signal")
	}
	tracker := NewRecentsTracker()
	tracker.StartRun("s1", "", time.Now().UTC())
	if !probe.HasSignal(ComposeInterruptSignalInput{SessionID: "s1", Recents: tracker}) {
		t.Fatalf("expected recents running signal")
	}
}

func TestWithComposeInterruptSignalProbeConfiguresAndResetsDefault(t *testing.T) {
	custom := &stubComposeInterruptSignalProbe{hasSignal: true}
	m := NewModel(nil, WithComposeInterruptSignalProbe(custom))
	if m.composeInterruptSignalProbeOrDefault() != custom {
		t.Fatalf("expected custom signal probe to be configured")
	}
	WithComposeInterruptSignalProbe(nil)(&m)
	if _, ok := m.composeInterruptSignalProbeOrDefault().(defaultComposeInterruptSignalProbe); !ok {
		t.Fatalf("expected default signal probe after reset, got %T", m.composeInterruptSignalProbeOrDefault())
	}
}

func TestWithComposeInterruptSignalProbeHandlesNilModel(t *testing.T) {
	WithComposeInterruptSignalProbe(&stubComposeInterruptSignalProbe{hasSignal: true})(nil)
}

func TestComposeInterruptSignalProbeOrDefaultHandlesNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.composeInterruptSignalProbeOrDefault().(defaultComposeInterruptSignalProbe); !ok {
		t.Fatalf("expected default signal probe for nil model")
	}
}

func TestComposeSessionHasInterruptSignalUsesConfiguredProbe(t *testing.T) {
	m := NewModel(nil)
	custom := &stubComposeInterruptSignalProbe{hasSignal: true}
	WithComposeInterruptSignalProbe(custom)(&m)

	if !m.composeSessionHasInterruptSignal("s1") {
		t.Fatalf("expected custom signal probe result")
	}
	if !custom.called {
		t.Fatalf("expected custom signal probe to be called")
	}
	if custom.input.SessionID != "s1" {
		t.Fatalf("expected session id passthrough, got %q", custom.input.SessionID)
	}
}

func TestComposeSessionSupportsInterruptUsesConfiguredCapabilityProbe(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	custom := &stubComposeInterruptCapabilityProbe{supports: true}
	WithComposeInterruptCapabilityProbe(custom)(m)

	if !m.composeSessionSupportsInterrupt("s1") {
		t.Fatalf("expected custom capability probe result")
	}
	if !custom.called {
		t.Fatalf("expected custom capability probe to be called")
	}
	if custom.input.Provider != "codex" {
		t.Fatalf("expected provider passthrough, got %q", custom.input.Provider)
	}
}

func TestCanInterruptComposeSessionUsesConfiguredEligibilityPolicy(t *testing.T) {
	m := newComposeInterruptTestModel("codex")
	policy := &stubComposeInterruptEligibilityPolicy{canInterrupt: true}
	WithComposeInterruptEligibilityPolicy(policy)(m)
	WithComposeInterruptCapabilityProbe(&stubComposeInterruptCapabilityProbe{supports: true})(m)
	WithComposeInterruptSignalProbe(&stubComposeInterruptSignalProbe{hasSignal: true})(m)

	if !m.canInterruptComposeSession("s1") {
		t.Fatalf("expected eligibility policy result")
	}
	if !policy.called {
		t.Fatalf("expected eligibility policy to be called")
	}
	if policy.input.SessionStatus != types.SessionStatusRunning {
		t.Fatalf("expected session status passthrough, got %q", policy.input.SessionStatus)
	}
}

type stubCapabilityModeResolver struct {
	mode sessionSemanticMode
}

func (s stubCapabilityModeResolver) ResolveMode(string, string, *transcriptdomain.CapabilityEnvelope) sessionSemanticMode {
	return s.mode
}

func TestDefaultComposeInterruptCapabilityProbeUsesResolver(t *testing.T) {
	probe := defaultComposeInterruptCapabilityProbe{}
	if !probe.SupportsInterrupt(ComposeInterruptCapabilityInput{
		SessionID:    "s1",
		Provider:     "codex",
		Capabilities: &transcriptdomain.CapabilityEnvelope{SupportsInterrupt: false},
		ModeResolver: stubCapabilityModeResolver{mode: sessionSemanticMode{SupportsInterrupt: true}},
	}) {
		t.Fatalf("expected capability probe to honor resolver output")
	}
}

func TestDefaultComposeInterruptCapabilityProbeUsesDefaultResolverWhenNil(t *testing.T) {
	probe := defaultComposeInterruptCapabilityProbe{}
	if !probe.SupportsInterrupt(ComposeInterruptCapabilityInput{
		SessionID:    "s1",
		Provider:     "codex",
		Capabilities: &transcriptdomain.CapabilityEnvelope{SupportsInterrupt: true},
		ModeResolver: nil,
	}) {
		t.Fatalf("expected capability probe to use default resolver when nil")
	}
}

func TestWithComposeInterruptCapabilityProbeConfiguresAndResetsDefault(t *testing.T) {
	custom := &stubComposeInterruptCapabilityProbe{supports: true}
	m := NewModel(nil, WithComposeInterruptCapabilityProbe(custom))
	if m.composeInterruptCapabilityProbeOrDefault() != custom {
		t.Fatalf("expected custom capability probe to be configured")
	}
	WithComposeInterruptCapabilityProbe(nil)(&m)
	if _, ok := m.composeInterruptCapabilityProbeOrDefault().(defaultComposeInterruptCapabilityProbe); !ok {
		t.Fatalf("expected default capability probe after reset, got %T", m.composeInterruptCapabilityProbeOrDefault())
	}
}

func TestWithComposeInterruptCapabilityProbeHandlesNilModel(t *testing.T) {
	WithComposeInterruptCapabilityProbe(&stubComposeInterruptCapabilityProbe{supports: true})(nil)
}

func TestComposeInterruptCapabilityProbeOrDefaultHandlesNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.composeInterruptCapabilityProbeOrDefault().(defaultComposeInterruptCapabilityProbe); !ok {
		t.Fatalf("expected default capability probe for nil model")
	}
}
