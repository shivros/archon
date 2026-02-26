package guidedworkflows

import "testing"

func TestNormalizeStepOutcomeDecisionAliases(t *testing.T) {
	tests := []struct {
		in   StepOutcomeDecision
		want StepOutcomeDecision
	}{
		{in: "success", want: StepOutcomeDecisionSuccess},
		{in: "succeeded", want: StepOutcomeDecisionSuccess},
		{in: "complete", want: StepOutcomeDecisionSuccess},
		{in: "completed", want: StepOutcomeDecisionSuccess},
		{in: "failure", want: StepOutcomeDecisionFailure},
		{in: "failed", want: StepOutcomeDecisionFailure},
		{in: "error", want: StepOutcomeDecisionFailure},
		{in: "undetermined", want: StepOutcomeDecisionUndetermined},
		{in: "awaiting_turn", want: StepOutcomeDecisionUndetermined},
		{in: "new_decision", want: StepOutcomeDecisionUndetermined},
	}
	for _, tt := range tests {
		if got := normalizeStepOutcomeDecision(tt.in); got != tt.want {
			t.Fatalf("normalizeStepOutcomeDecision(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeStepOutcomeEvaluationUndeterminedAddsDetail(t *testing.T) {
	eval := normalizeStepOutcomeEvaluation(StepOutcomeEvaluation{Decision: StepOutcomeDecision("bad_value")})
	if eval.Decision != StepOutcomeDecisionUndetermined {
		t.Fatalf("expected undetermined decision, got %q", eval.Decision)
	}
	if eval.SuccessDetail == "" {
		t.Fatalf("expected undetermined normalization detail")
	}
}

func TestNormalizeStepOutcomeEvaluationPreservesExplicitDetails(t *testing.T) {
	eval := normalizeStepOutcomeEvaluation(StepOutcomeEvaluation{
		Decision:      StepOutcomeDecisionUndetermined,
		SuccessDetail: "keep this detail",
	})
	if eval.SuccessDetail != "keep this detail" {
		t.Fatalf("expected explicit detail to be preserved, got %q", eval.SuccessDetail)
	}
}
