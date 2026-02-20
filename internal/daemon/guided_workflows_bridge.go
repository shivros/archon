package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"control/internal/config"
	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/providers"
	"control/internal/store"
	"control/internal/types"
)

func guidedWorkflowsConfigFromCoreConfig(cfg config.CoreConfig) guidedworkflows.Config {
	return guidedworkflows.Config{
		Enabled:         cfg.GuidedWorkflowsEnabled(),
		AutoStart:       cfg.GuidedWorkflowsAutoStart(),
		CheckpointStyle: cfg.GuidedWorkflowsCheckpointStyle(),
		Mode:            cfg.GuidedWorkflowsMode(),
		Policy: guidedworkflows.CheckpointPolicy{
			Style:                    cfg.GuidedWorkflowsCheckpointStyle(),
			ConfidenceThreshold:      cfg.GuidedWorkflowsPolicyConfidenceThreshold(),
			PauseThreshold:           cfg.GuidedWorkflowsPolicyPauseThreshold(),
			HighBlastRadiusFileCount: cfg.GuidedWorkflowsPolicyHighBlastRadiusFileCount(),
			HardGates: guidedworkflows.CheckpointPolicyGates{
				AmbiguityBlocker:         cfg.GuidedWorkflowsPolicyHardGateAmbiguityBlocker(),
				ConfidenceBelowThreshold: cfg.GuidedWorkflowsPolicyHardGateConfidenceBelowThreshold(),
				HighBlastRadius:          cfg.GuidedWorkflowsPolicyHardGateHighBlastRadius(),
				SensitiveFiles:           cfg.GuidedWorkflowsPolicyHardGateSensitiveFiles(),
				PreCommitApproval:        cfg.GuidedWorkflowsPolicyHardGatePreCommitApproval(),
				FailingChecks:            cfg.GuidedWorkflowsPolicyHardGateFailingChecks(),
			},
			ConditionalGates: guidedworkflows.CheckpointPolicyGates{
				AmbiguityBlocker:         cfg.GuidedWorkflowsPolicyConditionalGateAmbiguityBlocker(),
				ConfidenceBelowThreshold: cfg.GuidedWorkflowsPolicyConditionalGateConfidenceBelowThreshold(),
				HighBlastRadius:          cfg.GuidedWorkflowsPolicyConditionalGateHighBlastRadius(),
				SensitiveFiles:           cfg.GuidedWorkflowsPolicyConditionalGateSensitiveFiles(),
				PreCommitApproval:        cfg.GuidedWorkflowsPolicyConditionalGatePreCommitApproval(),
				FailingChecks:            cfg.GuidedWorkflowsPolicyConditionalGateFailingChecks(),
			},
		},
	}
}

func newGuidedWorkflowOrchestrator(coreCfg config.CoreConfig) guidedworkflows.Orchestrator {
	return guidedworkflows.New(guidedWorkflowsConfigFromCoreConfig(coreCfg))
}

func newGuidedWorkflowRunService(
	coreCfg config.CoreConfig,
	stores *Stores,
	manager *SessionManager,
	live *CodexLiveManager,
	logger logging.Logger,
) guidedworkflows.RunService {
	controls := guidedWorkflowsExecutionControlsFromCoreConfig(coreCfg)
	opts := []guidedworkflows.RunServiceOption{
		guidedworkflows.WithMaxActiveRuns(coreCfg.GuidedWorkflowsRolloutMaxActiveRuns()),
		guidedworkflows.WithTelemetryEnabled(coreCfg.GuidedWorkflowsRolloutTelemetryEnabled()),
	}
	workflowTemplatesPath, pathErr := config.WorkflowTemplatesPath()
	if pathErr != nil {
		workflowTemplatesPath = ""
	}
	if templateProvider := newGuidedWorkflowTemplateProvider(workflowTemplatesPath); templateProvider != nil {
		opts = append(opts, guidedworkflows.WithTemplateProvider(templateProvider))
	}
	if metricsStore := newGuidedWorkflowMetricsStore(stores); metricsStore != nil {
		opts = append(opts, guidedworkflows.WithRunMetricsStore(metricsStore))
	}
	if promptDispatcher := newGuidedWorkflowPromptDispatcher(coreCfg, manager, stores, live, logger); promptDispatcher != nil {
		opts = append(opts, guidedworkflows.WithStepPromptDispatcher(promptDispatcher))
	}
	if controls.Enabled {
		opts = append(opts, guidedworkflows.WithRunExecutionControls(controls))
	}
	return guidedworkflows.NewRunService(guidedWorkflowsConfigFromCoreConfig(coreCfg), opts...)
}

