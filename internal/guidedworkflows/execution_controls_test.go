package guidedworkflows

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type scriptedExecutionRunner struct {
	responses map[string][]CommandResult
	calls     []CommandRequest
}

func (r *scriptedExecutionRunner) Run(_ context.Context, req CommandRequest) CommandResult {
	r.calls = append(r.calls, req)
	key := strings.TrimSpace(req.HookID)
	if key == "" {
		key = strings.TrimSpace(req.StepID)
	}
	sequence := r.responses[key]
	if len(sequence) == 0 {
		return CommandResult{ExitCode: 0, Output: "ok", Retryable: false}
	}
	result := sequence[0]
	if len(sequence) == 1 {
		r.responses[key] = sequence
	} else {
		r.responses[key] = sequence[1:]
	}
	return result
}

func (r *scriptedExecutionRunner) countCalls(key string) int {
	key = strings.TrimSpace(key)
	count := 0
	for _, call := range r.calls {
		callKey := strings.TrimSpace(call.HookID)
		if callKey == "" {
			callKey = strings.TrimSpace(call.StepID)
		}
		if callKey == key {
			count++
		}
	}
	return count
}

func TestExecutionControlsHappyPathWithRetryAndCommitApproval(t *testing.T) {
	runner := &scriptedExecutionRunner{
		responses: map[string][]CommandResult{
			"tests": {
				{ExitCode: 1, Retryable: true, Output: "tests failed"},
				{ExitCode: 0, Retryable: false, Output: "tests passed"},
			},
			"lint":      {{ExitCode: 0, Retryable: false, Output: "lint ok"}},
			"typecheck": {{ExitCode: 0, Retryable: false, Output: "typecheck ok"}},
			"commit":    {{ExitCode: 0, Retryable: false, Output: "commit created"}},
		},
	}
	engine := NewEngine(
		WithExecutionControls(ExecutionControls{
			Enabled: true,
			Capabilities: ExecutionCapabilities{
				QualityChecks: true,
				Commit:        true,
			},
			RetryPolicy: RetryPolicy{MaxAttempts: 2},
			Commit: CommitConfig{
				RequireApproval: true,
			},
		}),
		WithExecutionRunner(runner),
	)
	service := NewRunService(Config{Enabled: true}, WithEngine(engine))

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	for i := 0; i < 32 && run.Status == WorkflowRunStatusRunning; i++ {
		run, err = service.AdvanceRun(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("AdvanceRun iteration %d: %v", i, err)
		}
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected run paused for pre-commit approval, got %q", run.Status)
	}
	if run.LatestDecision == nil {
		t.Fatalf("expected latest decision for commit approval gate")
	}
	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionActionApproveContinue,
		Note:   "approved commit",
	})
	if err != nil {
		t.Fatalf("HandleDecision approve_continue: %v", err)
	}
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed after approval and commit execution, got %q", run.Status)
	}
	if runner.countCalls("tests") != 2 {
		t.Fatalf("expected tests hook to retry once, got %d calls", runner.countCalls("tests"))
	}
	if runner.countCalls("lint") != 1 {
		t.Fatalf("expected lint hook to run once, got %d calls", runner.countCalls("lint"))
	}
	if runner.countCalls("typecheck") != 1 {
		t.Fatalf("expected typecheck hook to run once, got %d calls", runner.countCalls("typecheck"))
	}
	if runner.countCalls("commit") != 1 {
		t.Fatalf("expected commit hook to run once, got %d calls", runner.countCalls("commit"))
	}
	if !hasAuditAction(run.AuditTrail, "command_retry_scheduled") {
		t.Fatalf("expected retry audit entry in run audit trail")
	}
	if !hasAuditAction(run.AuditTrail, "policy_pause") {
		t.Fatalf("expected policy_pause audit entry in run audit trail")
	}
	if !hasAuditAction(run.AuditTrail, "decision_action") {
		t.Fatalf("expected decision_action audit entry in run audit trail")
	}
	commitReq := findLastCommandCall(runner.calls, "commit")
	if commitReq == nil {
		t.Fatalf("expected commit command request")
	}
	if message := strings.TrimSpace(commitReq.Metadata["commit_message"]); !isConventionalCommitMessage(message) {
		t.Fatalf("expected conventional commit message, got %q", message)
	}
}

func TestExecutionControlsQualityCapabilityDeniedFailsSafely(t *testing.T) {
	engine := NewEngine(
		WithExecutionControls(ExecutionControls{
			Enabled: true,
			Capabilities: ExecutionCapabilities{
				QualityChecks: false,
				Commit:        false,
			},
		}),
		WithExecutionRunner(&scriptedExecutionRunner{}),
	)
	service := NewRunService(Config{Enabled: true}, WithEngine(engine))
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	runID := run.ID
	for i := 0; i < 32 && run.Status == WorkflowRunStatusRunning; i++ {
		run, err = service.AdvanceRun(context.Background(), run.ID)
		if err != nil {
			break
		}
	}
	run, err = service.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run status, got %q", run.Status)
	}
	if !strings.Contains(strings.ToLower(run.LastError), "capability") {
		t.Fatalf("expected capability failure detail, got %q", run.LastError)
	}
	if !hasAuditAction(run.AuditTrail, "quality_checks_capability_denied") {
		t.Fatalf("expected capability-denied audit entry")
	}
}

