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
				Risk:               "high",
				ResolutionBoundary: "low",
			},
		},
	}
	defaults := guidedWorkflowPolicyDefaultsFromCoreConfig(cfg)
	if defaults.Risk != guidedworkflows.PolicyPresetHigh {
		t.Fatalf("expected risk default high, got %q", defaults.Risk)
	}
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
		Risk:               guidedworkflows.PolicyPresetLow,
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

func TestResolveGuidedWorkflowPolicyOverridesUsesBoundaryBeforeRisk(t *testing.T) {
	got := resolveGuidedWorkflowPolicyOverrides(nil, guidedWorkflowPolicyDefaults{
		Risk:               guidedworkflows.PolicyPresetLow,
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

func TestResolveGuidedWorkflowPolicyOverridesUsesRiskWhenBoundaryUnset(t *testing.T) {
	got := resolveGuidedWorkflowPolicyOverrides(nil, guidedWorkflowPolicyDefaults{
		Risk: guidedworkflows.PolicyPresetLow,
	})
	if got == nil {
		t.Fatalf("expected low risk defaults when boundary is unset")
	}
	if got.ConfidenceThreshold == nil || *got.ConfidenceThreshold != guidedworkflows.PolicyPresetLowConfidenceThreshold {
		t.Fatalf("unexpected confidence threshold for low risk: %#v", got.ConfidenceThreshold)
	}
	if got.PauseThreshold == nil || *got.PauseThreshold != guidedworkflows.PolicyPresetLowPauseThreshold {
		t.Fatalf("unexpected pause threshold for low risk: %#v", got.PauseThreshold)
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