type guidedWorkflowTemplateProvider struct {
	path string
}

func newGuidedWorkflowTemplateProvider(path string) guidedworkflows.TemplateProvider {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return &guidedWorkflowTemplateProvider{path: strings.TrimSpace(path)}
}

func (p *guidedWorkflowTemplateProvider) ListWorkflowTemplates(ctx context.Context) ([]guidedworkflows.WorkflowTemplate, error) {
	if p == nil || strings.TrimSpace(p.path) == "" {
		return nil, nil
	}
	return store.NewFileWorkflowTemplateStore(strings.TrimSpace(p.path)).ListWorkflowTemplates(ctx)
}

func (p *guidedWorkflowTemplateProvider) HasWorkflowTemplateConfig(context.Context) (bool, error) {
	if p == nil || strings.TrimSpace(p.path) == "" {
		return false, nil
	}
	_, err := os.Stat(strings.TrimSpace(p.path))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

type guidedWorkflowSessionGateway interface {
	ListWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error)
	ListWithMetaIncludingWorkflowOwned(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error)
	SendMessage(ctx context.Context, id string, input []map[string]any) (string, error)
}

type guidedWorkflowSessionStarter interface {
	Start(ctx context.Context, req StartSessionRequest) (*types.Session, error)
}

type guidedWorkflowPromptDispatcher struct {
	sessions    guidedWorkflowSessionGateway
	sessionMeta SessionMetaStore
	defaults    guidedWorkflowDispatchDefaults
	logger      logging.Logger
}

type guidedWorkflowDispatchDefaults struct {
	Provider  string
	Model     string
	Access    types.AccessLevel
	Reasoning types.ReasoningLevel
}

func newGuidedWorkflowPromptDispatcher(
	coreCfg config.CoreConfig,
	manager *SessionManager,
	stores *Stores,
	live *CodexLiveManager,
	logger logging.Logger,
) guidedworkflows.StepPromptDispatcher {
	if manager == nil || stores == nil {
		return nil
	}
	return &guidedWorkflowPromptDispatcher{
		sessions:    NewSessionService(manager, stores, live, logger),
		sessionMeta: stores.SessionMeta,
		defaults:    guidedWorkflowDispatchDefaultsFromCoreConfig(coreCfg),
		logger:      logger,
	}
}

