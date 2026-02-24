package daemon

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/types"
)

type API struct {
	Version                   string
	Manager                   *SessionManager
	Stores                    *Stores
	Shutdown                  func(context.Context) error
	Syncer                    SessionSyncer
	LiveCodex                 *CodexLiveManager
	LiveManager               LiveManager
	CodexHistoryPool          CodexHistoryPool
	Notifier                  NotificationPublisher
	GuidedWorkflows           guidedworkflows.Orchestrator
	WorkflowRuns              GuidedWorkflowRunService
	WorkflowRunMetrics        GuidedWorkflowRunMetricsService
	WorkflowRunMetricsReset   GuidedWorkflowRunMetricsResetService
	WorkflowTemplates         GuidedWorkflowTemplateService
	WorkflowPolicy            GuidedWorkflowPolicyResolver
	WorkflowDispatchDefaults  guidedWorkflowDispatchDefaults
	WorkflowSessionVisibility WorkflowRunSessionVisibilityService
	WorkflowSessionInterrupt  WorkflowRunSessionInterruptService
	WorkflowRunStop           WorkflowRunStopCoordinator
	Logger                    logging.Logger
}

type StartSessionRequest struct {
	Provider              string                           `json:"provider"`
	Cmd                   string                           `json:"cmd,omitempty"`
	Cwd                   string                           `json:"cwd,omitempty"`
	Args                  []string                         `json:"args,omitempty"`
	Env                   []string                         `json:"env,omitempty"`
	Title                 string                           `json:"title,omitempty"`
	Tags                  []string                         `json:"tags,omitempty"`
	WorkspaceID           string                           `json:"workspace_id,omitempty"`
	WorktreeID            string                           `json:"worktree_id,omitempty"`
	Text                  string                           `json:"text,omitempty"`
	RuntimeOptions        *types.SessionRuntimeOptions     `json:"runtime_options,omitempty"`
	NotificationOverrides *types.NotificationSettingsPatch `json:"notification_overrides,omitempty"`
}

type UpdateSessionRequest struct {
	Title                 string                           `json:"title,omitempty"`
	RuntimeOptions        *types.SessionRuntimeOptions     `json:"runtime_options,omitempty"`
	NotificationOverrides *types.NotificationSettingsPatch `json:"notification_overrides,omitempty"`
}

type TailItemsResponse struct {
	Items []map[string]any `json:"items"`
}

type SendSessionRequest struct {
	Text  string           `json:"text,omitempty"`
	Input []map[string]any `json:"input,omitempty"`
}

type SendSessionResponse struct {
	OK     bool   `json:"ok"`
	TurnID string `json:"turn_id,omitempty"`
}

type ApproveSessionRequest struct {
	RequestID      int            `json:"request_id"`
	Decision       string         `json:"decision"`
	Responses      []string       `json:"responses,omitempty"`
	AcceptSettings map[string]any `json:"accept_settings,omitempty"`
}

type CreateWorkflowRunRequest struct {
	TemplateID      string                                    `json:"template_id,omitempty"`
	WorkspaceID     string                                    `json:"workspace_id,omitempty"`
	WorktreeID      string                                    `json:"worktree_id,omitempty"`
	SessionID       string                                    `json:"session_id,omitempty"`
	TaskID          string                                    `json:"task_id,omitempty"`
	UserPrompt      string                                    `json:"user_prompt,omitempty"`
	PolicyOverrides *guidedworkflows.CheckpointPolicyOverride `json:"policy_overrides,omitempty"`
}

type WorkflowRunDecisionRequest struct {
	Action     guidedworkflows.DecisionAction `json:"action"`
	DecisionID string                         `json:"decision_id,omitempty"`
	Note       string                         `json:"note,omitempty"`
}

type WorkflowRunResumeRequest struct {
	ResumeFailed bool   `json:"resume_failed,omitempty"`
	Message      string `json:"message,omitempty"`
}

type WorkflowRunRenameRequest struct {
	Name string `json:"name,omitempty"`
}

type GuidedWorkflowRunService interface {
	CreateRun(ctx context.Context, req guidedworkflows.CreateRunRequest) (*guidedworkflows.WorkflowRun, error)
	ListRuns(ctx context.Context) ([]*guidedworkflows.WorkflowRun, error)
	ListRunsIncludingDismissed(ctx context.Context) ([]*guidedworkflows.WorkflowRun, error)
	RenameRun(ctx context.Context, runID, name string) (*guidedworkflows.WorkflowRun, error)
	StartRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	PauseRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	StopRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	ResumeRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	ResumeFailedRun(ctx context.Context, runID string, req guidedworkflows.ResumeFailedRunRequest) (*guidedworkflows.WorkflowRun, error)
	DismissRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	UndismissRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	HandleDecision(ctx context.Context, runID string, req guidedworkflows.DecisionActionRequest) (*guidedworkflows.WorkflowRun, error)
	GetRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	GetRunTimeline(ctx context.Context, runID string) ([]guidedworkflows.RunTimelineEvent, error)
}

