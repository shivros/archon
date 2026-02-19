package guidedworkflows

import (
	"context"
	"strings"
)

const (
	defaultExecutionMaxAttempts = 2
	maxExecutionMaxAttempts     = 5
	maxExecutionOutputLength    = 1024
)

type ExecutionCapabilities struct {
	QualityChecks bool `json:"quality_checks"`
	Commit        bool `json:"commit"`
}

type RetryPolicy struct {
	MaxAttempts int `json:"max_attempts"`
}

type QualityHook struct {
	ID       string `json:"id"`
	Command  string `json:"command"`
	Required bool   `json:"required"`
}

type QualityGateConfig struct {
	Enabled bool          `json:"enabled"`
	Hooks   []QualityHook `json:"hooks,omitempty"`
}

type CommitConfig struct {
	Enabled         bool   `json:"enabled"`
	RequireApproval bool   `json:"require_approval"`
	Message         string `json:"message,omitempty"`
}

type ExecutionControls struct {
	Enabled      bool                  `json:"enabled"`
	Capabilities ExecutionCapabilities `json:"capabilities"`
	RetryPolicy  RetryPolicy           `json:"retry_policy"`
	Quality      QualityGateConfig     `json:"quality"`
	Commit       CommitConfig          `json:"commit"`
}

type CommandRequest struct {
	RunID    string            `json:"run_id"`
	PhaseID  string            `json:"phase_id,omitempty"`
	StepID   string            `json:"step_id"`
	HookID   string            `json:"hook_id,omitempty"`
	Command  string            `json:"command"`
	Attempt  int               `json:"attempt"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type CommandResult struct {
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output,omitempty"`
	Retryable bool   `json:"retryable"`
	Err       error  `json:"-"`
}

type ExecutionRunner interface {
	Run(ctx context.Context, req CommandRequest) CommandResult
}

type noopExecutionRunner struct{}

func (noopExecutionRunner) Run(context.Context, CommandRequest) CommandResult {
	return CommandResult{ExitCode: 0, Retryable: false, Output: "noop runner: success"}
}

func NormalizeExecutionControls(in ExecutionControls) ExecutionControls {
	out := in
	if !out.Enabled {
		out.RetryPolicy.MaxAttempts = 1
		if out.Quality.Hooks != nil {
			out.Quality.Hooks = append([]QualityHook{}, out.Quality.Hooks...)
		}
		return out
	}
	if out.RetryPolicy.MaxAttempts <= 0 {
		out.RetryPolicy.MaxAttempts = defaultExecutionMaxAttempts
	}
	if out.RetryPolicy.MaxAttempts > maxExecutionMaxAttempts {
		out.RetryPolicy.MaxAttempts = maxExecutionMaxAttempts
	}
	if !out.Quality.Enabled {
		out.Quality.Enabled = true
	}
	if len(out.Quality.Hooks) == 0 {
		out.Quality.Hooks = defaultQualityHooks()
	} else {
		out.Quality.Hooks = normalizeQualityHooks(out.Quality.Hooks)
	}
	if !out.Commit.Enabled {
		out.Commit.Enabled = true
	}
	out.Commit.Message = strings.TrimSpace(out.Commit.Message)
	return out
}

func defaultQualityHooks() []QualityHook {
	return []QualityHook{
		{ID: "tests", Command: "go test ./...", Required: true},
		{ID: "lint", Command: "go test ./...", Required: true},
		{ID: "typecheck", Command: "go test ./...", Required: true},
	}
}

func normalizeQualityHooks(in []QualityHook) []QualityHook {
	if len(in) == 0 {
		return nil
	}
	out := make([]QualityHook, 0, len(in))
	for _, hook := range in {
		id := strings.TrimSpace(hook.ID)
		command := strings.TrimSpace(hook.Command)
		if id == "" || command == "" {
			continue
		}
		out = append(out, QualityHook{
			ID:       id,
			Command:  command,
			Required: hook.Required,
		})
	}
	if len(out) == 0 {
		return defaultQualityHooks()
	}
	return out
}

func clampOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= maxExecutionOutputLength {
		return output
	}
	return strings.TrimSpace(output[:maxExecutionOutputLength]) + "..."
}