func (d *guidedWorkflowPromptDispatcher) DispatchStepPrompt(
	ctx context.Context,
	req guidedworkflows.StepPromptDispatchRequest,
) (guidedworkflows.StepPromptDispatchResult, error) {
	if d == nil || d.sessions == nil {
		return guidedworkflows.StepPromptDispatchResult{}, fmt.Errorf("%w: session gateway unavailable", guidedworkflows.ErrStepDispatch)
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return guidedworkflows.StepPromptDispatchResult{}, fmt.Errorf("%w: prompt is empty", guidedworkflows.ErrStepDispatch)
	}
	sessionID, provider, model, err := d.resolveSession(ctx, req)
	if err != nil {
		return guidedworkflows.StepPromptDispatchResult{}, wrapStepDispatchError(err)
	}
	if strings.TrimSpace(sessionID) == "" {
		return guidedworkflows.StepPromptDispatchResult{}, fmt.Errorf(
			"%w: no dispatchable session found for workspace=%q worktree=%q",
			guidedworkflows.ErrStepDispatch,
			strings.TrimSpace(req.WorkspaceID),
			strings.TrimSpace(req.WorktreeID),
		)
	}
	if !guidedWorkflowProviderSupportsPromptDispatch(provider) {
		return guidedworkflows.StepPromptDispatchResult{}, fmt.Errorf(
			"%w: provider %q does not support step prompt dispatch",
			guidedworkflows.ErrStepDispatch,
			strings.TrimSpace(provider),
		)
	}
	turnID, err := d.sendStepPrompt(ctx, sessionID, prompt)
	if err != nil && shouldFallbackToReplacementWorkflowSession(err) {
		fallbackSessionID, fallbackProvider, fallbackModel, startErr := d.startWorkflowSession(ctx, req, nil, nil)
		if startErr == nil && strings.TrimSpace(fallbackSessionID) != "" && strings.TrimSpace(fallbackSessionID) != strings.TrimSpace(sessionID) {
			fallbackTurnID, fallbackErr := d.sendStepPrompt(ctx, fallbackSessionID, prompt)
			if fallbackErr == nil {
				sessionID = strings.TrimSpace(fallbackSessionID)
				provider = strings.TrimSpace(fallbackProvider)
				model = strings.TrimSpace(fallbackModel)
				turnID = strings.TrimSpace(fallbackTurnID)
				err = nil
			} else {
				err = fallbackErr
			}
		}
	}
	if err != nil {
		return guidedworkflows.StepPromptDispatchResult{}, wrapStepDispatchError(err)
	}
	d.linkSessionToWorkflow(ctx, sessionID, req.RunID)
	return guidedworkflows.StepPromptDispatchResult{
		Dispatched: true,
		SessionID:  sessionID,
		TurnID:     strings.TrimSpace(turnID),
		Provider:   strings.TrimSpace(provider),
		Model:      strings.TrimSpace(model),
	}, nil
}

func (d *guidedWorkflowPromptDispatcher) sendStepPrompt(ctx context.Context, sessionID string, prompt string) (string, error) {
	if d == nil || d.sessions == nil {
		return "", fmt.Errorf("%w: session gateway unavailable", guidedworkflows.ErrStepDispatch)
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("%w: session id is required", guidedworkflows.ErrStepDispatch)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("%w: prompt is empty", guidedworkflows.ErrStepDispatch)
	}
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		turnID, err := d.sessions.SendMessage(ctx, sessionID, []map[string]any{
			{"type": "text", "text": prompt},
		})
		if err == nil {
			return strings.TrimSpace(turnID), nil
		}
		lastErr = err
		if !isTurnAlreadyInProgressError(err) || attempt == maxAttempts {
			break
		}
		delay := time.Duration(attempt*150) * time.Millisecond
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(delay):
		}
	}
	return "", lastErr
}

func (d *guidedWorkflowPromptDispatcher) linkSessionToWorkflow(ctx context.Context, sessionID, runID string) {
	if d == nil || d.sessionMeta == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	if sessionID == "" || runID == "" {
		return
	}
	_, _ = d.sessionMeta.Upsert(ctx, &types.SessionMeta{
		SessionID:     sessionID,
		WorkflowRunID: runID,
	})
}

func guidedWorkflowProviderSupportsPromptDispatch(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex", "opencode":
		return true
	default:
		return false
	}
}

