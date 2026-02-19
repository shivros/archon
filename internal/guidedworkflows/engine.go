package guidedworkflows

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrRunNotFound       = errors.New("workflow run not found")
	ErrTemplateNotFound  = errors.New("workflow template not found")
	ErrInvalidTransition = errors.New("invalid workflow run transition")
	ErrRunNotRunning     = errors.New("workflow run is not running")
	ErrNoPendingSteps    = errors.New("workflow run has no pending steps")
	ErrRunLimitExceeded  = errors.New("workflow active run limit exceeded")
	ErrCapabilityDenied  = errors.New("workflow capability denied")
	ErrCommandFailed     = errors.New("workflow command failed")
	ErrStepDispatch      = errors.New("workflow step prompt dispatch unavailable")
)

type StepHandler func(ctx context.Context, run *WorkflowRun, phase *PhaseRun, step *StepRun) error

type Engine struct {
	now      func() time.Time
	handlers map[string]StepHandler
	controls ExecutionControls
	runner   ExecutionRunner
}

type EngineOption func(*Engine)

func WithStepHandler(stepID string, handler StepHandler) EngineOption {
	return func(e *Engine) {
		if e == nil {
			return
		}
		id := strings.TrimSpace(stepID)
		if id == "" || handler == nil {
			return
		}
		e.handlers[id] = handler
	}
}

func WithExecutionControls(controls ExecutionControls) EngineOption {
	return func(e *Engine) {
		if e == nil {
			return
		}
		e.controls = NormalizeExecutionControls(controls)
	}
}

func WithExecutionRunner(runner ExecutionRunner) EngineOption {
	return func(e *Engine) {
		if e == nil || runner == nil {
			return
		}
		e.runner = runner
	}
}

