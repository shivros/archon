package daemon

import (
	"context"
	"fmt"
	"testing"
	"time"

	"control/internal/config"
	"control/internal/guidedworkflows"
	"control/internal/types"
)

type stubWorkflowTemplateStore struct {
	templates []guidedworkflows.WorkflowTemplate
}

type stubGuidedWorkflowSessionGateway struct {
	sessions  []*types.Session
	meta      []*types.SessionMeta
	sendErr   error
	turnID    string
	sendCalls []struct {
		sessionID string
		input     []map[string]any
	}
}

func (s *stubWorkflowTemplateStore) ListWorkflowTemplates(context.Context) ([]guidedworkflows.WorkflowTemplate, error) {
	out := make([]guidedworkflows.WorkflowTemplate, len(s.templates))
	copy(out, s.templates)
	return out, nil
}

func (s *stubGuidedWorkflowSessionGateway) ListWithMeta(context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return s.sessions, s.meta, nil
}

func (s *stubGuidedWorkflowSessionGateway) SendMessage(_ context.Context, id string, input []map[string]any) (string, error) {
	s.sendCalls = append(s.sendCalls, struct {
		sessionID string
		input     []map[string]any
	}{
		sessionID: id,
		input:     input,
	})
	if s.sendErr != nil {
		return "", s.sendErr
	}
	return s.turnID, nil
}

func TestGuidedWorkflowsConfigFromCoreConfigDefaults(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	out := guidedWorkflowsConfigFromCoreConfig(cfg)
	if out.Enabled {
		t.Fatalf("expected guided workflows disabled by default")
	}
	if out.AutoStart {
		t.Fatalf("expected guided workflows auto_start disabled by default")
	}
	if out.CheckpointStyle != "confidence_weighted" {
		t.Fatalf("unexpected checkpoint style: %q", out.CheckpointStyle)
	}
	if out.Mode != "guarded_autopilot" {
		t.Fatalf("unexpected mode: %q", out.Mode)
	}
	if out.Policy.ConfidenceThreshold != 0.70 {
		t.Fatalf("unexpected policy confidence threshold: %v", out.Policy.ConfidenceThreshold)
	}
	if out.Policy.PauseThreshold != 0.60 {
		t.Fatalf("unexpected policy pause threshold: %v", out.Policy.PauseThreshold)
	}
	if out.Policy.HighBlastRadiusFileCount != 20 {
		t.Fatalf("unexpected policy high blast radius file count: %d", out.Policy.HighBlastRadiusFileCount)
	}
	if !out.Policy.HardGates.AmbiguityBlocker || !out.Policy.ConditionalGates.HighBlastRadius {
		t.Fatalf("unexpected policy default gates: %#v", out.Policy)
	}
	controls := guidedWorkflowsExecutionControlsFromCoreConfig(cfg)
	if controls.Enabled {
		t.Fatalf("expected execution controls disabled by default rollout policy")
	}
}

func TestGuidedWorkflowsExecutionControlsFromCoreConfig(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Rollout.AutomationEnabled = boolPtr(true)
	cfg.GuidedWorkflows.Rollout.AllowQualityChecks = boolPtr(true)
	cfg.GuidedWorkflows.Rollout.AllowCommit = boolPtr(true)
	cfg.GuidedWorkflows.Rollout.RequireCommitApproval = boolPtr(false)
	cfg.GuidedWorkflows.Rollout.MaxRetryAttempts = 4

	controls := guidedWorkflowsExecutionControlsFromCoreConfig(cfg)
	if !controls.Enabled {
		t.Fatalf("expected execution controls to be enabled")
	}
	if !controls.Capabilities.QualityChecks || !controls.Quality.Enabled {
		t.Fatalf("expected quality automation enabled, got %#v", controls)
	}
	if !controls.Capabilities.Commit || !controls.Commit.Enabled {
		t.Fatalf("expected commit automation enabled, got %#v", controls)
	}
	if controls.Commit.RequireApproval {
		t.Fatalf("expected commit approval requirement disabled via rollout override")
	}
	if controls.RetryPolicy.MaxAttempts != 4 {
		t.Fatalf("unexpected retry max attempts: %d", controls.RetryPolicy.MaxAttempts)
	}
}

func TestNewGuidedWorkflowNotificationPublisherSkipsWrapperWhenDisabled(t *testing.T) {
	downstream := &recordNotificationPublisher{}
	orchestrator := &recordGuidedWorkflowOrchestrator{}

	got := NewGuidedWorkflowNotificationPublisher(downstream, orchestrator)
	if got != downstream {
		t.Fatalf("expected downstream publisher to be returned unchanged when orchestrator disabled")
	}
}

func TestNewGuidedWorkflowRunServiceBuildsService(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	service := newGuidedWorkflowRunService(cfg, nil, nil, nil, nil)
	if service == nil {
		t.Fatalf("expected run service")
	}
	if _, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}); err == nil {
		t.Fatalf("expected disabled guided workflows to reject run creation")
	}
}