func (d *guidedWorkflowPromptDispatcher) resolveSession(
	ctx context.Context,
	req guidedworkflows.StepPromptDispatchRequest,
) (string, string, string, error) {
	explicitSessionID := strings.TrimSpace(req.SessionID)
	sessions, meta, err := d.sessions.ListWithMetaIncludingWorkflowOwned(ctx)
	if err != nil {
		return "", "", "", err
	}
	metaBySessionID := make(map[string]*types.SessionMeta, len(meta))
	for _, item := range meta {
		if item == nil {
			continue
		}
		metaBySessionID[strings.TrimSpace(item.SessionID)] = item
	}
	if explicitSessionID != "" {
		for _, session := range sessions {
			if session == nil {
				continue
			}
			if strings.TrimSpace(session.ID) == explicitSessionID {
				provider := strings.TrimSpace(session.Provider)
				model := sessionModel(metaBySessionID[explicitSessionID])
				if guidedWorkflowProviderSupportsPromptDispatch(provider) {
					return explicitSessionID, provider, model, nil
				}
				fallbackSessionID, fallbackProvider, fallbackModel, err := d.startWorkflowSession(ctx, req, sessions, metaBySessionID)
				if err != nil {
					return "", "", "", err
				}
				if strings.TrimSpace(fallbackSessionID) != "" {
					return strings.TrimSpace(fallbackSessionID), strings.TrimSpace(fallbackProvider), strings.TrimSpace(fallbackModel), nil
				}
				return "", "", "", fmt.Errorf(
					"%w: explicit session %q uses unsupported provider %q",
					guidedworkflows.ErrStepDispatch,
					explicitSessionID,
					provider,
				)
			}
		}
		fallbackSessionID, fallbackProvider, fallbackModel, err := d.startWorkflowSession(ctx, req, sessions, metaBySessionID)
		if err != nil {
			return "", "", "", err
		}
		if strings.TrimSpace(fallbackSessionID) != "" {
			return strings.TrimSpace(fallbackSessionID), strings.TrimSpace(fallbackProvider), strings.TrimSpace(fallbackModel), nil
		}
		return "", "", "", fmt.Errorf("%w: explicit session %q not found", guidedworkflows.ErrStepDispatch, explicitSessionID)
	}
	workflowSessionID, workflowProvider, workflowModel := d.resolveOwnedWorkflowSession(req, sessions, metaBySessionID)
	if workflowSessionID != "" {
		return workflowSessionID, workflowProvider, workflowModel, nil
	}
	return d.startWorkflowSession(ctx, req, sessions, metaBySessionID)
}

func (d *guidedWorkflowPromptDispatcher) resolveOwnedWorkflowSession(
	req guidedworkflows.StepPromptDispatchRequest,
	sessions []*types.Session,
	metaBySessionID map[string]*types.SessionMeta,
) (string, string, string) {
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		return "", "", ""
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	worktreeID := strings.TrimSpace(req.WorktreeID)
	var selectedSessionID string
	var selectedProvider string
	var selectedModel string
	var selectedAt time.Time
	for _, session := range sessions {
		if session == nil || !isGuidedWorkflowDispatchableSessionStatus(session.Status) {
			continue
		}
		sessionID := strings.TrimSpace(session.ID)
		if sessionID == "" {
			continue
		}
		meta := metaBySessionID[sessionID]
		if meta == nil || strings.TrimSpace(meta.WorkflowRunID) != runID {
			continue
		}
		if worktreeID != "" && strings.TrimSpace(meta.WorktreeID) != worktreeID {
			continue
		}
		if worktreeID == "" && workspaceID != "" && strings.TrimSpace(meta.WorkspaceID) != workspaceID {
			continue
		}
		provider := strings.TrimSpace(session.Provider)
		if !guidedWorkflowProviderSupportsPromptDispatch(provider) {
			continue
		}
		candidateAt := session.CreatedAt
		if meta.LastActiveAt != nil {
			candidateAt = meta.LastActiveAt.UTC()
		}
		if selectedSessionID == "" || candidateAt.After(selectedAt) {
			selectedSessionID = sessionID
			selectedProvider = provider
			selectedModel = sessionModel(meta)
			selectedAt = candidateAt
		}
	}
	return selectedSessionID, selectedProvider, selectedModel
}

