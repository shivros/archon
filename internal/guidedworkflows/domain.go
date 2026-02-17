package guidedworkflows

import (
	"context"
	"time"
)

const (
	TemplateIDSolidPhaseDelivery = "solid_phase_delivery"
)

type WorkflowTemplate struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Phases      []WorkflowTemplatePhase `json:"phases"`
}

type WorkflowTemplatePhase struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Steps []WorkflowTemplateStep `json:"steps"`
}

type WorkflowTemplateStep struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prompt string `json:"prompt,omitempty"`
}

type WorkflowRunStatus string

const (
	WorkflowRunStatusCreated   WorkflowRunStatus = "created"
	WorkflowRunStatusRunning   WorkflowRunStatus = "running"
	WorkflowRunStatusPaused    WorkflowRunStatus = "paused"
	WorkflowRunStatusCompleted WorkflowRunStatus = "completed"
	WorkflowRunStatusFailed    WorkflowRunStatus = "failed"
)

type PhaseRunStatus string

const (
	PhaseRunStatusPending   PhaseRunStatus = "pending"
	PhaseRunStatusRunning   PhaseRunStatus = "running"
	PhaseRunStatusCompleted PhaseRunStatus = "completed"
	PhaseRunStatusFailed    PhaseRunStatus = "failed"
)

type StepRunStatus string

const (
	StepRunStatusPending   StepRunStatus = "pending"
	StepRunStatusRunning   StepRunStatus = "running"
	StepRunStatusCompleted StepRunStatus = "completed"
	StepRunStatusFailed    StepRunStatus = "failed"
)

type WorkflowRun struct {
	ID                  string                    `json:"id"`
	TemplateID          string                    `json:"template_id"`
	TemplateName        string                    `json:"template_name"`
	WorkspaceID         string                    `json:"workspace_id,omitempty"`
	WorktreeID          string                    `json:"worktree_id,omitempty"`
	SessionID           string                    `json:"session_id,omitempty"`
	TaskID              string                    `json:"task_id,omitempty"`
	UserPrompt          string                    `json:"user_prompt,omitempty"`
	Mode                string                    `json:"mode"`
	CheckpointStyle     string                    `json:"checkpoint_style"`
	Status              WorkflowRunStatus         `json:"status"`
	CreatedAt           time.Time                 `json:"created_at"`
	StartedAt           *time.Time                `json:"started_at,omitempty"`
	PausedAt            *time.Time                `json:"paused_at,omitempty"`
	CompletedAt         *time.Time                `json:"completed_at,omitempty"`
	CurrentPhaseIndex   int                       `json:"current_phase_index"`
	CurrentStepIndex    int                       `json:"current_step_index"`
	Policy              CheckpointPolicy          `json:"policy"`
	PolicyOverrides     *CheckpointPolicyOverride `json:"policy_overrides,omitempty"`
	Phases              []PhaseRun                `json:"phases"`
	LatestDecision      *CheckpointDecision       `json:"latest_decision,omitempty"`
	CheckpointDecisions []CheckpointDecision      `json:"checkpoint_decisions,omitempty"`
	AuditTrail          []RunAuditEntry           `json:"audit_trail,omitempty"`
	LastError           string                    `json:"last_error,omitempty"`
}

type PhaseRun struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Status      PhaseRunStatus `json:"status"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Steps       []StepRun      `json:"steps"`
}

type StepRun struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Prompt            string             `json:"prompt,omitempty"`
	Status            StepRunStatus      `json:"status"`
	AwaitingTurn      bool               `json:"awaiting_turn,omitempty"`
	TurnID            string             `json:"turn_id,omitempty"`
	StartedAt         *time.Time         `json:"started_at,omitempty"`
	CompletedAt       *time.Time         `json:"completed_at,omitempty"`
	Attempts          int                `json:"attempts,omitempty"`
	Outcome           string             `json:"outcome,omitempty"`
	Output            string             `json:"output,omitempty"`
	Error             string             `json:"error,omitempty"`
	Execution         *StepExecutionRef  `json:"execution,omitempty"`
	ExecutionAttempts []StepExecutionRef `json:"execution_attempts,omitempty"`
	ExecutionState    StepExecutionState `json:"execution_state,omitempty"`
	ExecutionMessage  string             `json:"execution_message,omitempty"`
}

type StepExecutionState string

const (
	StepExecutionStateNone        StepExecutionState = "none"
	StepExecutionStateLinked      StepExecutionState = "linked"
	StepExecutionStateUnavailable StepExecutionState = "unavailable"
)

type StepExecutionRef struct {
	TraceID        string     `json:"trace_id,omitempty"`
	SessionID      string     `json:"session_id,omitempty"`
	SessionScope   string     `json:"session_scope,omitempty"`
	Provider       string     `json:"provider,omitempty"`
	Model          string     `json:"model,omitempty"`
	TurnID         string     `json:"turn_id,omitempty"`
	PromptSnapshot string     `json:"prompt_snapshot,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