func TestNewGuidedWorkflowRunServiceAppliesRolloutGuardrails(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Enabled = boolPtr(true)
	cfg.GuidedWorkflows.Rollout.TelemetryEnabled = boolPtr(false)
	cfg.GuidedWorkflows.Rollout.MaxActiveRuns = 1

	service := newGuidedWorkflowRunService(cfg, nil, nil, nil, nil)
	if service == nil {
		t.Fatalf("expected run service")
	}
	if _, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}); err != nil {
		t.Fatalf("unexpected create run error: %v", err)
	}
	if _, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	}); err == nil {
		t.Fatalf("expected create run to fail when max_active_runs guardrail is reached")
	}
	metricsProvider, ok := any(service).(guidedWorkflowRunMetricsProvider)
	if !ok {
		t.Fatalf("expected run service to expose metrics provider")
	}
	metrics, err := metricsProvider.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.Enabled {
		t.Fatalf("expected rollout telemetry_enabled=false to disable telemetry")
	}
}

func TestNewGuidedWorkflowRunServiceLoadsTemplatesFromStore(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Enabled = boolPtr(true)
	custom := guidedworkflows.WorkflowTemplate{
		ID:   "custom_flow",
		Name: "Custom Flow",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{
						ID:     "step_1",
						Name:   "Step 1",
						Prompt: "custom prompt",
					},
				},
			},
		},
	}
	stores := &Stores{
		WorkflowTemplates: &stubWorkflowTemplateStore{templates: []guidedworkflows.WorkflowTemplate{custom}},
	}

	service := newGuidedWorkflowRunService(cfg, stores, nil, nil, nil)
	run, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		TemplateID:  "custom_flow",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun with custom template: %v", err)
	}
	if run.TemplateID != "custom_flow" || run.TemplateName != "Custom Flow" {
		t.Fatalf("expected custom template to be used, got id=%q name=%q", run.TemplateID, run.TemplateName)
	}
	if len(run.Phases) != 1 || len(run.Phases[0].Steps) != 1 || run.Phases[0].Steps[0].Prompt != "custom prompt" {
		t.Fatalf("expected custom prompt to be snapshotted, got %#v", run.Phases)
	}
}

func TestGuidedWorkflowPromptDispatcherUsesExplicitSession(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-1", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{
				SessionID:   "sess-1",
				WorkspaceID: "ws-1",
				WorktreeID:  "wt-1",
				RuntimeOptions: &types.SessionRuntimeOptions{
					Model: "gpt-5",
				},
			},
		},
		turnID: "turn-1",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-1" || result.TurnID != "turn-1" {
		t.Fatalf("unexpected dispatch result: %#v", result)
	}
	if result.Provider != "codex" || result.Model != "gpt-5" {
		t.Fatalf("expected provider/model in dispatch result, got %#v", result)
	}
	if len(gateway.sendCalls) != 1 || gateway.sendCalls[0].sessionID != "sess-1" {
		t.Fatalf("expected prompt to be sent to explicit session, got %#v", gateway.sendCalls)
	}
}

func TestGuidedWorkflowPromptDispatcherResolvesMostRecentContextSession(t *testing.T) {
	older := time.Now().UTC().Add(-2 * time.Hour)
	newer := time.Now().UTC()
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-old", Provider: "codex", Status: types.SessionStatusRunning},
			{ID: "sess-new", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{
				SessionID:      "sess-old",
				WorkspaceID:    "ws-1",
				WorktreeID:     "wt-1",
				LastActiveAt:   &older,
				RuntimeOptions: &types.SessionRuntimeOptions{Model: "gpt-4.1"},
			},
			{
				SessionID:      "sess-new",
				WorkspaceID:    "ws-1",
				WorktreeID:     "wt-1",
				LastActiveAt:   &newer,
				RuntimeOptions: &types.SessionRuntimeOptions{Model: "gpt-5"},
			},
		},
		turnID: "turn-2",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-new" {
		t.Fatalf("expected newest matching session, got %#v", result)
	}
	if result.Provider != "codex" || result.Model != "gpt-5" {
		t.Fatalf("expected newest model to be carried through, got %#v", result)
	}
	if len(gateway.sendCalls) != 1 || gateway.sendCalls[0].sessionID != "sess-new" {
		t.Fatalf("expected send to sess-new, got %#v", gateway.sendCalls)
	}
}

func TestGuidedWorkflowPromptDispatcherSkipsUnsupportedProvider(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-1", Provider: "claude", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-1", WorkspaceID: "ws-1"},
		},
		turnID: "turn-3",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		WorkspaceID: "ws-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected unsupported provider to skip dispatch, got %#v", result)
	}
	if len(gateway.sendCalls) != 0 {
		t.Fatalf("expected no send calls for unsupported provider, got %#v", gateway.sendCalls)
	}
}