func (d *guidedWorkflowPromptDispatcher) startWorkflowSession(
	ctx context.Context,
	req guidedworkflows.StepPromptDispatchRequest,
	sessions []*types.Session,
	metaBySessionID map[string]*types.SessionMeta,
) (string, string, string, error) {
	if d == nil || d.sessions == nil {
		return "", "", "", nil
	}
	starter, ok := d.sessions.(guidedWorkflowSessionStarter)
	if !ok || starter == nil {
		return "", "", "", nil
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	worktreeID := strings.TrimSpace(req.WorktreeID)
	if workspaceID == "" && worktreeID == "" {
		return "", "", "", nil
	}
	settings := d.resolveWorkflowSessionStartSettings(req, sessions, metaBySessionID)
	d.logWorkflowSessionStartRequested(req, settings)
	session, err := starter.Start(ctx, StartSessionRequest{
		Provider:       settings.Provider,
		Title:          guidedWorkflowSessionTitle(req.RunID),
		WorkspaceID:    workspaceID,
		WorktreeID:     worktreeID,
		RuntimeOptions: settings.RuntimeOptions,
	})
	if err != nil {
		return "", "", "", err
	}
	if session == nil {
		return "", "", "", nil
	}
	sessionID := strings.TrimSpace(session.ID)
	sessionProvider := strings.TrimSpace(session.Provider)
	d.logWorkflowSessionStarted(req, sessionID, sessionProvider, settings)
	return sessionID, sessionProvider, settings.Model, nil
}

func (d *guidedWorkflowPromptDispatcher) resolveWorkflowSessionProvider(
	workspaceID string,
	worktreeID string,
	sessions []*types.Session,
	metaBySessionID map[string]*types.SessionMeta,
) string {
	provider := d.preferredProviderForContext(workspaceID, worktreeID, sessions, metaBySessionID)
	if configuredProvider := strings.TrimSpace(d.defaults.Provider); configuredProvider != "" {
		provider = configuredProvider
	}
	return normalizeGuidedWorkflowDispatchProvider(provider)
}

type guidedWorkflowSessionStartSettings struct {
	Provider       string
	Model          string
	Access         types.AccessLevel
	Reasoning      types.ReasoningLevel
	RuntimeOptions *types.SessionRuntimeOptions
}

func (d *guidedWorkflowPromptDispatcher) resolveWorkflowSessionStartSettings(
	req guidedworkflows.StepPromptDispatchRequest,
	sessions []*types.Session,
	metaBySessionID map[string]*types.SessionMeta,
) guidedWorkflowSessionStartSettings {
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	worktreeID := strings.TrimSpace(req.WorktreeID)
	provider := d.resolveWorkflowSessionProvider(workspaceID, worktreeID, sessions, metaBySessionID)
	settings := guidedWorkflowEffectiveDispatchSettings(req.DefaultAccessLevel, d.defaults)
	settings.Provider = provider
	return settings
}

func (d *guidedWorkflowPromptDispatcher) logWorkflowSessionStartRequested(
	req guidedworkflows.StepPromptDispatchRequest,
	settings guidedWorkflowSessionStartSettings,
) {
	if d == nil || d.logger == nil {
		return
	}
	d.logger.Info("guided_workflow_session_start_requested",
		logging.F("run_id", strings.TrimSpace(req.RunID)),
		logging.F("template_id", strings.TrimSpace(req.TemplateID)),
		logging.F("workspace_id", strings.TrimSpace(req.WorkspaceID)),
		logging.F("worktree_id", strings.TrimSpace(req.WorktreeID)),
		logging.F("effective_provider", settings.Provider),
		logging.F("effective_model", settings.Model),
		logging.F("effective_access", settings.Access),
		logging.F("effective_reasoning", settings.Reasoning),
	)
}

func (d *guidedWorkflowPromptDispatcher) logWorkflowSessionStarted(
	req guidedworkflows.StepPromptDispatchRequest,
	sessionID string,
	sessionProvider string,
	settings guidedWorkflowSessionStartSettings,
) {
	if d == nil || d.logger == nil {
		return
	}
	d.logger.Info("guided_workflow_session_started",
		logging.F("run_id", strings.TrimSpace(req.RunID)),
		logging.F("template_id", strings.TrimSpace(req.TemplateID)),
		logging.F("workspace_id", strings.TrimSpace(req.WorkspaceID)),
		logging.F("worktree_id", strings.TrimSpace(req.WorktreeID)),
		logging.F("session_id", strings.TrimSpace(sessionID)),
		logging.F("effective_provider", strings.TrimSpace(sessionProvider)),
		logging.F("effective_model", settings.Model),
		logging.F("effective_access", settings.Access),
		logging.F("effective_reasoning", settings.Reasoning),
	)
}

func (d *guidedWorkflowPromptDispatcher) preferredProviderForContext(
	workspaceID string,
	worktreeID string,
	sessions []*types.Session,
	metaBySessionID map[string]*types.SessionMeta,
) string {
	var selectedProvider string
	var selectedAt time.Time
	for _, session := range sessions {
		if session == nil {
			continue
		}
		provider := strings.TrimSpace(session.Provider)
		if !guidedWorkflowProviderSupportsPromptDispatch(provider) {
			continue
		}
		sessionID := strings.TrimSpace(session.ID)
		if sessionID == "" {
			continue
		}
		meta := metaBySessionID[sessionID]
		if meta == nil {
			continue
		}
		if worktreeID != "" {
			if strings.TrimSpace(meta.WorktreeID) != worktreeID {
				continue
			}
		} else if workspaceID != "" && strings.TrimSpace(meta.WorkspaceID) != workspaceID {
			continue
		}
		candidateAt := session.CreatedAt
		if meta.LastActiveAt != nil {
			candidateAt = meta.LastActiveAt.UTC()
		}
		if selectedProvider == "" || candidateAt.After(selectedAt) {
			selectedProvider = provider
			selectedAt = candidateAt
		}
	}
	return selectedProvider
}

func guidedWorkflowSessionTitle(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "guided workflow"
	}
	return "guided workflow " + runID
}

