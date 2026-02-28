package app

import "testing"

type stubDebugPanelRefreshPolicy struct {
	deferValue bool
}

func (p stubDebugPanelRefreshPolicy) ShouldDefer(DebugPanelRefreshInput) bool {
	return p.deferValue
}

func TestDefaultDebugPanelRefreshPolicyShouldDefer(t *testing.T) {
	policy := defaultDebugPanelRefreshPolicy{}
	if !policy.ShouldDefer(DebugPanelRefreshInput{ProjectionInFlight: true}) {
		t.Fatalf("expected in-flight projections to defer")
	}
	if !policy.ShouldDefer(DebugPanelRefreshInput{
		DebugStreamsEnabled: true,
		PanelVisible:        false,
		PanelWidth:          80,
	}) {
		t.Fatalf("expected hidden debug panel to defer when debug streams are enabled")
	}
	if !policy.ShouldDefer(DebugPanelRefreshInput{
		DebugStreamsEnabled: true,
		PanelVisible:        true,
		PanelWidth:          0,
	}) {
		t.Fatalf("expected zero-width debug panel to defer when debug streams are enabled")
	}
	if policy.ShouldDefer(DebugPanelRefreshInput{
		DebugStreamsEnabled: false,
		PanelVisible:        false,
		PanelWidth:          0,
	}) {
		t.Fatalf("expected disabled debug streams to avoid visibility defer")
	}
}

func TestWithDebugPanelRefreshPolicyConfiguresAndResetsDefault(t *testing.T) {
	custom := stubDebugPanelRefreshPolicy{deferValue: true}
	m := NewModel(nil, WithDebugPanelRefreshPolicy(custom))
	if got := m.debugPanelRefreshPolicy.ShouldDefer(DebugPanelRefreshInput{}); !got {
		t.Fatalf("expected custom refresh policy to be used")
	}
	WithDebugPanelRefreshPolicy(nil)(&m)
	if _, ok := m.debugPanelRefreshPolicy.(defaultDebugPanelRefreshPolicy); !ok {
		t.Fatalf("expected default debug panel refresh policy after reset, got %T", m.debugPanelRefreshPolicy)
	}
}

func TestWithDebugPanelRefreshPolicyHandlesNilModel(t *testing.T) {
	WithDebugPanelRefreshPolicy(stubDebugPanelRefreshPolicy{deferValue: true})(nil)
}

func TestDebugPanelRefreshPolicyOrDefaultHandlesNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.debugPanelRefreshPolicyOrDefault().(defaultDebugPanelRefreshPolicy); !ok {
		t.Fatalf("expected default policy for nil model")
	}
}
