package guidedworkflows

import (
	"context"
	"errors"
	"strings"

	"control/internal/types"
)

const (
	DefaultCheckpointStyle = "confidence_weighted"
	DefaultMode            = "guarded_autopilot"
)

var (
	ErrDisabled       = errors.New("guided workflows are disabled")
	ErrMissingContext = errors.New("workspace_id or worktree_id is required")
)

type Config struct {
	Enabled         bool
	AutoStart       bool
	CheckpointStyle string
	Mode            string
	Policy          CheckpointPolicy
}

type StartRunRequest struct {
	WorkspaceID string
	WorktreeID  string
	SessionID   string
	TaskID      string
	UserPrompt  string
}

type Run = WorkflowRun

// Orchestrator is intentionally narrow so it can be extracted to a plugin boundary later.
type Orchestrator interface {
	Enabled() bool
	Config() Config
	StartRun(ctx context.Context, req StartRunRequest) (*Run, error)
	OnTurnEvent(ctx context.Context, event types.NotificationEvent)
}

func New(cfg Config) Orchestrator {
	cfg = NormalizeConfig(cfg)
	if !cfg.Enabled {
		return noopOrchestrator{cfg: cfg}
	}
	return &guardedAutopilotOrchestrator{cfg: cfg, runs: NewRunService(cfg)}
}

func NormalizeConfig(cfg Config) Config {
	cfg.CheckpointStyle = normalizeCheckpointStyle(cfg.CheckpointStyle)
	cfg.Mode = normalizeMode(cfg.Mode)
	if checkpointPolicyIsZero(cfg.Policy) {
		cfg.Policy = DefaultCheckpointPolicy(cfg.CheckpointStyle)
	}
	cfg.Policy = NormalizeCheckpointPolicy(cfg.Policy)
	cfg.Policy.Style = cfg.CheckpointStyle
	return cfg
}

func normalizeMode(raw string) string {
	switch normalizeValue(raw) {
	case "guarded_autopilot":
		return "guarded_autopilot"
	default:
		return DefaultMode
	}
}

func normalizeValue(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}

func checkpointPolicyIsZero(policy CheckpointPolicy) bool {
	return strings.TrimSpace(policy.Style) == "" &&
		policy.ConfidenceThreshold == 0 &&
		policy.PauseThreshold == 0 &&
		policy.HighBlastRadiusFileCount == 0 &&
		!policy.HardGates.AmbiguityBlocker &&
		!policy.HardGates.ConfidenceBelowThreshold &&
		!policy.HardGates.HighBlastRadius &&
		!policy.HardGates.SensitiveFiles &&
		!policy.HardGates.PreCommitApproval &&
		!policy.HardGates.FailingChecks &&
		!policy.ConditionalGates.AmbiguityBlocker &&
		!policy.ConditionalGates.ConfidenceBelowThreshold &&
		!policy.ConditionalGates.HighBlastRadius &&
		!policy.ConditionalGates.SensitiveFiles &&
		!policy.ConditionalGates.PreCommitApproval &&
		!policy.ConditionalGates.FailingChecks
}

type noopOrchestrator struct {
	cfg Config
}

func (o noopOrchestrator) Enabled() bool {
	return false
}

func (o noopOrchestrator) Config() Config {
	cfg := o.cfg
	cfg.Enabled = false
	return cfg
}

func (o noopOrchestrator) StartRun(context.Context, StartRunRequest) (*Run, error) {
	return nil, ErrDisabled
}

func (o noopOrchestrator) OnTurnEvent(context.Context, types.NotificationEvent) {
}

type guardedAutopilotOrchestrator struct {
	cfg  Config
	runs RunService
}

func (o *guardedAutopilotOrchestrator) Enabled() bool {
	return o != nil
}

func (o *guardedAutopilotOrchestrator) Config() Config {
	if o == nil {
		return NormalizeConfig(Config{})
	}
	return o.cfg
}

func (o *guardedAutopilotOrchestrator) StartRun(ctx context.Context, req StartRunRequest) (*Run, error) {
	if o == nil {
		return nil, ErrDisabled
	}
	if strings.TrimSpace(req.WorkspaceID) == "" && strings.TrimSpace(req.WorktreeID) == "" {
		return nil, ErrMissingContext
	}
	if o.runs == nil {
		return nil, ErrDisabled
	}
	run, err := o.runs.CreateRun(ctx, CreateRunRequest{
		TemplateID:  TemplateIDSolidPhaseDelivery,
		WorkspaceID: strings.TrimSpace(req.WorkspaceID),
		WorktreeID:  strings.TrimSpace(req.WorktreeID),
		SessionID:   strings.TrimSpace(req.SessionID),
		TaskID:      strings.TrimSpace(req.TaskID),
		UserPrompt:  strings.TrimSpace(req.UserPrompt),
	})
	if err != nil {
		return nil, err
	}
	return o.runs.StartRun(ctx, run.ID)
}

func (o *guardedAutopilotOrchestrator) OnTurnEvent(context.Context, types.NotificationEvent) {
	if o == nil {
		return
	}
}