func isGuidedWorkflowDispatchableSessionStatus(status types.SessionStatus) bool {
	switch status {
	case types.SessionStatusCreated, types.SessionStatusStarting, types.SessionStatusRunning, types.SessionStatusInactive:
		return true
	default:
		return false
	}
}

func sessionModel(meta *types.SessionMeta) string {
	if meta == nil || meta.RuntimeOptions == nil {
		return ""
	}
	return strings.TrimSpace(meta.RuntimeOptions.Model)
}

func guidedWorkflowDispatchDefaultsFromCoreConfig(cfg config.CoreConfig) guidedWorkflowDispatchDefaults {
	out := guidedWorkflowDispatchDefaults{
		Provider:  providers.Normalize(cfg.GuidedWorkflowsDefaultProvider()),
		Model:     strings.TrimSpace(cfg.GuidedWorkflowsDefaultModel()),
		Access:    cfg.GuidedWorkflowsDefaultAccessLevel(),
		Reasoning: cfg.GuidedWorkflowsDefaultReasoningLevel(),
	}
	if !guidedWorkflowProviderSupportsPromptDispatch(out.Provider) {
		out.Provider = ""
	}
	return out
}

func normalizeGuidedWorkflowDispatchProvider(provider string) string {
	normalized := providers.Normalize(provider)
	if normalized == "" {
		return "codex"
	}
	if !guidedWorkflowProviderSupportsPromptDispatch(normalized) {
		return "codex"
	}
	return normalized
}

func guidedWorkflowEffectiveDispatchSettings(level types.AccessLevel, defaults guidedWorkflowDispatchDefaults) guidedWorkflowSessionStartSettings {
	runtimeOptions := guidedWorkflowRuntimeOptionsForDispatch(level, defaults)
	settings := guidedWorkflowSessionStartSettings{
		Provider:       normalizeGuidedWorkflowDispatchProvider(defaults.Provider),
		Model:          guidedWorkflowDispatchModel(runtimeOptions),
		RuntimeOptions: runtimeOptions,
	}
	if runtimeOptions == nil {
		return settings
	}
	settings.Access = runtimeOptions.Access
	settings.Reasoning = runtimeOptions.Reasoning
	return settings
}

func guidedWorkflowRuntimeOptionsForDispatch(level types.AccessLevel, defaults guidedWorkflowDispatchDefaults) *types.SessionRuntimeOptions {
	options := &types.SessionRuntimeOptions{
		Model:     strings.TrimSpace(defaults.Model),
		Reasoning: defaults.Reasoning,
	}
	normalized, ok := guidedworkflows.NormalizeTemplateAccessLevel(level)
	if ok && normalized != "" {
		options.Access = normalized
	} else {
		options.Access = defaults.Access
	}
	if options.Model == "" && options.Reasoning == "" && options.Access == "" {
		return nil
	}
	return options
}

func guidedWorkflowDispatchModel(options *types.SessionRuntimeOptions) string {
	if options == nil {
		return ""
	}
	return strings.TrimSpace(options.Model)
}