func TestGuidedWorkflowNotificationPublisherForwardsAndObservesTurnCompleted(t *testing.T) {
	downstream := &recordNotificationPublisher{}
	orchestrator := &recordGuidedWorkflowOrchestrator{enabled: true}

	publisher := NewGuidedWorkflowNotificationPublisher(downstream, orchestrator)
	if publisher == nil {
		t.Fatalf("expected wrapped publisher")
	}

	publisher.Publish(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
	})
	publisher.Publish(types.NotificationEvent{
		Trigger:   types.NotificationTriggerSessionFailed,
		SessionID: "sess-1",
	})

	if len(downstream.events) != 2 {
		t.Fatalf("expected downstream notifications to be preserved, got %d", len(downstream.events))
	}
	if len(orchestrator.turnEvents) != 1 {
		t.Fatalf("expected one guided workflow turn event, got %d", len(orchestrator.turnEvents))
	}
	if orchestrator.turnEvents[0].Trigger != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected guided workflow event trigger: %q", orchestrator.turnEvents[0].Trigger)
	}
}

func TestGuidedWorkflowNotificationPublisherEmitsDecisionNeededPayload(t *testing.T) {
	downstream := &recordNotificationPublisher{}
	orchestrator := &recordGuidedWorkflowOrchestrator{enabled: true}
	runService := guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	preCommitHardGate := true
	run, err := runService.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
		PolicyOverrides: &guidedworkflows.CheckpointPolicyOverride{
			HardGates: &guidedworkflows.CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = runService.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running before turn progression, got %q", run.Status)
	}

	publisher := NewGuidedWorkflowNotificationPublisher(downstream, orchestrator, runService)
	decisionCount := 0
	lastTurnID := ""
	for i := 0; i < 20; i++ {
		lastTurnID = fmt.Sprintf("turn-%d", i+1)
		publisher.Publish(types.NotificationEvent{
			Trigger:     types.NotificationTriggerTurnCompleted,
			SessionID:   "sess-1",
			WorkspaceID: "ws-1",
			WorktreeID:  "wt-1",
			Provider:    "codex",
			TurnID:      lastTurnID,
		})
		decisionCount = countDecisionNeededEvents(downstream.events)
		if decisionCount > 0 {
			break
		}
	}
	if decisionCount != 1 {
		t.Fatalf("expected exactly one decision-needed event, got %d", decisionCount)
	}
	decisionEvent := lastDecisionNeededEvent(downstream.events)
	if decisionEvent == nil {
		t.Fatalf("expected decision-needed event")
	}
	if decisionEvent.Status != "decision_needed" {
		t.Fatalf("expected decision_needed status, got %q", decisionEvent.Status)
	}
	if reason := asString(decisionEvent.Payload["reason"]); reason == "" {
		t.Fatalf("expected reason in payload")
	}
	if riskSummary := asString(decisionEvent.Payload["risk_summary"]); riskSummary == "" {
		t.Fatalf("expected risk_summary in payload")
	}
	if recommended := asString(decisionEvent.Payload["recommended_action"]); recommended == "" {
		t.Fatalf("expected recommended_action in payload")
	}

	// Replaying the same turn event should not emit duplicate decision-needed notifications.
	publisher.Publish(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		TurnID:    lastTurnID,
	})
	if got := countDecisionNeededEvents(downstream.events); got != 1 {
		t.Fatalf("expected duplicate replay to be deduped, got %d decision-needed events", got)
	}
}

type recordNotificationPublisher struct {
	events []types.NotificationEvent
}

func (p *recordNotificationPublisher) Publish(event types.NotificationEvent) {
	p.events = append(p.events, event)
}

func countDecisionNeededEvents(events []types.NotificationEvent) int {
	count := 0
	for _, event := range events {
		if event.Status == "decision_needed" {
			count++
		}
	}
	return count
}

func lastDecisionNeededEvent(events []types.NotificationEvent) *types.NotificationEvent {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Status == "decision_needed" {
			event := events[i]
			return &event
		}
	}
	return nil
}

type recordGuidedWorkflowOrchestrator struct {
	enabled    bool
	turnEvents []types.NotificationEvent
}

func (o *recordGuidedWorkflowOrchestrator) Enabled() bool {
	return o.enabled
}

func (o *recordGuidedWorkflowOrchestrator) Config() guidedworkflows.Config {
	return guidedworkflows.Config{
		Enabled: o.enabled,
	}
}

func (o *recordGuidedWorkflowOrchestrator) StartRun(context.Context, guidedworkflows.StartRunRequest) (*guidedworkflows.Run, error) {
	return nil, nil
}

func (o *recordGuidedWorkflowOrchestrator) OnTurnEvent(_ context.Context, event types.NotificationEvent) {
	o.turnEvents = append(o.turnEvents, event)
}

func boolPtr(v bool) *bool {
	return &v
}
