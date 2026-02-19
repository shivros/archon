package daemon

import (
	"control/internal/config"
	"control/internal/guidedworkflows"
)

const (
	guidedWorkflowBoundaryLowConfidenceThreshold  = 0.35
	guidedWorkflowBoundaryLowPauseThreshold       = 0.85
	guidedWorkflowBoundaryHighConfidenceThreshold = 0.75
	guidedWorkflowBoundaryHighPauseThreshold      = 0.45
)

type guidedWorkflowPolicyDefaults struct {
	Risk               string
	ResolutionBoundary string
}

func guidedWorkflowPolicyDefaultsFromCoreConfig(cfg config.CoreConfig) guidedWorkflowPolicyDefaults {
	return guidedWorkflowPolicyDefaults{
		Risk:               cfg.GuidedWorkflowsDefaultRisk(),
		ResolutionBoundary: cfg.GuidedWorkflowsDefaultResolutionBoundary(),
	}
}

func resolveGuidedWorkflowPolicyOverrides(
	explicit *guidedworkflows.CheckpointPolicyOverride,
	defaults guidedWorkflowPolicyDefaults,
) *guidedworkflows.CheckpointPolicyOverride {
	if explicit != nil {
		return cloneGuidedWorkflowPolicyOverride(explicit)
	}
	boundary := defaults.ResolutionBoundary
	if boundary == "" {
		boundary = defaults.Risk
	}
	return guidedWorkflowPolicyOverridesForBoundary(boundary)
}

func guidedWorkflowPolicyOverridesForBoundary(boundary string) *guidedworkflows.CheckpointPolicyOverride {
	switch boundary {
	case "low":
		confidence := guidedWorkflowBoundaryLowConfidenceThreshold
		pause := guidedWorkflowBoundaryLowPauseThreshold
		return &guidedworkflows.CheckpointPolicyOverride{
			ConfidenceThreshold: &confidence,
			PauseThreshold:      &pause,
		}
	case "high":
		confidence := guidedWorkflowBoundaryHighConfidenceThreshold
		pause := guidedWorkflowBoundaryHighPauseThreshold
		return &guidedworkflows.CheckpointPolicyOverride{
			ConfidenceThreshold: &confidence,
			PauseThreshold:      &pause,
		}
	default:
		return nil
	}
}

func cloneGuidedWorkflowPolicyOverride(in *guidedworkflows.CheckpointPolicyOverride) *guidedworkflows.CheckpointPolicyOverride {
	if in == nil {
		return nil
	}
	out := *in
	if in.HardGates != nil {
		hardGates := *in.HardGates
		out.HardGates = &hardGates
	}
	if in.ConditionalGates != nil {
		conditionalGates := *in.ConditionalGates
		out.ConditionalGates = &conditionalGates
	}
	return &out
}