func wrapStepDispatchError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, guidedworkflows.ErrStepDispatch) {
		return err
	}
	return fmt.Errorf("%w: %v", guidedworkflows.ErrStepDispatch, err)
}

func isTurnAlreadyInProgressError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "turn already in progress")
}

func shouldFallbackToReplacementWorkflowSession(err error) bool {
	if err == nil {
		return false
	}
	return isTurnAlreadyInProgressError(err) || isCodexMissingThreadError(err)
}

func guidedWorkflowsExecutionControlsFromCoreConfig(cfg config.CoreConfig) guidedworkflows.ExecutionControls {
	if !cfg.GuidedWorkflowsRolloutAutomationEnabled() {
		return guidedworkflows.ExecutionControls{}
	}
	allowQuality := cfg.GuidedWorkflowsRolloutAllowQualityChecks()
	allowCommit := cfg.GuidedWorkflowsRolloutAllowCommit()
	return guidedworkflows.ExecutionControls{
		Enabled: true,
		Capabilities: guidedworkflows.ExecutionCapabilities{
			QualityChecks: allowQuality,
			Commit:        allowCommit,
		},
		RetryPolicy: guidedworkflows.RetryPolicy{
			MaxAttempts: cfg.GuidedWorkflowsRolloutMaxRetryAttempts(),
		},
		Quality: guidedworkflows.QualityGateConfig{
			Enabled: allowQuality,
		},
		Commit: guidedworkflows.CommitConfig{
			Enabled:         allowCommit,
			RequireApproval: cfg.GuidedWorkflowsRolloutRequireCommitApproval(),
		},
	}
}

// NewGuidedWorkflowNotificationPublisher forwards existing notifications and
// routes turn-completed events to guided workflows when the feature is enabled.
func NewGuidedWorkflowNotificationPublisher(
	downstream NotificationPublisher,
	orchestrator guidedworkflows.Orchestrator,
	turnProcessors ...guidedworkflows.TurnEventProcessor,
) NotificationPublisher {
	var turnProcessor guidedworkflows.TurnEventProcessor
	for _, candidate := range turnProcessors {
		if candidate != nil {
			turnProcessor = candidate
			break
		}
	}
	if (orchestrator == nil || !orchestrator.Enabled()) && turnProcessor == nil {
		return downstream
	}
	return &guidedWorkflowNotificationPublisher{
		downstream:         downstream,
		orchestrator:       orchestrator,
		turnProcessor:      turnProcessor,
		decisionNotifiedBy: map[string]struct{}{},
	}
}

type guidedWorkflowNotificationPublisher struct {
	downstream         NotificationPublisher
	orchestrator       guidedworkflows.Orchestrator
	turnProcessor      guidedworkflows.TurnEventProcessor
	mu                 sync.Mutex
	decisionNotifiedBy map[string]struct{}
}

func (p *guidedWorkflowNotificationPublisher) Publish(event types.NotificationEvent) {
	if p.downstream != nil {
		p.downstream.Publish(event)
	}
	if event.Trigger != types.NotificationTriggerTurnCompleted {
		return
	}
	if p.orchestrator != nil && p.orchestrator.Enabled() {
		p.orchestrator.OnTurnEvent(context.Background(), event)
	}
	if p.turnProcessor == nil {
		return
	}
	updatedRuns, err := p.turnProcessor.OnTurnCompleted(context.Background(), guidedworkflows.TurnSignal{
		SessionID:   strings.TrimSpace(event.SessionID),
		WorkspaceID: strings.TrimSpace(event.WorkspaceID),
		WorktreeID:  strings.TrimSpace(event.WorktreeID),
		TurnID:      strings.TrimSpace(event.TurnID),
	})
	if err != nil {
		return
	}
	for _, run := range updatedRuns {
		p.publishDecisionNeeded(event, run)
	}
}

