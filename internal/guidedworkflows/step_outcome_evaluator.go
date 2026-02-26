package guidedworkflows

import (
	"context"
	"strings"
)

type StepOutcomeDecision string

const (
	StepOutcomeDecisionSuccess      StepOutcomeDecision = "success"
	StepOutcomeDecisionFailure      StepOutcomeDecision = "failure"
	StepOutcomeDecisionUndetermined StepOutcomeDecision = "undetermined"
)

type StepOutcomeEvaluationInput struct {
	RunID   string
	PhaseID string
	StepID  string
	Step    StepRun
	Signal  TurnSignal
}

type StepOutcomeEvaluation struct {
	Decision      StepOutcomeDecision
	FailureDetail string
	SuccessDetail string
	Outcome       string
}

type StepOutcomeEvaluator interface {
	EvaluateStepOutcome(ctx context.Context, input StepOutcomeEvaluationInput) StepOutcomeEvaluation
}

type defaultStepOutcomeEvaluator struct{}

func (defaultStepOutcomeEvaluator) EvaluateStepOutcome(_ context.Context, input StepOutcomeEvaluationInput) StepOutcomeEvaluation {
	if failure, failed := TurnSignalFailureDetail(input.Signal); failed {
		return StepOutcomeEvaluation{
			Decision:      StepOutcomeDecisionFailure,
			FailureDetail: failure,
			Outcome:       "failed",
		}
	}
	return StepOutcomeEvaluation{
		Decision:      StepOutcomeDecisionSuccess,
		SuccessDetail: "completed by turn signal",
		Outcome:       "success",
	}
}

func normalizeStepOutcomeEvaluation(eval StepOutcomeEvaluation) StepOutcomeEvaluation {
	eval.Decision = normalizeStepOutcomeDecision(eval.Decision)
	eval.FailureDetail = strings.TrimSpace(eval.FailureDetail)
	eval.SuccessDetail = strings.TrimSpace(eval.SuccessDetail)
	eval.Outcome = strings.TrimSpace(eval.Outcome)
	if eval.Decision == StepOutcomeDecisionUndetermined && eval.SuccessDetail == "" && eval.FailureDetail == "" {
		eval.SuccessDetail = "invalid step outcome decision; deferred"
	}
	return eval
}

func normalizeStepOutcomeDecision(raw StepOutcomeDecision) StepOutcomeDecision {
	switch strings.ToLower(strings.TrimSpace(string(raw))) {
	case "failure", "failed", "error":
		return StepOutcomeDecisionFailure
	case "success", "succeeded", "complete", "completed":
		return StepOutcomeDecisionSuccess
	case "undetermined", "unknown", "defer", "pending", "awaiting", "awaiting_turn":
		return StepOutcomeDecisionUndetermined
	default:
		return StepOutcomeDecisionUndetermined
	}
}