type CheckpointDecision struct {
	ID          string                     `json:"id"`
	RunID       string                     `json:"run_id"`
	PhaseID     string                     `json:"phase_id,omitempty"`
	StepID      string                     `json:"step_id,omitempty"`
	Decision    string                     `json:"decision"`
	Reason      string                     `json:"reason,omitempty"`
	Source      string                     `json:"source,omitempty"`
	RequestedAt time.Time                  `json:"requested_at"`
	DecidedAt   *time.Time                 `json:"decided_at,omitempty"`
	Metadata    CheckpointDecisionMetadata `json:"metadata"`
}

type RunTimelineEvent struct {
	At      time.Time `json:"at"`
	Type    string    `json:"type"`
	RunID   string    `json:"run_id"`
	PhaseID string    `json:"phase_id,omitempty"`
	StepID  string    `json:"step_id,omitempty"`
	Message string    `json:"message,omitempty"`
}

type RunAuditEntry struct {
	At      time.Time `json:"at"`
	RunID   string    `json:"run_id"`
	PhaseID string    `json:"phase_id,omitempty"`
	StepID  string    `json:"step_id,omitempty"`
	Scope   string    `json:"scope"`
	Action  string    `json:"action"`
	Attempt int       `json:"attempt,omitempty"`
	Outcome string    `json:"outcome,omitempty"`
	Detail  string    `json:"detail,omitempty"`
}

type CreateRunRequest struct {
	TemplateID      string                    `json:"template_id,omitempty"`
	WorkspaceID     string                    `json:"workspace_id,omitempty"`
	WorktreeID      string                    `json:"worktree_id,omitempty"`
	SessionID       string                    `json:"session_id,omitempty"`
	TaskID          string                    `json:"task_id,omitempty"`
	UserPrompt      string                    `json:"user_prompt,omitempty"`
	PolicyOverrides *CheckpointPolicyOverride `json:"policy_overrides,omitempty"`
}

type DecisionAction string

const (
	DecisionActionApproveContinue DecisionAction = "approve_continue"
	DecisionActionRequestRevision DecisionAction = "request_revision"
	DecisionActionPauseRun        DecisionAction = "pause_run"
)

type DecisionActionRequest struct {
	Action     DecisionAction `json:"action"`
	DecisionID string         `json:"decision_id,omitempty"`
	Note       string         `json:"note,omitempty"`
}

type TurnSignal struct {
	SessionID   string `json:"session_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	WorktreeID  string `json:"worktree_id,omitempty"`
	TurnID      string `json:"turn_id,omitempty"`
}

type StepPromptDispatchRequest struct {
	RunID       string `json:"run_id"`
	TemplateID  string `json:"template_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	WorktreeID  string `json:"worktree_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	PhaseID     string `json:"phase_id,omitempty"`
	StepID      string `json:"step_id,omitempty"`
	Prompt      string `json:"prompt"`
}