type GuidedWorkflowTemplateService interface {
	ListTemplates(ctx context.Context) ([]guidedworkflows.WorkflowTemplate, error)
}

type GuidedWorkflowRunMetricsService interface {
	GetRunMetrics(ctx context.Context) (guidedworkflows.RunMetricsSnapshot, error)
}

type GuidedWorkflowRunMetricsResetService interface {
	ResetRunMetrics(ctx context.Context) (guidedworkflows.RunMetricsSnapshot, error)
}

type WorkflowRunSessionVisibilityService interface {
	SyncWorkflowRunSessionVisibility(run *guidedworkflows.WorkflowRun, dismissed bool)
}

type WorkflowRunSessionInterruptService interface {
	InterruptWorkflowRunSessions(ctx context.Context, run *guidedworkflows.WorkflowRun) error
}

type WorkflowRunStopCoordinator interface {
	StopWorkflowRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
}

type GuidedWorkflowPolicyResolver interface {
	ResolvePolicyOverrides(explicit *guidedworkflows.CheckpointPolicyOverride) *guidedworkflows.CheckpointPolicyOverride
}

func parseLines(raw string) int {
	if raw == "" {
		return 200
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return 200
	}
	return val
}

func isFollowRequest(r *http.Request) bool {
	return parseBoolQueryValue(r.URL.Query().Get("follow"))
}

func isRefreshRequest(r *http.Request) bool {
	return parseBoolQueryValue(r.URL.Query().Get("refresh"))
}

func parseBoolQueryValue(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return value == "1" || value == "true" || value == "yes"
}

func (a *API) newSessionService() *SessionService {
	opts := make([]SessionServiceOption, 0, 2)
	if a != nil && a.CodexHistoryPool != nil {
		opts = append(opts, WithCodexHistoryPool(a.CodexHistoryPool))
	}
	if a != nil && a.Notifier != nil {
		opts = append(opts, WithNotificationPublisher(a.Notifier))
	}
	if a != nil && a.GuidedWorkflows != nil {
		opts = append(opts, WithGuidedWorkflowOrchestrator(a.GuidedWorkflows))
	}
	if a != nil && a.LiveManager != nil {
		opts = append(opts, WithLiveManager(a.LiveManager))
	}
	return NewSessionService(a.Manager, a.Stores, a.Logger, opts...)
}

func (a *API) workflowRunService() GuidedWorkflowRunService {
	if a == nil {
		return nil
	}
	return a.WorkflowRuns
}

func (a *API) workflowRunMetricsService() GuidedWorkflowRunMetricsService {
	if a == nil {
		return nil
	}
	return a.WorkflowRunMetrics
}

func (a *API) workflowRunMetricsResetService() GuidedWorkflowRunMetricsResetService {
	if a == nil {
		return nil
	}
	return a.WorkflowRunMetricsReset
}

func (a *API) workflowTemplateService() GuidedWorkflowTemplateService {
	if a == nil {
		return nil
	}
	if a.WorkflowTemplates != nil {
		return a.WorkflowTemplates
	}
	if a.WorkflowRuns != nil {
		if provider, ok := any(a.WorkflowRuns).(GuidedWorkflowTemplateService); ok {
			return provider
		}
	}
	return nil
}

func (a *API) workflowPolicyResolver() GuidedWorkflowPolicyResolver {
	if a == nil || a.WorkflowPolicy == nil {
		return guidedWorkflowNoopPolicyResolver{}
	}
	return a.WorkflowPolicy
}

func (a *API) workflowDispatchDefaults() guidedWorkflowDispatchDefaults {
	if a == nil {
		return guidedWorkflowDispatchDefaults{}
	}
	return a.WorkflowDispatchDefaults
}

func (a *API) workflowSessionVisibilityService() WorkflowRunSessionVisibilityService {
	if a == nil {
		return nil
	}
	return a.WorkflowSessionVisibility
}

func (a *API) workflowSessionInterruptService() WorkflowRunSessionInterruptService {
	if a == nil {
		return nil
	}
	return a.WorkflowSessionInterrupt
}

func (a *API) workflowRunStopCoordinator() WorkflowRunStopCoordinator {
	if a == nil {
		return nil
	}
	if a.WorkflowRunStop != nil {
		return a.WorkflowRunStop
	}
	return newWorkflowRunStopCoordinator(a.workflowRunService(), a.workflowSessionInterruptService(), a.Logger)
}
