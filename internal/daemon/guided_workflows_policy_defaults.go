package daemon

import (
	"control/internal/config"
	"control/internal/guidedworkflows"
)

type guidedWorkflowPolicyDefaults struct {
	ResolutionBoundary guidedworkflows.PolicyPreset
}

type guidedWorkflowCorePolicyResolver struct {
	defaults guidedWorkflowPolicyDefaults
}

type guidedWorkflowNoopPolicyResolver struct{}

func newGuidedWorkflowPolicyResolver(cfg config.CoreConfig) GuidedWorkflowPolicyResolver {
	return guidedWorkflowCorePolicyResolver{
		defaults: guidedWorkflowPolicyDefaultsFromCoreConfig(cfg),
	}
}

func guidedWorkflowPolicyDefaultsFromCoreConfig(cfg config.CoreConfig) guidedWorkflowPolicyDefaults {
	boundary, _ := guidedworkflows.NormalizePolicyPreset(cfg.GuidedWorkflowsDefaultResolutionBoundary())
	return guidedWorkflowPolicyDefaults{
		ResolutionBoundary: boundary,
	}
}

func (r guidedWorkflowCorePolicyResolver) ResolvePolicyOverrides(explicit *guidedworkflows.CheckpointPolicyOverride) *guidedworkflows.CheckpointPolicyOverride {
	return resolveGuidedWorkflowPolicyOverrides(explicit, r.defaults)
}

func (guidedWorkflowNoopPolicyResolver) ResolvePolicyOverrides(explicit *guidedworkflows.CheckpointPolicyOverride) *guidedworkflows.CheckpointPolicyOverride {
	return guidedworkflows.CloneCheckpointPolicyOverride(explicit)
}

func resolveGuidedWorkflowPolicyOverrides(
	explicit *guidedworkflows.CheckpointPolicyOverride,
	defaults guidedWorkflowPolicyDefaults,
) *guidedworkflows.CheckpointPolicyOverride {
	if explicit != nil {
		return guidedworkflows.CloneCheckpointPolicyOverride(explicit)
	}
	preset := defaults.ResolutionBoundary
	return guidedworkflows.PolicyOverrideForPreset(preset)
}
