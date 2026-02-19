package guidedworkflows

import "strings"

type PolicyPreset string

const (
	PolicyPresetLow      PolicyPreset = "low"
	PolicyPresetBalanced PolicyPreset = "balanced"
	PolicyPresetHigh     PolicyPreset = "high"
)

const (
	PolicyPresetLowConfidenceThreshold  = 0.35
	PolicyPresetLowPauseThreshold       = 0.85
	PolicyPresetHighConfidenceThreshold = 0.75
	PolicyPresetHighPauseThreshold      = 0.45
)

func NormalizePolicyPreset(raw string) (PolicyPreset, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	switch value {
	case "":
		return "", true
	case "low":
		return PolicyPresetLow, true
	case "balanced", "medium", "default":
		return PolicyPresetBalanced, true
	case "high":
		return PolicyPresetHigh, true
	default:
		return "", false
	}
}

func ResolvePolicyPreset(resolutionBoundary PolicyPreset, risk PolicyPreset) PolicyPreset {
	if resolutionBoundary != "" {
		return resolutionBoundary
	}
	return risk
}

func PolicyOverrideForPreset(preset PolicyPreset) *CheckpointPolicyOverride {
	switch preset {
	case PolicyPresetLow:
		confidence := PolicyPresetLowConfidenceThreshold
		pause := PolicyPresetLowPauseThreshold
		return &CheckpointPolicyOverride{
			ConfidenceThreshold: &confidence,
			PauseThreshold:      &pause,
		}
	case PolicyPresetHigh:
		confidence := PolicyPresetHighConfidenceThreshold
		pause := PolicyPresetHighPauseThreshold
		return &CheckpointPolicyOverride{
			ConfidenceThreshold: &confidence,
			PauseThreshold:      &pause,
		}
	default:
		return nil
	}
}
