package daemon

import (
	"testing"

	"control/internal/config"
	"control/internal/guidedworkflows"
)

func TestGuidedWorkflowPolicyDefaultsFromCoreConfig(t *testing.T) {
	cfg := config.CoreConfig{
		GuidedWorkflows: config.CoreGuidedWorkflowsConfig{
			Defaults: config.CoreGuidedWorkflowsDefaultsConfig{
				ResolutionBoundary: "low",
			},
		},
	}
	defaults := guidedWorkflowPolicyDefaultsFromCoreConfig(cfg)
	if defaults.ResolutionBoundary != guidedworkflows.PolicyPresetLow {
		t.Fatalf("expected resolution boundary low, got %q", defaults.ResolutionBoundary)
	}
}

func TestResolveGuidedWorkflowPolicyOverridesPrefersExplicitOverride(t *testing.T) {
	confidence := 0.9
	pause := 0.1
	explicit := &guidedworkflows.CheckpointPolicyOverride{
		ConfidenceThreshold: &confidence,
		PauseThreshold:      &pause,
	}

	got := resolveGuidedWorkflowPolicyOverrides(explicit, guidedWorkflowPolicyDefaults{
		ResolutionBoundary: guidedworkflows.PolicyPresetHigh,
	})
	if got == nil {
		t.Fatalf("expected explicit policy override to be preserved")
	}
	if got == explicit {
		t.Fatalf("expected explicit policy override to be cloned")
	}
	if got.ConfidenceThreshold == nil || *got.ConfidenceThreshold != 0.9 {
		t.Fatalf("unexpected explicit confidence threshold: %#v", got.ConfidenceThreshold)
	}
	if got.PauseThreshold == nil || *got.PauseThreshold != 0.1 {
		t.Fatalf("unexpected explicit pause threshold: %#v", got.PauseThreshold)
	}
}

func TestResolveGuidedWorkflowPolicyOverridesUsesResolutionBoundary(t *testing.T) {
	got := resolveGuidedWorkflowPolicyOverrides(nil, guidedWorkflowPolicyDefaults{
		ResolutionBoundary: guidedworkflows.PolicyPresetHigh,
	})
	if got == nil {
		t.Fatalf("expected high boundary defaults")
	}
	if got.ConfidenceThreshold == nil || *got.ConfidenceThreshold != guidedworkflows.PolicyPresetHighConfidenceThreshold {
		t.Fatalf("unexpected confidence threshold for high boundary: %#v", got.ConfidenceThreshold)
	}
	if got.PauseThreshold == nil || *got.PauseThreshold != guidedworkflows.PolicyPresetHighPauseThreshold {
		t.Fatalf("unexpected pause threshold for high boundary: %#v", got.PauseThreshold)
	}
}

func TestResolveGuidedWorkflowPolicyOverridesReturnsNilWhenBoundaryUnset(t *testing.T) {
	got := resolveGuidedWorkflowPolicyOverrides(nil, guidedWorkflowPolicyDefaults{})
	if got != nil {
		t.Fatalf("expected nil override when boundary is unset, got %#v", got)
	}
}

func TestResolveGuidedWorkflowPolicyOverridesReturnsNilForBalancedOrUnknown(t *testing.T) {
	if got := resolveGuidedWorkflowPolicyOverrides(nil, guidedWorkflowPolicyDefaults{ResolutionBoundary: guidedworkflows.PolicyPresetBalanced}); got != nil {
		t.Fatalf("expected nil override for balanced boundary, got %#v", got)
	}
	if got := resolveGuidedWorkflowPolicyOverrides(nil, guidedWorkflowPolicyDefaults{}); got != nil {
		t.Fatalf("expected nil override for unknown boundary, got %#v", got)
	}
}

func TestGuidedWorkflowNoopPolicyResolverClonesExplicitOverrides(t *testing.T) {
	confidence := 0.5
	explicit := &guidedworkflows.CheckpointPolicyOverride{ConfidenceThreshold: &confidence}
	resolver := guidedWorkflowNoopPolicyResolver{}
	got := resolver.ResolvePolicyOverrides(explicit)
	if got == nil || got.ConfidenceThreshold == nil || *got.ConfidenceThreshold != 0.5 {
		t.Fatalf("unexpected cloned override: %#v", got)
	}
	if got == explicit {
		t.Fatalf("expected clone, got same pointer")
	}
}