type StepPromptDispatchResult struct {
	Dispatched bool   `json:"dispatched"`
	SessionID  string `json:"session_id,omitempty"`
	TurnID     string `json:"turn_id,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
}

type StepPromptDispatcher interface {
	DispatchStepPrompt(ctx context.Context, req StepPromptDispatchRequest) (StepPromptDispatchResult, error)
}

type RunStatusSnapshot struct {
	Run      *WorkflowRun       `json:"run"`
	Timeline []RunTimelineEvent `json:"timeline"`
}

type RunMetricsSnapshot struct {
	Enabled              bool           `json:"enabled"`
	CapturedAt           time.Time      `json:"captured_at"`
	RunsStarted          int            `json:"runs_started"`
	RunsCompleted        int            `json:"runs_completed"`
	RunsFailed           int            `json:"runs_failed"`
	PauseCount           int            `json:"pause_count"`
	PauseRate            float64        `json:"pause_rate"`
	ApprovalCount        int            `json:"approval_count"`
	ApprovalLatencyAvgMS int64          `json:"approval_latency_avg_ms"`
	ApprovalLatencyMaxMS int64          `json:"approval_latency_max_ms"`
	InterventionCauses   map[string]int `json:"intervention_causes,omitempty"`
}

type CheckpointAction string

const (
	CheckpointActionContinue CheckpointAction = "continue"
	CheckpointActionPause    CheckpointAction = "pause"
)

type DecisionSeverity string

const (
	DecisionSeverityLow      DecisionSeverity = "low"
	DecisionSeverityMedium   DecisionSeverity = "medium"
	DecisionSeverityHigh     DecisionSeverity = "high"
	DecisionSeverityCritical DecisionSeverity = "critical"
)

type DecisionTier string

const (
	DecisionTier0 DecisionTier = "tier_0"
	DecisionTier1 DecisionTier = "tier_1"
	DecisionTier2 DecisionTier = "tier_2"
	DecisionTier3 DecisionTier = "tier_3"
)

type CheckpointReason struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	HardGate bool   `json:"hard_gate"`
}

type CheckpointDecisionMetadata struct {
	Action              CheckpointAction   `json:"action"`
	Reasons             []CheckpointReason `json:"reasons,omitempty"`
	Severity            DecisionSeverity   `json:"severity"`
	Tier                DecisionTier       `json:"tier"`
	Style               string             `json:"style"`
	Confidence          float64            `json:"confidence"`
	ConfidenceThreshold float64            `json:"confidence_threshold"`
	Score               float64            `json:"score"`
	PauseThreshold      float64            `json:"pause_threshold"`
	HardGateTriggered   bool               `json:"hard_gate_triggered"`
	EvaluatedAt         time.Time          `json:"evaluated_at"`
}

type CheckpointPolicy struct {
	Style                    string                `json:"style"`
	ConfidenceThreshold      float64               `json:"confidence_threshold"`
	PauseThreshold           float64               `json:"pause_threshold"`
	HighBlastRadiusFileCount int                   `json:"high_blast_radius_file_count"`
	HardGates                CheckpointPolicyGates `json:"hard_gates"`
	ConditionalGates         CheckpointPolicyGates `json:"conditional_gates"`
}

type CheckpointPolicyGates struct {
	AmbiguityBlocker         bool `json:"ambiguity_blocker"`
	ConfidenceBelowThreshold bool `json:"confidence_below_threshold"`
	HighBlastRadius          bool `json:"high_blast_radius"`
	SensitiveFiles           bool `json:"sensitive_files"`
	PreCommitApproval        bool `json:"pre_commit_approval"`
	FailingChecks            bool `json:"failing_checks"`
}

type CheckpointPolicyOverride struct {
	Style                    *string                        `json:"style,omitempty"`
	ConfidenceThreshold      *float64                       `json:"confidence_threshold,omitempty"`
	PauseThreshold           *float64                       `json:"pause_threshold,omitempty"`
	HighBlastRadiusFileCount *int                           `json:"high_blast_radius_file_count,omitempty"`
	HardGates                *CheckpointPolicyGatesOverride `json:"hard_gates,omitempty"`
	ConditionalGates         *CheckpointPolicyGatesOverride `json:"conditional_gates,omitempty"`
}

type CheckpointPolicyGatesOverride struct {
	AmbiguityBlocker         *bool `json:"ambiguity_blocker,omitempty"`
	ConfidenceBelowThreshold *bool `json:"confidence_below_threshold,omitempty"`
	HighBlastRadius          *bool `json:"high_blast_radius,omitempty"`
	SensitiveFiles           *bool `json:"sensitive_files,omitempty"`
	PreCommitApproval        *bool `json:"pre_commit_approval,omitempty"`
	FailingChecks            *bool `json:"failing_checks,omitempty"`
}

type PolicyEvaluationInput struct {
	Confidence                *float64 `json:"confidence,omitempty"`
	AmbiguityDetected         bool     `json:"ambiguity_detected,omitempty"`
	BlockerDetected           bool     `json:"blocker_detected,omitempty"`
	FailingChecks             bool     `json:"failing_checks,omitempty"`
	PreCommitApprovalRequired bool     `json:"pre_commit_approval_required,omitempty"`
	ChangedFiles              int      `json:"changed_files,omitempty"`
	SensitiveFiles            []string `json:"sensitive_files,omitempty"`
}
