package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"control/internal/config"
	"control/internal/guidedworkflows"
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

func newGuidedWorkflowRunService(coreCfg config.CoreConfig, stores *Stores) guidedworkflows.RunService {
	controls := guidedWorkflowsExecutionControlsFromCoreConfig(coreCfg)
	opts := []guidedworkflows.RunServiceOption{
		guidedworkflows.WithMaxActiveRuns(coreCfg.GuidedWorkflowsRolloutMaxActiveRuns()),
		guidedworkflows.WithTelemetryEnabled(coreCfg.GuidedWorkflowsRolloutTelemetryEnabled()),
	}
	if metricsStore := newGuidedWorkflowMetricsStore(stores); metricsStore != nil {
		opts = append(opts, guidedworkflows.WithRunMetricsStore(metricsStore))
	}
	if controls.Enabled {
		opts = append(opts, guidedworkflows.WithRunExecutionControls(controls))
	}
	return guidedworkflows.NewRunService(guidedWorkflowsConfigFromCoreConfig(coreCfg), opts...)
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
