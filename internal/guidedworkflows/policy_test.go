package guidedworkflows

import (
	"testing"
	"time"
)

func TestNormalizeCheckpointPolicyDefaults(t *testing.T) {
	policy := NormalizeCheckpointPolicy(CheckpointPolicy{})
	if policy.Style != DefaultCheckpointStyle {
		t.Fatalf("unexpected style default: %q", policy.Style)
	}
	if policy.ConfidenceThreshold != 0.70 {
		t.Fatalf("unexpected confidence threshold default: %v", policy.ConfidenceThreshold)
	}
	if policy.PauseThreshold != 0.60 {
		t.Fatalf("unexpected pause threshold default: %v", policy.PauseThreshold)
	}
	if policy.HighBlastRadiusFileCount != 20 {
		t.Fatalf("unexpected blast radius default: %d", policy.HighBlastRadiusFileCount)
	}
}

func TestMergeCheckpointPolicyOverride(t *testing.T) {
	base := DefaultCheckpointPolicy("confidence_weighted")
	confidenceThreshold := 0.85
	highBlastRadius := 45
	hardFailingChecks := false
	merged := MergeCheckpointPolicy(base, &CheckpointPolicyOverride{
		ConfidenceThreshold:      &confidenceThreshold,
		HighBlastRadiusFileCount: &highBlastRadius,
		HardGates: &CheckpointPolicyGatesOverride{
			FailingChecks: &hardFailingChecks,
		},
	})
	if merged.ConfidenceThreshold != 0.85 {
		t.Fatalf("unexpected confidence threshold: %v", merged.ConfidenceThreshold)
	}
	if merged.HighBlastRadiusFileCount != 45 {
		t.Fatalf("unexpected high blast radius file count: %d", merged.HighBlastRadiusFileCount)
	}
	if merged.HardGates.FailingChecks {
		t.Fatalf("expected hard failing_checks override=false")
	}
}

func TestEvaluateCheckpointPolicyContinueWithoutTriggers(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	decision := EvaluateCheckpointPolicy(DefaultCheckpointPolicy("confidence_weighted"), PolicyEvaluationInput{}, now)
	if decision.Action != CheckpointActionContinue {
		t.Fatalf("expected continue action, got %q", decision.Action)
	}
	if len(decision.Reasons) != 0 {
		t.Fatalf("expected no reasons for no-trigger evaluation, got %#v", decision.Reasons)
	}
	if decision.Severity != DecisionSeverityLow || decision.Tier != DecisionTier0 {
		t.Fatalf("unexpected severity/tier: %s/%s", decision.Severity, decision.Tier)
	}
}

func TestEvaluateCheckpointPolicyConfidenceThresholdBoundary(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	confidence := 0.70
	decision := EvaluateCheckpointPolicy(DefaultCheckpointPolicy("confidence_weighted"), PolicyEvaluationInput{
		Confidence: &confidence,
	}, now)
	if decision.Action != CheckpointActionContinue {
		t.Fatalf("expected continue at threshold boundary, got %q", decision.Action)
	}
	for _, reason := range decision.Reasons {
		if reason.Code == reasonConfidenceBelowThreshold {
			t.Fatalf("did not expect confidence-below-threshold reason at exact boundary")
		}
	}
}

func TestEvaluateCheckpointPolicyPauseOnHardGate(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	decision := EvaluateCheckpointPolicy(DefaultCheckpointPolicy("confidence_weighted"), PolicyEvaluationInput{
		AmbiguityDetected: true,
	}, now)
	if decision.Action != CheckpointActionPause {
		t.Fatalf("expected pause action for ambiguity hard gate, got %q", decision.Action)
	}
	if !decision.HardGateTriggered {
		t.Fatalf("expected hard gate trigger")
	}
	if decision.Severity != DecisionSeverityCritical || decision.Tier != DecisionTier3 {
		t.Fatalf("unexpected severity/tier for hard gate: %s/%s", decision.Severity, decision.Tier)
	}
}

func TestEvaluateCheckpointPolicyPauseOnConditionalBlastRadius(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	policy := DefaultCheckpointPolicy("confidence_weighted")
	policy.HardGates.AmbiguityBlocker = false
	policy.HardGates.SensitiveFiles = false
	policy.HardGates.FailingChecks = false
	decision := EvaluateCheckpointPolicy(policy, PolicyEvaluationInput{
		ChangedFiles: 40,
	}, now)
	if decision.Action != CheckpointActionPause {
		t.Fatalf("expected pause for high blast radius conditional gate, got %q", decision.Action)
	}
	if decision.HardGateTriggered {
		t.Fatalf("did not expect hard gate for blast radius conditional case")
	}
	if decision.Score < policy.PauseThreshold {
		t.Fatalf("expected score >= pause threshold, score=%v threshold=%v", decision.Score, policy.PauseThreshold)
	}
}

func TestEvaluateCheckpointPolicySmallConfidenceDropContinues(t *testing.T) {
	now := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	confidence := 0.69
	decision := EvaluateCheckpointPolicy(DefaultCheckpointPolicy("confidence_weighted"), PolicyEvaluationInput{
		Confidence: &confidence,
	}, now)
	if decision.Action != CheckpointActionContinue {
		t.Fatalf("expected continue for small confidence delta, got %q", decision.Action)
	}
	if len(decision.Reasons) == 0 {
		t.Fatalf("expected confidence reason even when continue")
	}
}