func NewEngine(opts ...EngineOption) *Engine {
	e := &Engine{
		now:      func() time.Time { return time.Now().UTC() },
		handlers: map[string]StepHandler{},
		controls: NormalizeExecutionControls(ExecutionControls{}),
		runner:   noopExecutionRunner{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	if e.runner == nil {
		e.runner = noopExecutionRunner{}
	}
	e.controls = NormalizeExecutionControls(e.controls)
	return e
}

func (e *Engine) Advance(ctx context.Context, run *WorkflowRun, timeline *[]RunTimelineEvent) error {
	if e == nil {
		e = NewEngine()
	}
	if run == nil {
		return fmt.Errorf("%w: run is required", ErrInvalidTransition)
	}
	if run.Status != WorkflowRunStatusRunning {
		return fmt.Errorf("%w: %w", ErrInvalidTransition, ErrRunNotRunning)
	}

	phaseIndex, stepIndex, ok := findNextPending(run)
	if !ok {
		now := e.now()
		run.Status = WorkflowRunStatusCompleted
		run.CompletedAt = &now
		run.LastError = ""
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "run",
			Action:  "run_completed",
			Outcome: "success",
			Detail:  "all workflow steps completed",
		})
		appendTimelineEvent(timeline, RunTimelineEvent{
			At:    now,
			Type:  "run_completed",
			RunID: run.ID,
		})
		return nil
	}
	run.CurrentPhaseIndex = phaseIndex
	run.CurrentStepIndex = stepIndex

	phase := &run.Phases[phaseIndex]
	if phase.Status == PhaseRunStatusPending {
		now := e.now()
		phase.Status = PhaseRunStatusRunning
		phase.StartedAt = &now
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "phase",
			Action:  "phase_started",
			PhaseID: phase.ID,
			Outcome: "running",
			Detail:  phase.Name,
		})
		appendTimelineEvent(timeline, RunTimelineEvent{
			At:      now,
			Type:    "phase_started",
			RunID:   run.ID,
			PhaseID: phase.ID,
		})
	}

	step := &phase.Steps[stepIndex]
	now := e.now()
	step.Status = StepRunStatusRunning
	step.StartedAt = &now
	step.CompletedAt = nil
	step.Error = ""
	step.Outcome = "running"
	step.Output = ""
	step.Attempts = 0
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "step",
		Action:  "step_started",
		PhaseID: phase.ID,
		StepID:  step.ID,
		Outcome: "running",
		Detail:  step.Name,
	})
	appendTimelineEvent(timeline, RunTimelineEvent{
		At:      now,
		Type:    "step_started",
		RunID:   run.ID,
		PhaseID: phase.ID,
		StepID:  step.ID,
	})

	handler := e.handlerFor(step.ID)
	if err := handler(ctx, run, phase, step); err != nil {
		finished := e.now()
		step.Status = StepRunStatusFailed
		step.CompletedAt = &finished
		step.Error = err.Error()
		step.Outcome = "failed"
		phase.Status = PhaseRunStatusFailed
		phase.CompletedAt = &finished
		run.Status = WorkflowRunStatusFailed
		run.CompletedAt = &finished
		run.LastError = err.Error()
		appendRunAudit(run, RunAuditEntry{
			At:      finished,
			Scope:   "step",
			Action:  "step_failed",
			PhaseID: phase.ID,
			StepID:  step.ID,
			Attempt: step.Attempts,
			Outcome: "failed",
			Detail:  err.Error(),
		})
		appendRunAudit(run, RunAuditEntry{
			At:      finished,
			Scope:   "run",
			Action:  "run_failed",
			PhaseID: phase.ID,
			StepID:  step.ID,
			Outcome: "failed",
			Detail:  err.Error(),
		})
		appendTimelineEvent(timeline, RunTimelineEvent{
			At:      finished,
			Type:    "step_failed",
			RunID:   run.ID,
			PhaseID: phase.ID,
			StepID:  step.ID,
			Message: err.Error(),
		})
		appendTimelineEvent(timeline, RunTimelineEvent{
			At:      finished,
			Type:    "run_failed",
			RunID:   run.ID,
			Message: err.Error(),
		})
		return err
	}

	finished := e.now()
	step.Status = StepRunStatusCompleted
	step.CompletedAt = &finished
	step.Error = ""
	step.Outcome = "success"
	appendRunAudit(run, RunAuditEntry{
		At:      finished,
		Scope:   "step",
		Action:  "step_completed",
		PhaseID: phase.ID,
		StepID:  step.ID,
		Attempt: step.Attempts,
		Outcome: "success",
		Detail:  step.Name,
	})
	appendTimelineEvent(timeline, RunTimelineEvent{
		At:      finished,
		Type:    "step_completed",
		RunID:   run.ID,
		PhaseID: phase.ID,
		StepID:  step.ID,
	})

	if phaseComplete(phase) {
		phase.Status = PhaseRunStatusCompleted
		phase.CompletedAt = &finished
		appendRunAudit(run, RunAuditEntry{
			At:      finished,
			Scope:   "phase",
			Action:  "phase_completed",
			PhaseID: phase.ID,
			Outcome: "success",
			Detail:  phase.Name,
		})
		appendTimelineEvent(timeline, RunTimelineEvent{
			At:      finished,
			Type:    "phase_completed",
			RunID:   run.ID,
			PhaseID: phase.ID,
		})
	}

	nextPhase, nextStep, hasNext := findNextPending(run)
	if hasNext {
		run.CurrentPhaseIndex = nextPhase
		run.CurrentStepIndex = nextStep
		return nil
	}

	run.Status = WorkflowRunStatusCompleted
	run.CompletedAt = &finished
	run.LastError = ""
	appendRunAudit(run, RunAuditEntry{
		At:      finished,
		Scope:   "run",
		Action:  "run_completed",
		Outcome: "success",
		Detail:  "all workflow steps completed",
	})
	appendTimelineEvent(timeline, RunTimelineEvent{
		At:    finished,
		Type:  "run_completed",
		RunID: run.ID,
	})
	return nil
}

func (e *Engine) handlerFor(stepID string) StepHandler {
	if e != nil {
		if handler, ok := e.handlers[strings.TrimSpace(stepID)]; ok && handler != nil {
			return handler
		}
		if handler := e.builtinHandler(strings.TrimSpace(stepID)); handler != nil {
			return handler
		}
	}
	return noopStepHandler
}