func (p *guidedWorkflowNotificationPublisher) publishDecisionNeeded(turnEvent types.NotificationEvent, run *guidedworkflows.WorkflowRun) {
	if p == nil || p.downstream == nil || run == nil {
		return
	}
	notification, ok := guidedWorkflowDecisionNotificationEvent(&turnEvent, run)
	if !ok {
		return
	}
	decisionID := strings.TrimSpace(run.LatestDecision.ID)
	key := strings.TrimSpace(run.ID) + "|" + decisionID
	p.mu.Lock()
	if _, exists := p.decisionNotifiedBy[key]; exists {
		p.mu.Unlock()
		return
	}
	p.decisionNotifiedBy[key] = struct{}{}
	p.mu.Unlock()
	p.downstream.Publish(notification)
}

func guidedWorkflowDecisionNotificationEvent(turnEvent *types.NotificationEvent, run *guidedworkflows.WorkflowRun) (types.NotificationEvent, bool) {
	if run == nil || run.Status != guidedworkflows.WorkflowRunStatusPaused {
		return types.NotificationEvent{}, false
	}
	if run.LatestDecision == nil || run.LatestDecision.Metadata.Action != guidedworkflows.CheckpointActionPause {
		return types.NotificationEvent{}, false
	}
	decisionID := strings.TrimSpace(run.LatestDecision.ID)
	if decisionID == "" {
		return types.NotificationEvent{}, false
	}
	turnID := ""
	provider := ""
	sessionID := strings.TrimSpace(run.SessionID)
	workspaceID := strings.TrimSpace(run.WorkspaceID)
	worktreeID := strings.TrimSpace(run.WorktreeID)
	if turnEvent != nil {
		turnID = strings.TrimSpace(turnEvent.TurnID)
		provider = strings.TrimSpace(turnEvent.Provider)
		sessionID = firstNonEmpty(sessionID, turnEvent.SessionID)
		workspaceID = firstNonEmpty(workspaceID, turnEvent.WorkspaceID)
		worktreeID = firstNonEmpty(worktreeID, turnEvent.WorktreeID)
	}
	metadata := run.LatestDecision.Metadata
	payload := map[string]any{
		"kind":               "guided_workflow_decision_needed",
		"run_id":             strings.TrimSpace(run.ID),
		"decision_id":        decisionID,
		"phase_id":           strings.TrimSpace(run.LatestDecision.PhaseID),
		"step_id":            strings.TrimSpace(run.LatestDecision.StepID),
		"reason":             strings.TrimSpace(run.LatestDecision.Reason),
		"confidence":         metadata.Confidence,
		"risk_summary":       fmt.Sprintf("severity=%s tier=%s score=%.2f pause_threshold=%.2f", metadata.Severity, metadata.Tier, metadata.Score, metadata.PauseThreshold),
		"recommended_action": recommendedDecisionAction(metadata),
		"actions": []string{
			string(guidedworkflows.DecisionActionApproveContinue),
			string(guidedworkflows.DecisionActionRequestRevision),
			string(guidedworkflows.DecisionActionPauseRun),
		},
		"trigger_reasons": metadata.Reasons,
		"turn_id":         turnID,
	}
	notification := types.NotificationEvent{
		Trigger:     types.NotificationTriggerTurnCompleted,
		OccurredAt:  time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:   sessionID,
		WorkspaceID: workspaceID,
		WorktreeID:  worktreeID,
		Provider:    provider,
		Title:       "guided workflow checkpoint",
		Status:      "decision_needed",
		Source:      "guided_workflow_decision:" + strings.TrimSpace(run.ID) + ":" + decisionID,
		Payload:     payload,
	}
	return notification, true
}

func recommendedDecisionAction(metadata guidedworkflows.CheckpointDecisionMetadata) string {
	if metadata.HardGateTriggered {
		return string(guidedworkflows.DecisionActionRequestRevision)
	}
	switch metadata.Severity {
	case guidedworkflows.DecisionSeverityHigh, guidedworkflows.DecisionSeverityCritical:
		return string(guidedworkflows.DecisionActionRequestRevision)
	default:
		return string(guidedworkflows.DecisionActionApproveContinue)
	}
}

func firstNonEmpty(primary, secondary string) string {
	value := strings.TrimSpace(primary)
	if value != "" {
		return value
	}
	return strings.TrimSpace(secondary)
}
