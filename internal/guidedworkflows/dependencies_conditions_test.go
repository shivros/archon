package guidedworkflows

import (
	"errors"
	"testing"
	"time"
)

func TestDependencyConditionNormalizationAndValidation(t *testing.T) {
	t.Parallel()

	normalized, ok := NormalizeDependencyCondition("")
	if !ok || normalized != DependencyConditionOnCompleted {
		t.Fatalf("expected empty condition to normalize to on_completed")
	}

	normalized, ok = NormalizeDependencyCondition("  on_completed  ")
	if !ok || normalized != DependencyConditionOnCompleted {
		t.Fatalf("expected on_completed to normalize with surrounding whitespace")
	}

	if _, ok := NormalizeDependencyCondition("unknown_condition"); ok {
		t.Fatalf("expected unknown condition to be rejected")
	}

	if err := ValidateDependencyCondition(DependencyConditionOnCompleted); err != nil {
		t.Fatalf("expected on_completed to validate, got %v", err)
	}
	if err := ValidateDependencyCondition(DependencyCondition("unknown_condition")); !errors.Is(err, ErrDependencyCondition) {
		t.Fatalf("expected dependency condition error, got %v", err)
	}
}

func TestDefaultDependencyValidatorRejectsSelfDependency(t *testing.T) {
	t.Parallel()

	validator := defaultDependencyValidator{}
	_, err := validator.NormalizeAndValidate("gwf-self", []string{"gwf-self"}, map[string]*WorkflowRun{
		"gwf-self": {ID: "gwf-self"},
	})
	if !errors.Is(err, ErrDependencyGraph) {
		t.Fatalf("expected dependency graph error, got %v", err)
	}
}

func TestDefaultDependencyValidatorDetectsDependencyCyclePath(t *testing.T) {
	t.Parallel()

	validator := defaultDependencyValidator{}
	existing := map[string]*WorkflowRun{
		"run-a": {ID: "run-a"},
		"run-b": {
			ID:           "run-b",
			Dependencies: []RunDependency{{RunID: "run-a", Condition: DependencyConditionOnCompleted}},
		},
		"run-c": {
			ID:           "run-c",
			Dependencies: []RunDependency{{RunID: "run-b", Condition: DependencyConditionOnCompleted}},
		},
	}
	_, err := validator.NormalizeAndValidate("run-a", []string{"run-c"}, existing)
	if !errors.Is(err, ErrDependencyGraph) {
		t.Fatalf("expected dependency cycle error, got %v", err)
	}
}

func TestReverseDependencyGraphIndexRemoveRunCleansReverseLinks(t *testing.T) {
	t.Parallel()

	index := newReverseDependencyGraphIndex()
	index.SetRun(&WorkflowRun{
		ID:           "down-1",
		Dependencies: []RunDependency{{RunID: "up-1", Condition: DependencyConditionOnCompleted}},
	})
	index.SetRun(&WorkflowRun{
		ID:           "down-2",
		Dependencies: []RunDependency{{RunID: "up-1", Condition: DependencyConditionOnCompleted}},
	})

	if got := index.Dependents("up-1"); len(got) != 2 {
		t.Fatalf("expected two dependents before remove, got %#v", got)
	}

	index.RemoveRun("down-1")
	if got := index.Dependents("up-1"); len(got) != 1 || got[0] != "down-2" {
		t.Fatalf("expected one remaining dependent after first remove, got %#v", got)
	}

	index.RemoveRun("down-2")
	if got := index.Dependents("up-1"); len(got) != 0 {
		t.Fatalf("expected no dependents after second remove, got %#v", got)
	}
}

func TestEvaluateDependencyConditionBranches(t *testing.T) {
	t.Parallel()

	satisfied, blocked, reason := evaluateDependencyCondition(DependencyConditionOnCompleted, WorkflowRunStatusCompleted)
	if !satisfied || blocked || reason != "" {
		t.Fatalf("expected completed dependency to satisfy, got satisfied=%v blocked=%v reason=%q", satisfied, blocked, reason)
	}

	satisfied, blocked, reason = evaluateDependencyCondition(DependencyConditionOnCompleted, WorkflowRunStatusFailed)
	if satisfied || !blocked || reason != "dependency failed" {
		t.Fatalf("expected failed dependency to block, got satisfied=%v blocked=%v reason=%q", satisfied, blocked, reason)
	}

	satisfied, blocked, reason = evaluateDependencyCondition(DependencyConditionOnCompleted, WorkflowRunStatusStopped)
	if satisfied || !blocked || reason != "dependency stopped" {
		t.Fatalf("expected stopped dependency to block, got satisfied=%v blocked=%v reason=%q", satisfied, blocked, reason)
	}

	satisfied, blocked, reason = evaluateDependencyCondition(DependencyConditionOnCompleted, WorkflowRunStatusRunning)
	if satisfied || blocked || reason != "" {
		t.Fatalf("expected running dependency to remain unmet but not blocked, got satisfied=%v blocked=%v reason=%q", satisfied, blocked, reason)
	}

	satisfied, blocked, reason = evaluateDependencyCondition(DependencyCondition("invalid"), WorkflowRunStatusRunning)
	if satisfied || !blocked || reason != "dependency condition invalid" {
		t.Fatalf("expected invalid condition to block, got satisfied=%v blocked=%v reason=%q", satisfied, blocked, reason)
	}
}

func TestDefaultDependencyEvaluatorInvalidConditionBlocksRun(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	evaluator := defaultDependencyEvaluator{}
	run := &WorkflowRun{
		ID: "down-1",
		Dependencies: []RunDependency{
			{RunID: "up-1", Condition: DependencyCondition("invalid")},
		},
	}

	state := evaluator.Evaluate(now, run, func(runID string) (*WorkflowRun, bool) {
		if runID != "up-1" {
			return nil, false
		}
		return &WorkflowRun{ID: "up-1", Status: WorkflowRunStatusRunning}, true
	})
	if state.Ready {
		t.Fatalf("expected invalid dependency condition to keep run unready")
	}
	if !state.Blocking {
		t.Fatalf("expected invalid dependency condition to block run")
	}
	if state.Reason != "dependency condition invalid" {
		t.Fatalf("unexpected blocking reason: %q", state.Reason)
	}
	if len(state.Unmet) != 1 || !state.Unmet[0].Blocking {
		t.Fatalf("expected one blocking unmet dependency, got %#v", state.Unmet)
	}
}
