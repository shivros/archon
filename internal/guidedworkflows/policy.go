package guidedworkflows

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	defaultPolicyConfidenceThreshold      = 0.70
	defaultPolicyPauseThreshold           = 0.60
	defaultPolicyHighBlastRadiusFileCount = 20
)

const (
	reasonAmbiguityBlocker         = "ambiguity_or_blocker_detected"
	reasonConfidenceBelowThreshold = "confidence_below_threshold"
	reasonHighBlastRadius          = "high_blast_radius"
	reasonSensitiveFiles           = "sensitive_files_touched"
	reasonPreCommitApproval        = "pre_commit_approval_required"
	reasonFailingChecks            = "failing_checks"
)

func DefaultCheckpointPolicy(style string) CheckpointPolicy {
	return CheckpointPolicy{
		Style:                    normalizeCheckpointStyle(style),
		ConfidenceThreshold:      defaultPolicyConfidenceThreshold,
		PauseThreshold:           defaultPolicyPauseThreshold,
		HighBlastRadiusFileCount: defaultPolicyHighBlastRadiusFileCount,
		HardGates: CheckpointPolicyGates{
			AmbiguityBlocker:  true,
			SensitiveFiles:    true,
			PreCommitApproval: false,
			FailingChecks:     true,
		},
		ConditionalGates: CheckpointPolicyGates{
			ConfidenceBelowThreshold: true,
			HighBlastRadius:          true,
			AmbiguityBlocker:         true,
			PreCommitApproval:        false,
			FailingChecks:            true,
			SensitiveFiles:           false,
		},
	}
}

func NormalizeCheckpointPolicy(policy CheckpointPolicy) CheckpointPolicy {
	out := policy
	out.Style = normalizeCheckpointStyle(out.Style)
	if out.ConfidenceThreshold <= 0 || out.ConfidenceThreshold > 1 {
		out.ConfidenceThreshold = defaultPolicyConfidenceThreshold
	}
	if out.PauseThreshold <= 0 || out.PauseThreshold > 1 {
		out.PauseThreshold = defaultPolicyPauseThreshold
	}
	if out.HighBlastRadiusFileCount <= 0 {
		out.HighBlastRadiusFileCount = defaultPolicyHighBlastRadiusFileCount
	}
	return out
}

func MergeCheckpointPolicy(base CheckpointPolicy, override *CheckpointPolicyOverride) CheckpointPolicy {
	merged := NormalizeCheckpointPolicy(base)
	if override == nil {
		return merged
	}
	if override.Style != nil {
		merged.Style = normalizeCheckpointStyle(*override.Style)
	}
	if override.ConfidenceThreshold != nil {
		merged.ConfidenceThreshold = *override.ConfidenceThreshold
	}
	if override.PauseThreshold != nil {
		merged.PauseThreshold = *override.PauseThreshold
	}
	if override.HighBlastRadiusFileCount != nil {
		merged.HighBlastRadiusFileCount = *override.HighBlastRadiusFileCount
	}
	mergeGateOverrides(&merged.HardGates, override.HardGates)
	mergeGateOverrides(&merged.ConditionalGates, override.ConditionalGates)
	return NormalizeCheckpointPolicy(merged)
}

func mergeGateOverrides(base *CheckpointPolicyGates, override *CheckpointPolicyGatesOverride) {
	if base == nil || override == nil {
		return
	}
	if override.AmbiguityBlocker != nil {
		base.AmbiguityBlocker = *override.AmbiguityBlocker
	}
	if override.ConfidenceBelowThreshold != nil {
		base.ConfidenceBelowThreshold = *override.ConfidenceBelowThreshold
	}
	if override.HighBlastRadius != nil {
		base.HighBlastRadius = *override.HighBlastRadius
	}
	if override.SensitiveFiles != nil {
		base.SensitiveFiles = *override.SensitiveFiles
	}
	if override.PreCommitApproval != nil {
		base.PreCommitApproval = *override.PreCommitApproval
	}
	if override.FailingChecks != nil {
		base.FailingChecks = *override.FailingChecks
	}
}

func cloneCheckpointPolicyOverride(in *CheckpointPolicyOverride) *CheckpointPolicyOverride {
	if in == nil {
		return nil
	}
	out := *in
	if in.Style != nil {
		value := *in.Style
		out.Style = &value
	}
	if in.ConfidenceThreshold != nil {
		value := *in.ConfidenceThreshold
		out.ConfidenceThreshold = &value
	}
	if in.PauseThreshold != nil {
		value := *in.PauseThreshold
		out.PauseThreshold = &value
	}
	if in.HighBlastRadiusFileCount != nil {
		value := *in.HighBlastRadiusFileCount
		out.HighBlastRadiusFileCount = &value
	}
	if in.HardGates != nil {
		gates := cloneCheckpointPolicyGatesOverride(in.HardGates)
		out.HardGates = &gates
	}
	if in.ConditionalGates != nil {
		gates := cloneCheckpointPolicyGatesOverride(in.ConditionalGates)
		out.ConditionalGates = &gates
	}
	return &out
}