func (e *Engine) builtinHandler(stepID string) StepHandler {
	if e == nil || !e.controls.Enabled {
		return nil
	}
	switch strings.TrimSpace(stepID) {
	case "quality_checks":
		return e.qualityChecksHandler
	case "commit":
		return e.commitStepHandler
	default:
		return nil
	}
}

func (e *Engine) qualityChecksHandler(ctx context.Context, run *WorkflowRun, phase *PhaseRun, step *StepRun) error {
	if e == nil || !e.controls.Enabled || !e.controls.Quality.Enabled {
		return nil
	}
	if !e.controls.Capabilities.QualityChecks {
		appendRunAudit(run, RunAuditEntry{
			At:      e.now(),
			Scope:   "step",
			Action:  "quality_checks_capability_denied",
			PhaseID: phase.ID,
			StepID:  step.ID,
			Outcome: "failed",
			Detail:  "quality checks capability is disabled",
		})
		return fmt.Errorf("%w: quality checks capability is disabled", ErrCapabilityDenied)
	}
	for _, hook := range e.controls.Quality.Hooks {
		hookID := strings.TrimSpace(hook.ID)
		command := strings.TrimSpace(hook.Command)
		if hookID == "" || command == "" {
			continue
		}
		if err := e.executeCommandWithRetry(ctx, run, phase, step, hookID, command, nil); err != nil {
			if hook.Required {
				return err
			}
			appendRunAudit(run, RunAuditEntry{
				At:      e.now(),
				Scope:   "step",
				Action:  "quality_hook_optional_failed",
				PhaseID: phase.ID,
				StepID:  step.ID,
				Outcome: "ignored",
				Detail:  hookID + ": " + err.Error(),
			})
		}
	}
	return nil
}

func (e *Engine) commitStepHandler(ctx context.Context, run *WorkflowRun, phase *PhaseRun, step *StepRun) error {
	if e == nil || !e.controls.Enabled || !e.controls.Commit.Enabled {
		return nil
	}
	if !e.controls.Capabilities.Commit {
		appendRunAudit(run, RunAuditEntry{
			At:      e.now(),
			Scope:   "step",
			Action:  "commit_capability_denied",
			PhaseID: phase.ID,
			StepID:  step.ID,
			Outcome: "failed",
			Detail:  "commit capability is disabled",
		})
		return fmt.Errorf("%w: commit capability is disabled", ErrCapabilityDenied)
	}
	message := e.commitMessageForRun(run)
	if !isConventionalCommitMessage(message) {
		appendRunAudit(run, RunAuditEntry{
			At:      e.now(),
			Scope:   "step",
			Action:  "commit_message_invalid",
			PhaseID: phase.ID,
			StepID:  step.ID,
			Outcome: "failed",
			Detail:  message,
		})
		return fmt.Errorf("%w: commit message must follow conventional commit format", ErrInvalidTransition)
	}
	command := fmt.Sprintf("git commit -m %q", message)
	metadata := map[string]string{
		"commit_message": message,
	}
	if err := e.executeCommandWithRetry(ctx, run, phase, step, "commit", command, metadata); err != nil {
		return err
	}
	return nil
}