func TestExecutionControlsRetryBoundedFailurePath(t *testing.T) {
	runner := &scriptedExecutionRunner{
		responses: map[string][]CommandResult{
			"tests": {
				{ExitCode: 1, Retryable: true, Output: "fail once"},
				{ExitCode: 1, Retryable: true, Output: "fail twice"},
			},
		},
	}
	engine := NewEngine(
		WithExecutionControls(ExecutionControls{
			Enabled: true,
			Capabilities: ExecutionCapabilities{
				QualityChecks: true,
				Commit:        false,
			},
			RetryPolicy: RetryPolicy{MaxAttempts: 2},
		}),
		WithExecutionRunner(runner),
	)
	service := NewRunService(Config{Enabled: true}, WithEngine(engine))
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	runID := run.ID
	for i := 0; i < 32 && run.Status == WorkflowRunStatusRunning; i++ {
		run, err = service.AdvanceRun(context.Background(), run.ID)
		if err != nil {
			break
		}
	}
	run, err = service.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run status, got %q", run.Status)
	}
	qualityStep := findStep(run, "quality_checks")
	if qualityStep == nil {
		t.Fatalf("expected quality_checks step in run")
	}
	if qualityStep.Attempts != 2 {
		t.Fatalf("expected bounded retries at 2 attempts, got %d", qualityStep.Attempts)
	}
	if !hasAuditAction(run.AuditTrail, "command_retry_scheduled") {
		t.Fatalf("expected retry audit action")
	}
	if !hasAuditAction(run.AuditTrail, "command_failed") {
		t.Fatalf("expected command_failed audit action")
	}
}

func hasAuditAction(entries []RunAuditEntry, action string) bool {
	action = strings.TrimSpace(action)
	for _, entry := range entries {
		if strings.TrimSpace(entry.Action) == action {
			return true
		}
	}
	return false
}

func findLastCommandCall(calls []CommandRequest, key string) *CommandRequest {
	key = strings.TrimSpace(key)
	for i := len(calls) - 1; i >= 0; i-- {
		callKey := strings.TrimSpace(calls[i].HookID)
		if callKey == "" {
			callKey = strings.TrimSpace(calls[i].StepID)
		}
		if callKey == key {
			out := calls[i]
			return &out
		}
	}
	return nil
}

func findStep(run *WorkflowRun, stepID string) *StepRun {
	if run == nil {
		return nil
	}
	for p := range run.Phases {
		for s := range run.Phases[p].Steps {
			if run.Phases[p].Steps[s].ID == stepID {
				return &run.Phases[p].Steps[s]
			}
		}
	}
	return nil
}

func TestConventionalCommitPattern(t *testing.T) {
	valid := []string{
		"chore: update workflow output",
		"feat(guided): add retry policy",
		"fix(engine)!: preserve step state",
	}
	for _, message := range valid {
		if !isConventionalCommitMessage(message) {
			t.Fatalf("expected valid conventional commit message: %q", message)
		}
	}
	invalid := []string{
		"",
		"update workflow output",
		"chore update workflow output",
	}
	for _, message := range invalid {
		if isConventionalCommitMessage(message) {
			t.Fatalf("expected invalid conventional commit message: %q", message)
		}
	}
}

func TestNormalizeExecutionControlsDefaults(t *testing.T) {
	controls := NormalizeExecutionControls(ExecutionControls{Enabled: true})
	if controls.RetryPolicy.MaxAttempts != defaultExecutionMaxAttempts {
		t.Fatalf("unexpected default max attempts: %d", controls.RetryPolicy.MaxAttempts)
	}
	if len(controls.Quality.Hooks) != 3 {
		t.Fatalf("expected default tests/lint/typecheck hooks, got %d", len(controls.Quality.Hooks))
	}
	if controls.Capabilities.Commit {
		t.Fatalf("commit capability should stay opt-in by default")
	}
	for i, hook := range controls.Quality.Hooks {
		if strings.TrimSpace(hook.ID) == "" || strings.TrimSpace(hook.Command) == "" {
			t.Fatalf("unexpected empty hook at index %d: %+v", i, hook)
		}
	}
}

func BenchmarkScriptedRunnerKeyLookup(b *testing.B) {
	runner := &scriptedExecutionRunner{
		responses: map[string][]CommandResult{
			"tests": {{ExitCode: 0}},
		},
	}
	req := CommandRequest{StepID: "quality_checks", HookID: "tests"}
	for i := 0; i < b.N; i++ {
		result := runner.Run(context.Background(), req)
		if result.ExitCode != 0 {
			b.Fatalf("unexpected runner exit code: %d", result.ExitCode)
		}
	}
}

func ExampleNormalizeExecutionControls() {
	controls := NormalizeExecutionControls(ExecutionControls{Enabled: true})
	fmt.Println(controls.Enabled, controls.RetryPolicy.MaxAttempts, len(controls.Quality.Hooks))
	// Output: true 2 3
}