func cloneCheckpointPolicyGatesOverride(in *CheckpointPolicyGatesOverride) CheckpointPolicyGatesOverride {
	if in == nil {
		return CheckpointPolicyGatesOverride{}
	}
	out := *in
	if in.AmbiguityBlocker != nil {
		value := *in.AmbiguityBlocker
		out.AmbiguityBlocker = &value
	}
	if in.ConfidenceBelowThreshold != nil {
		value := *in.ConfidenceBelowThreshold
		out.ConfidenceBelowThreshold = &value
	}
	if in.HighBlastRadius != nil {
		value := *in.HighBlastRadius
		out.HighBlastRadius = &value
	}
	if in.SensitiveFiles != nil {
		value := *in.SensitiveFiles
		out.SensitiveFiles = &value
	}
	if in.PreCommitApproval != nil {
		value := *in.PreCommitApproval
		out.PreCommitApproval = &value
	}
	if in.FailingChecks != nil {
		value := *in.FailingChecks
		out.FailingChecks = &value
	}
	return out
}

func EvaluateCheckpointPolicy(policy CheckpointPolicy, input PolicyEvaluationInput, now time.Time) CheckpointDecisionMetadata {
	normalized := NormalizeCheckpointPolicy(policy)
	confidence := 1.0
	if input.Confidence != nil {
		confidence = clampFloat64(*input.Confidence, 0, 1)
	}

	reasons := make([]CheckpointReason, 0, 4)
	score := 0.0
	hardGateTriggered := false

	addReason := func(reason CheckpointReason, weight float64) {
		reasons = append(reasons, reason)
		score += clampFloat64(weight, 0, 1)
		if reason.HardGate {
			hardGateTriggered = true
		}
	}

	if input.AmbiguityDetected || input.BlockerDetected {
		hard := normalized.HardGates.AmbiguityBlocker
		if hard || normalized.ConditionalGates.AmbiguityBlocker {
			addReason(CheckpointReason{
				Code:     reasonAmbiguityBlocker,
				Message:  "Ambiguity or blocker was detected and requires clarification.",
				HardGate: hard,
			}, 0.80)
		}
	}

	if confidence < normalized.ConfidenceThreshold {
		hard := normalized.HardGates.ConfidenceBelowThreshold
		if hard || normalized.ConditionalGates.ConfidenceBelowThreshold {
			delta := (normalized.ConfidenceThreshold - confidence) / normalized.ConfidenceThreshold
			weight := 0.35 + 0.55*clampFloat64(delta, 0, 1)
			addReason(CheckpointReason{
				Code:     reasonConfidenceBelowThreshold,
				Message:  fmt.Sprintf("Confidence %.2f is below threshold %.2f.", confidence, normalized.ConfidenceThreshold),
				HardGate: hard,
			}, weight)
		}
	}

	if input.ChangedFiles >= normalized.HighBlastRadiusFileCount {
		hard := normalized.HardGates.HighBlastRadius
		if hard || normalized.ConditionalGates.HighBlastRadius {
			ratio := float64(input.ChangedFiles) / float64(normalized.HighBlastRadiusFileCount)
			weight := 0.30 + 0.45*clampFloat64(ratio, 0, 1)
			addReason(CheckpointReason{
				Code:     reasonHighBlastRadius,
				Message:  fmt.Sprintf("Change set touches %d files (threshold %d).", input.ChangedFiles, normalized.HighBlastRadiusFileCount),
				HardGate: hard,
			}, weight)
		}
	}

	if len(input.SensitiveFiles) > 0 {
		hard := normalized.HardGates.SensitiveFiles
		if hard || normalized.ConditionalGates.SensitiveFiles {
			addReason(CheckpointReason{
				Code:     reasonSensitiveFiles,
				Message:  "Sensitive files were touched and require review.",
				HardGate: hard,
			}, 0.85)
		}
	}

	if input.PreCommitApprovalRequired {
		hard := normalized.HardGates.PreCommitApproval
		if hard || normalized.ConditionalGates.PreCommitApproval {
			addReason(CheckpointReason{
				Code:     reasonPreCommitApproval,
				Message:  "Pre-commit approval is required before continuing.",
				HardGate: hard,
			}, 0.65)
		}
	}

	if input.FailingChecks {
		hard := normalized.HardGates.FailingChecks
		if hard || normalized.ConditionalGates.FailingChecks {
			addReason(CheckpointReason{
				Code:     reasonFailingChecks,
				Message:  "Quality checks are failing.",
				HardGate: hard,
			}, 0.90)
		}
	}

	score = clampFloat64(score, 0, 1)
	action := CheckpointActionContinue
	if hardGateTriggered || score >= normalized.PauseThreshold {
		action = CheckpointActionPause
	}

	severity := DecisionSeverityLow
	tier := DecisionTier0
	switch {
	case hardGateTriggered || score >= 0.90:
		severity = DecisionSeverityCritical
		tier = DecisionTier3
	case action == CheckpointActionPause:
		severity = DecisionSeverityHigh
		tier = DecisionTier2
	case score >= (normalized.PauseThreshold * 0.60):
		severity = DecisionSeverityMedium
		tier = DecisionTier1
	default:
		severity = DecisionSeverityLow
		tier = DecisionTier0
	}

	return CheckpointDecisionMetadata{
		Action:              action,
		Reasons:             reasons,
		Severity:            severity,
		Tier:                tier,
		Style:               normalized.Style,
		Confidence:          confidence,
		ConfidenceThreshold: normalized.ConfidenceThreshold,
		Score:               score,
		PauseThreshold:      normalized.PauseThreshold,
		HardGateTriggered:   hardGateTriggered,
		EvaluatedAt:         now.UTC(),
	}
}

func normalizeCheckpointStyle(style string) string {
	value := strings.ToLower(strings.TrimSpace(style))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	switch value {
	case "confidence_weighted":
		return "confidence_weighted"
	default:
		return DefaultCheckpointStyle
	}
}

func clampFloat64(value, min, max float64) float64 {
	if math.IsNaN(value) {
		return min
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