func (e *Engine) executeCommandWithRetry(
	ctx context.Context,
	run *WorkflowRun,
	phase *PhaseRun,
	step *StepRun,
	hookID string,
	command string,
	metadata map[string]string,
) error {
	if e == nil {
		return fmt.Errorf("%w: engine is nil", ErrInvalidTransition)
	}
	maxAttempts := e.controls.RetryPolicy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		step.Attempts++
		started := e.now()
		appendRunAudit(run, RunAuditEntry{
			At:      started,
			Scope:   "step",
			Action:  "command_started",
			PhaseID: phase.ID,
			StepID:  step.ID,
			Attempt: attempt,
			Outcome: "running",
			Detail:  hookID + ": " + command,
		})
		result := e.runner.Run(ctx, CommandRequest{
			RunID:    run.ID,
			PhaseID:  phase.ID,
			StepID:   step.ID,
			HookID:   hookID,
			Command:  command,
			Attempt:  attempt,
			Metadata: metadata,
		})
		output := clampOutput(result.Output)
		if output != "" {
			step.Output = output
		}
		if result.Err == nil && result.ExitCode == 0 {
			step.Outcome = "success"
			appendRunAudit(run, RunAuditEntry{
				At:      e.now(),
				Scope:   "step",
				Action:  "command_succeeded",
				PhaseID: phase.ID,
				StepID:  step.ID,
				Attempt: attempt,
				Outcome: "success",
				Detail:  hookID,
			})
			return nil
		}

		detail := strings.TrimSpace(output)
		if detail == "" && result.Err != nil {
			detail = strings.TrimSpace(result.Err.Error())
		}
		if detail == "" && result.ExitCode != 0 {
			detail = fmt.Sprintf("exit code %d", result.ExitCode)
		}
		if detail == "" {
			detail = "unknown command failure"
		}

		retryable := result.Retryable
		if !retryable && result.Err != nil {
			retryable = true
		}
		if !retryable && result.ExitCode != 0 {
			retryable = true
		}
		if attempt < maxAttempts && retryable {
			step.Outcome = "retrying"
			appendRunAudit(run, RunAuditEntry{
				At:      e.now(),
				Scope:   "step",
				Action:  "command_retry_scheduled",
				PhaseID: phase.ID,
				StepID:  step.ID,
				Attempt: attempt,
				Outcome: "retrying",
				Detail:  detail,
			})
			continue
		}
		step.Outcome = "failed"
		appendRunAudit(run, RunAuditEntry{
			At:      e.now(),
			Scope:   "step",
			Action:  "command_failed",
			PhaseID: phase.ID,
			StepID:  step.ID,
			Attempt: attempt,
			Outcome: "failed",
			Detail:  detail,
		})
		return fmt.Errorf("%w: %s (%s)", ErrCommandFailed, detail, hookID)
	}
	return fmt.Errorf("%w: retries exhausted", ErrCommandFailed)
}

func (e *Engine) commitMessageForRun(run *WorkflowRun) string {
	if e != nil && strings.TrimSpace(e.controls.Commit.Message) != "" {
		return strings.TrimSpace(e.controls.Commit.Message)
	}
	runID := ""
	if run != nil {
		runID = strings.TrimSpace(run.ID)
	}
	if runID == "" {
		runID = "unknown"
	}
	return fmt.Sprintf("chore(guided-workflow): complete run %s", runID)
}

var conventionalCommitPattern = regexp.MustCompile(`^[a-z]+(\([^)]+\))?!?:\s.+`)

func isConventionalCommitMessage(message string) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
	}
	return conventionalCommitPattern.MatchString(message)
}

func appendRunAudit(run *WorkflowRun, entry RunAuditEntry) {
	if run == nil {
		return
	}
	entry.RunID = strings.TrimSpace(run.ID)
	if entry.At.IsZero() {
		entry.At = time.Now().UTC()
	}
	entry.Scope = strings.TrimSpace(entry.Scope)
	entry.Action = strings.TrimSpace(entry.Action)
	entry.Outcome = strings.TrimSpace(entry.Outcome)
	entry.Detail = strings.TrimSpace(entry.Detail)
	run.AuditTrail = append(run.AuditTrail, entry)
}

func (e *Engine) executionControls() ExecutionControls {
	if e == nil {
		return NormalizeExecutionControls(ExecutionControls{})
	}
	return NormalizeExecutionControls(e.controls)
}

func noopStepHandler(context.Context, *WorkflowRun, *PhaseRun, *StepRun) error {
	return nil
}

func phaseComplete(phase *PhaseRun) bool {
	if phase == nil || len(phase.Steps) == 0 {
		return true
	}
	for _, step := range phase.Steps {
		if step.Status != StepRunStatusCompleted {
			return false
		}
	}
	return true
}

func findNextPending(run *WorkflowRun) (phaseIndex int, stepIndex int, ok bool) {
	if run == nil {
		return 0, 0, false
	}
	for pIndex, phase := range run.Phases {
		for sIndex, step := range phase.Steps {
			if step.Status == StepRunStatusPending {
				return pIndex, sIndex, true
			}
		}
	}
	return 0, 0, false
}

func appendTimelineEvent(timeline *[]RunTimelineEvent, event RunTimelineEvent) {
	if timeline == nil {
		return
	}
	*timeline = append(*timeline, event)
}
