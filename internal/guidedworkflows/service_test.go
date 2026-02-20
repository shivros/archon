package guidedworkflows

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"control/internal/types"
)

type stubRunMetricsStore struct {
	loadSnapshot RunMetricsSnapshot
	loadErr      error
	saved        []RunMetricsSnapshot
}

type stubTemplateProvider struct {
	templates         []WorkflowTemplate
	err               error
	explicitConfig    bool
	explicitConfigErr error
}

type stubStepPromptDispatcher struct {
	calls     []StepPromptDispatchRequest
	responses []StepPromptDispatchResult
	err       error
}

func (s *stubTemplateProvider) ListWorkflowTemplates(context.Context) ([]WorkflowTemplate, error) {
	if s == nil {
		return nil, nil
	}
	if s.err != nil {
		return nil, s.err
	}
	out := make([]WorkflowTemplate, len(s.templates))
	for i := range s.templates {
		out[i] = cloneTemplate(s.templates[i])
	}
	return out, nil
}

func (s *stubTemplateProvider) HasWorkflowTemplateConfig(context.Context) (bool, error) {
	if s == nil {
		return false, nil
	}
	if s.explicitConfigErr != nil {
		return false, s.explicitConfigErr
	}
	return s.explicitConfig, nil
}

func (s *stubStepPromptDispatcher) DispatchStepPrompt(_ context.Context, req StepPromptDispatchRequest) (StepPromptDispatchResult, error) {
	if s == nil {
		return StepPromptDispatchResult{}, nil
	}
	s.calls = append(s.calls, req)
	if s.err != nil {
		return StepPromptDispatchResult{}, s.err
	}
	if len(s.responses) == 0 {
		return StepPromptDispatchResult{Dispatched: true, SessionID: "sess-dispatch"}, nil
	}
	result := s.responses[0]
	if len(s.responses) == 1 {
		s.responses = s.responses[:0]
	} else {
		s.responses = s.responses[1:]
	}
	return result, nil
}

func (s *stubRunMetricsStore) LoadRunMetrics(context.Context) (RunMetricsSnapshot, error) {
	if s == nil {
		return RunMetricsSnapshot{}, nil
	}
	if s.loadErr != nil {
		return RunMetricsSnapshot{}, s.loadErr
	}
	return s.loadSnapshot, nil
}

func (s *stubRunMetricsStore) SaveRunMetrics(_ context.Context, snapshot RunMetricsSnapshot) error {
	if s == nil {
		return nil
	}
	s.saved = append(s.saved, snapshot)
	return nil
}

func TestRunLifecycleNoopEndToEnd(t *testing.T) {
	service := NewRunService(Config{
		Enabled:         true,
		CheckpointStyle: "confidence_weighted",
		Mode:            "guarded_autopilot",
	})

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.Status != WorkflowRunStatusCreated {
		t.Fatalf("expected created status, got %q", run.Status)
	}
	if len(run.Phases) == 0 || len(run.Phases[0].Steps) == 0 {
		t.Fatalf("expected default template phases/steps")
	}
	if run.Phases[0].Steps[0].Prompt == "" {
		t.Fatalf("expected first step prompt to be snapshotted on run creation")
	}

	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}

	// StartRun advances one no-op step. Continue advancing to completion.
	for i := 0; i < 16; i++ {
		if run.Status != WorkflowRunStatusRunning {
			break
		}
		run, err = service.AdvanceRun(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("AdvanceRun iteration %d: %v", i, err)
		}
	}
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed status, got %q", run.Status)
	}
	if run.CompletedAt == nil {
		t.Fatalf("expected completed timestamp")
	}

	timeline, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	if len(timeline) == 0 {
		t.Fatalf("expected non-empty timeline")
	}
	if timeline[0].Type != "run_created" {
		t.Fatalf("unexpected first event: %q", timeline[0].Type)
	}
	last := timeline[len(timeline)-1]
	if last.Type != "run_completed" {
		t.Fatalf("unexpected final event: %q", last.Type)
	}
}

func TestRunLifecycleTemplateProviderOverridesBuiltinTemplate(t *testing.T) {
	custom := WorkflowTemplate{
		ID:          TemplateIDSolidPhaseDelivery,
		Name:        "Custom SOLID",
		Description: "customized template",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_custom",
				Name: "Custom",
				Steps: []WorkflowTemplateStep{
					{ID: "phase_plan", Name: "phase plan", Prompt: "custom phase plan prompt"},
				},
			},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{templates: []WorkflowTemplate{custom}}),
	)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.TemplateName != "Custom SOLID" {
		t.Fatalf("expected custom template name, got %q", run.TemplateName)
	}
	if len(run.Phases) != 1 || len(run.Phases[0].Steps) != 1 {
		t.Fatalf("expected custom template steps, got %#v", run.Phases)
	}
	if run.Phases[0].Steps[0].Prompt != "custom phase plan prompt" {
		t.Fatalf("expected custom step prompt, got %q", run.Phases[0].Steps[0].Prompt)
	}
}

func TestRunLifecycleTemplateProviderReplacesDefaultTemplatesWhenConfigured(t *testing.T) {
	custom := WorkflowTemplate{
		ID:   "custom_only",
		Name: "Custom Only",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_custom",
				Name: "Custom",
				Steps: []WorkflowTemplateStep{
					{ID: "step_custom", Name: "step custom", Prompt: "custom prompt"},
				},
			},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			templates:      []WorkflowTemplate{custom},
			explicitConfig: true,
		}),
	)

	if _, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("expected built-in template to be unavailable when explicit config exists, got %v", err)
	}

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "custom_only",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun custom_only: %v", err)
	}
	if run.TemplateID != "custom_only" {
		t.Fatalf("expected custom template id, got %q", run.TemplateID)
	}
}

func TestRunLifecycleTemplateProviderFallsBackToDefaultsWhenNoExplicitConfig(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			templates:      []WorkflowTemplate{},
			explicitConfig: false,
		}),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun with default template fallback: %v", err)
	}
	if run.TemplateID != TemplateIDSolidPhaseDelivery {
		t.Fatalf("expected built-in template id, got %q", run.TemplateID)
	}
}

func TestRunLifecycleTemplateProviderAllowsExplicitEmptyConfig(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			templates:      []WorkflowTemplate{},
			explicitConfig: true,
		}),
	)
	if _, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("expected template not found for explicit empty template config, got %v", err)
	}
}

func TestRunLifecycleListTemplatesUsesResolvedCatalog(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			templates: []WorkflowTemplate{
				{
					ID:          "custom_release",
					Name:        "Release Delivery",
					Description: "release flow",
					Phases: []WorkflowTemplatePhase{
						{
							ID:   "phase_release",
							Name: "Release",
							Steps: []WorkflowTemplateStep{
								{ID: "step_release", Name: "release step", Prompt: "release prompt"},
							},
						},
					},
				},
				{
					ID:   "custom_bugfix",
					Name: "Bugfix Delivery",
					Phases: []WorkflowTemplatePhase{
						{
							ID:   "phase_bugfix",
							Name: "Bugfix",
							Steps: []WorkflowTemplateStep{
								{ID: "step_bugfix", Name: "bugfix step", Prompt: "bugfix prompt"},
							},
						},
					},
				},
			},
			explicitConfig: true,
		}),
	)

	templates, err := service.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(templates))
	}
	if templates[0].ID != "custom_bugfix" || templates[1].ID != "custom_release" {
		t.Fatalf("expected templates sorted by name, got %#v", templates)
	}
}

func TestRunLifecycleCreateRunWithoutTemplateUsesFirstResolvedTemplate(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			templates: []WorkflowTemplate{
				{
					ID:   "z_custom",
					Name: "Zulu",
					Phases: []WorkflowTemplatePhase{
						{
							ID:   "phase_zulu",
							Name: "Zulu",
							Steps: []WorkflowTemplateStep{
								{ID: "step_zulu", Name: "step zulu", Prompt: "zulu prompt"},
							},
						},
					},
				},
				{
					ID:   "a_custom",
					Name: "Alpha",
					Phases: []WorkflowTemplatePhase{
						{
							ID:   "phase_alpha",
							Name: "Alpha",
							Steps: []WorkflowTemplateStep{
								{ID: "step_alpha", Name: "step alpha", Prompt: "alpha prompt"},
							},
						},
					},
				},
			},
			explicitConfig: true,
		}),
	)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.TemplateID != "a_custom" {
		t.Fatalf("expected first lexicographic template id fallback, got %q", run.TemplateID)
	}
	if run.TemplateName != "Alpha" {
		t.Fatalf("expected fallback template name Alpha, got %q", run.TemplateName)
	}
}

func TestRunLifecycleListTemplatesFallsBackToDefaultsWhenProviderErrors(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{err: errors.New("template provider down")}),
	)

	templates, err := service.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) == 0 {
		t.Fatalf("expected built-in templates fallback when provider fails")
	}
	foundDefault := false
	for _, template := range templates {
		if template.ID == TemplateIDSolidPhaseDelivery {
			foundDefault = true
			break
		}
	}
	if !foundDefault {
		t.Fatalf("expected fallback list to include %q, got %#v", TemplateIDSolidPhaseDelivery, templates)
	}
}

func TestRunLifecycleTemplateProviderPresenceProbeErrorFallsBackToDefaults(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			templates: []WorkflowTemplate{
				{ID: "invalid_no_steps", Name: "Invalid"},
			},
			explicitConfigErr: errors.New("config probe failed"),
		}),
	)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun with fallback defaults: %v", err)
	}
	if run.TemplateID != TemplateIDSolidPhaseDelivery {
		t.Fatalf("expected fallback default template id %q, got %q", TemplateIDSolidPhaseDelivery, run.TemplateID)
	}
}

func TestRunLifecycleListTemplatesFiltersInvalidProviderTemplates(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			explicitConfig: true,
			templates: []WorkflowTemplate{
				{
					ID:   "",
					Name: "Missing ID",
					Phases: []WorkflowTemplatePhase{
						{
							ID:    "phase",
							Name:  "phase",
							Steps: []WorkflowTemplateStep{{ID: "step", Name: "step", Prompt: "prompt"}},
						},
					},
				},
				{
					ID:   "invalid_no_steps",
					Name: "No Steps",
					Phases: []WorkflowTemplatePhase{
						{ID: "phase", Name: "phase", Steps: nil},
					},
				},
				{
					ID:   "valid_template",
					Name: "Valid",
					Phases: []WorkflowTemplatePhase{
						{
							ID:    "phase",
							Name:  "phase",
							Steps: []WorkflowTemplateStep{{ID: "step", Name: "step", Prompt: "prompt"}},
						},
					},
				},
			},
		}),
	)

	templates, err := service.ListTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) != 1 || templates[0].ID != "valid_template" {
		t.Fatalf("expected only valid provider template to remain, got %#v", templates)
	}
}

func TestRunLifecycleListTemplatesNilService(t *testing.T) {
	var service *InMemoryRunService
	if _, err := service.ListTemplates(context.Background()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition for nil service, got %v", err)
	}
}

func TestDefaultTemplateIDSelectionRules(t *testing.T) {
	if got := defaultTemplateID(nil); got != "" {
		t.Fatalf("expected empty default template id for nil map, got %q", got)
	}
	if got := defaultTemplateID(map[string]WorkflowTemplate{
		"  ": {ID: "  ", Name: "blank"},
	}); got != "" {
		t.Fatalf("expected empty default template id when no non-blank ids exist, got %q", got)
	}
	if got := defaultTemplateID(map[string]WorkflowTemplate{
		"b_custom": {ID: "b_custom"},
		"a_custom": {ID: "a_custom"},
	}); got != "a_custom" {
		t.Fatalf("expected lexical fallback a_custom, got %q", got)
	}
	if got := defaultTemplateID(map[string]WorkflowTemplate{
		TemplateIDSolidPhaseDelivery: {ID: TemplateIDSolidPhaseDelivery},
		"a_custom":                   {ID: "a_custom"},
	}); got != TemplateIDSolidPhaseDelivery {
		t.Fatalf("expected built-in default %q, got %q", TemplateIDSolidPhaseDelivery, got)
	}
}

func TestRunLifecyclePauseResumeTransitions(t *testing.T) {
	service := NewRunService(Config{Enabled: true})

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
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running, got %q", run.Status)
	}

	run, err = service.PauseRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("PauseRun: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused, got %q", run.Status)
	}

	run, err = service.ResumeRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning && run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected running or completed after resume, got %q", run.Status)
	}
}

func TestRunLifecycleInvalidTransitions(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	if _, err := service.ResumeRun(context.Background(), run.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for resume before pause, got %v", err)
	}
	if _, err := service.PauseRun(context.Background(), run.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for pause before start, got %v", err)
	}

	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for duplicate start, got %v", err)
	}
}

func TestRunLifecycleCreateRunValidation(t *testing.T) {
	disabled := NewRunService(Config{})
	if _, err := disabled.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}

	enabled := NewRunService(Config{Enabled: true})
	if _, err := enabled.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  TemplateIDSolidPhaseDelivery,
		WorkspaceID: "",
		WorktreeID:  "",
	}); !errors.Is(err, ErrMissingContext) {
		t.Fatalf("expected ErrMissingContext, got %v", err)
	}
	if _, err := enabled.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "unknown",
		WorkspaceID: "ws-1",
	}); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("expected ErrTemplateNotFound, got %v", err)
	}
}

func TestRunLifecycleStepHandlerFailureTransitionsToFailed(t *testing.T) {
	engine := NewEngine(WithStepHandler("implementation", func(context.Context, *WorkflowRun, *PhaseRun, *StepRun) error {
		return errors.New("boom")
	}))
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
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}

	_, err = service.AdvanceRun(context.Background(), run.ID)
	if err == nil {
		t.Fatalf("expected advance to fail on implementation step")
	}
	run, err = service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed status, got %q", run.Status)
	}
	if run.LastError == "" {
		t.Fatalf("expected last error to be captured")
	}
}

func TestRunLifecyclePerRunPolicyOverridesApplied(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	confidenceThreshold := 0.82
	blastRadiusCount := 12
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &CheckpointPolicyOverride{
			ConfidenceThreshold:      &confidenceThreshold,
			HighBlastRadiusFileCount: &blastRadiusCount,
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.Policy.ConfidenceThreshold != 0.82 {
		t.Fatalf("unexpected per-run confidence threshold: %v", run.Policy.ConfidenceThreshold)
	}
	if run.Policy.HighBlastRadiusFileCount != 12 {
		t.Fatalf("unexpected per-run blast radius threshold: %d", run.Policy.HighBlastRadiusFileCount)
	}
}

func TestRunLifecyclePolicyPauseDecisionMetadata(t *testing.T) {
	service := NewRunService(Config{Enabled: true}, WithTemplate(WorkflowTemplate{
		ID:   "single_commit",
		Name: "Single Commit",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "one",
				Name: "one",
				Steps: []WorkflowTemplateStep{
					{ID: "commit", Name: "commit"},
				},
			},
		},
	}))
	preCommitHardGate := true
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "single_commit",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &CheckpointPolicyOverride{
			HardGates: &CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected run paused by pre-commit hard gate, got %q", run.Status)
	}
	if run.LatestDecision == nil {
		t.Fatalf("expected latest decision metadata")
	}
	if run.LatestDecision.Metadata.Action != CheckpointActionPause {
		t.Fatalf("expected pause decision action, got %q", run.LatestDecision.Metadata.Action)
	}
	if run.LatestDecision.Metadata.Severity != DecisionSeverityCritical {
		t.Fatalf("expected critical severity, got %q", run.LatestDecision.Metadata.Severity)
	}
	if len(run.LatestDecision.Metadata.Reasons) == 0 {
		t.Fatalf("expected decision reasons")
	}
}

func TestRunLifecycleHandleDecisionActions(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}

	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionActionPauseRun,
		Note:   "pause for review",
	})
	if err != nil {
		t.Fatalf("HandleDecision pause_run: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused after pause_run decision, got %q", run.Status)
	}

	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionActionRequestRevision,
		Note:   "needs revision",
	})
	if err != nil {
		t.Fatalf("HandleDecision request_revision: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused after request_revision decision, got %q", run.Status)
	}

	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionActionApproveContinue,
		Note:   "continue anyway",
	})
	if err != nil {
		t.Fatalf("HandleDecision approve_continue: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning && run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected running/completed after approve_continue, got %q", run.Status)
	}
}

func TestRunLifecycleHandleDecisionIdempotent(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
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
	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionActionPauseRun,
	})
	if err != nil {
		t.Fatalf("HandleDecision pause_run: %v", err)
	}
	timelineBefore, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action:     DecisionActionRequestRevision,
		DecisionID: run.LatestDecision.ID,
	})
	if err != nil {
		t.Fatalf("HandleDecision request_revision first: %v", err)
	}
	timelineAfterFirst, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline first: %v", err)
	}
	if len(timelineAfterFirst) <= len(timelineBefore) {
		t.Fatalf("expected timeline growth after first request_revision")
	}
	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action:     DecisionActionRequestRevision,
		DecisionID: run.LatestDecision.ID,
	})
	if err != nil {
		t.Fatalf("HandleDecision request_revision second: %v", err)
	}
	timelineAfterSecond, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline second: %v", err)
	}
	if len(timelineAfterSecond) != len(timelineAfterFirst) {
		t.Fatalf("expected idempotent repeated decision action; first=%d second=%d", len(timelineAfterFirst), len(timelineAfterSecond))
	}
}

func TestRunLifecycleOnTurnCompletedIdempotent(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-turn",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}
	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-turn",
		TurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted first: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one run update on first turn, got %d", len(updated))
	}
	updated, err = service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-turn",
		TurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted duplicate: %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("expected duplicate turn event to be deduped, got %d updates", len(updated))
	}
}

func TestRunLifecyclePromptDispatchWaitsForTurnThenAdvances(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted",
		Name: "Prompted",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
					{ID: "step_2", Name: "step 2", Prompt: "prompt 2"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-a"},
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-b"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
	)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected first prompt dispatch on start, got %d calls", len(dispatcher.calls))
	}
	first := run.Phases[0].Steps[0]
	if !first.AwaitingTurn || first.Status != StepRunStatusRunning {
		t.Fatalf("expected first step awaiting turn after dispatch, got %#v", first)
	}
	if first.Execution == nil {
		t.Fatalf("expected execution reference for dispatched step")
	}
	if first.Execution.SessionID != "sess-1" || first.Execution.TurnID != "turn-a" {
		t.Fatalf("unexpected first step execution ref: %#v", first.Execution)
	}
	if len(first.ExecutionAttempts) != 1 {
		t.Fatalf("expected single execution attempt, got %d", len(first.ExecutionAttempts))
	}

	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted turn-a: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after turn-a, got %d", len(updated))
	}
	run = updated[0]
	if run.Phases[0].Steps[0].Status != StepRunStatusCompleted {
		t.Fatalf("expected first step completed after turn-a, got %q", run.Phases[0].Steps[0].Status)
	}
	if run.Phases[0].Steps[0].Execution == nil || run.Phases[0].Steps[0].Execution.CompletedAt == nil {
		t.Fatalf("expected first step execution completion metadata after turn-a")
	}
	if run.Phases[0].Steps[1].Status != StepRunStatusRunning || !run.Phases[0].Steps[1].AwaitingTurn {
		t.Fatalf("expected second step awaiting turn after turn-a, got %#v", run.Phases[0].Steps[1])
	}
	if len(dispatcher.calls) != 2 {
		t.Fatalf("expected second prompt dispatch after turn-a, got %d calls", len(dispatcher.calls))
	}

	updated, err = service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-b",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted turn-b: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after turn-b, got %d", len(updated))
	}
	run = updated[0]
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed status after final turn, got %q", run.Status)
	}
	last := run.Phases[0].Steps[1]
	if last.Execution == nil || last.Execution.CompletedAt == nil {
		t.Fatalf("expected final step execution completion metadata")
	}
}

func TestRunLifecyclePromptDispatchCapturesProviderAndModel(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_with_model",
		Name: "Prompted with model",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-a", Provider: "codex", Model: "gpt-5"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_with_model",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	step := run.Phases[0].Steps[0]
	if step.Execution == nil {
		t.Fatalf("expected execution metadata")
	}
	if step.Execution.Provider != "codex" || step.Execution.Model != "gpt-5" {
		t.Fatalf("expected provider/model from dispatcher, got %#v", step.Execution)
	}
	if step.Execution.PromptSnapshot != "prompt 1" {
		t.Fatalf("expected prompt snapshot, got %q", step.Execution.PromptSnapshot)
	}
	if strings.TrimSpace(step.Execution.TraceID) == "" {
		t.Fatalf("expected trace id to be set")
	}
}

func TestRunLifecycleFirstDispatchPrependsUserPromptOnlyOnce(t *testing.T) {
	template := WorkflowTemplate{
		ID:                 "prompted_with_brief",
		Name:               "Prompted with brief",
		DefaultAccessLevel: types.AccessReadOnly,
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "overall plan prompt"},
					{ID: "step_2", Name: "step 2", Prompt: "phase plan prompt"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-a"},
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-b"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_with_brief",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
		UserPrompt:  "Fix bug in workflow setup",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.UserPrompt != "Fix bug in workflow setup" {
		t.Fatalf("expected run user prompt to persist, got %q", run.UserPrompt)
	}

	_, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected first dispatch call, got %d", len(dispatcher.calls))
	}
	if got := dispatcher.calls[0].Prompt; got != "Fix bug in workflow setup\n\noverall plan prompt" {
		t.Fatalf("expected first dispatch to prepend user prompt, got %q", got)
	}
	if got := dispatcher.calls[0].DefaultAccessLevel; got != types.AccessReadOnly {
		t.Fatalf("expected first dispatch default access %q, got %q", types.AccessReadOnly, got)
	}

	_, err = service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted turn-a: %v", err)
	}
	if len(dispatcher.calls) != 2 {
		t.Fatalf("expected second dispatch call, got %d", len(dispatcher.calls))
	}
	if got := dispatcher.calls[1].Prompt; got != "phase plan prompt" {
		t.Fatalf("expected only template prompt after first dispatch, got %q", got)
	}
	if got := dispatcher.calls[1].DefaultAccessLevel; got != types.AccessReadOnly {
		t.Fatalf("expected second dispatch default access %q, got %q", types.AccessReadOnly, got)
	}
}

func TestNormalizeTemplateAccessLevel(t *testing.T) {
	tests := []struct {
		name string
		in   types.AccessLevel
		want types.AccessLevel
		ok   bool
	}{
		{name: "empty", in: "", want: "", ok: true},
		{name: "read_only", in: types.AccessReadOnly, want: types.AccessReadOnly, ok: true},
		{name: "on-request", in: "on-request", want: types.AccessOnRequest, ok: true},
		{name: "full-access", in: "full-access", want: types.AccessFull, ok: true},
		{name: "invalid", in: "invalid", want: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeTemplateAccessLevel(tt.in)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("NormalizeTemplateAccessLevel(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestRunLifecycleAdvanceRunDoesNotBypassAwaitingTurn(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted",
		Name: "Prompted",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-a"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected initial dispatch count=1, got %d", len(dispatcher.calls))
	}
	run, err = service.AdvanceRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("AdvanceRun: %v", err)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected no additional dispatch while awaiting turn, got %d", len(dispatcher.calls))
	}
	if run.Phases[0].Steps[0].Status != StepRunStatusRunning || !run.Phases[0].Steps[0].AwaitingTurn {
		t.Fatalf("expected step to remain awaiting turn after manual advance, got %#v", run.Phases[0].Steps[0])
	}
}

func TestRunLifecyclePromptDispatchFailureFailsRun(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted",
		Name: "Prompted",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
			},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(&stubStepPromptDispatcher{err: errors.New("dispatch failed")}),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err == nil {
		t.Fatalf("expected start to fail when prompt dispatch fails")
	}
	run, err = service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run after dispatch error, got %q", run.Status)
	}
	if !strings.Contains(strings.ToLower(run.LastError), "dispatch failed") {
		t.Fatalf("expected dispatch failure in LastError, got %q", run.LastError)
	}
}

func TestRunLifecyclePromptDispatchUnavailableFailsRun(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted",
		Name: "Prompted",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: false},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err == nil {
		t.Fatalf("expected start to fail when prompt dispatch is unavailable")
	} else if !errors.Is(err, ErrStepDispatch) {
		t.Fatalf("expected ErrStepDispatch, got %v", err)
	}
	run, err = service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run after unavailable dispatch, got %q", run.Status)
	}
	if !strings.Contains(strings.ToLower(run.LastError), "dispatcher did not dispatch") {
		t.Fatalf("expected unavailable dispatch detail in LastError, got %q", run.LastError)
	}
}

func TestRunLifecycleOnTurnCompletedCanReachPolicyPause(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	preCommitHardGate := true
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-policy",
		PolicyOverrides: &CheckpointPolicyOverride{
			HardGates: &CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}
	for i := 0; i < 20 && run.Status == WorkflowRunStatusRunning; i++ {
		updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
			SessionID: "sess-policy",
			TurnID:    fmt.Sprintf("turn-%d", i+1),
		})
		if err != nil {
			t.Fatalf("OnTurnCompleted iteration %d: %v", i, err)
		}
		if len(updated) > 0 {
			run = updated[len(updated)-1]
		}
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected run paused by policy on turn progression, got %q", run.Status)
	}
	if run.LatestDecision == nil || run.LatestDecision.Metadata.Action != CheckpointActionPause {
		t.Fatalf("expected latest pause decision metadata after turn progression")
	}
}

func TestRunLifecycleHandleDecisionRejectsUnknownAction(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionAction("unknown"),
	}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for unknown decision action, got %v", err)
	}
}

func TestRunLifecycleMetricsSnapshot(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
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
			t.Fatalf("AdvanceRun %d: %v", i, err)
		}
	}
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed run status, got %q", run.Status)
	}
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if !metrics.Enabled {
		t.Fatalf("expected metrics enabled")
	}
	if metrics.RunsStarted != 1 {
		t.Fatalf("expected runs_started=1, got %d", metrics.RunsStarted)
	}
	if metrics.RunsCompleted != 1 {
		t.Fatalf("expected runs_completed=1, got %d", metrics.RunsCompleted)
	}
	if metrics.PauseRate != 0 {
		t.Fatalf("expected pause_rate=0 for uninterrupted run, got %f", metrics.PauseRate)
	}
}

func TestRunLifecycleMetricsApprovalLatencyAndInterventionCause(t *testing.T) {
	service := NewRunService(Config{Enabled: true}, WithTemplate(WorkflowTemplate{
		ID:   "single_commit",
		Name: "Single Commit",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []WorkflowTemplateStep{
					{ID: "commit", Name: "commit"},
				},
			},
		},
	}))
	preCommitHardGate := true
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "single_commit",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &CheckpointPolicyOverride{
			HardGates: &CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused status from policy checkpoint, got %q", run.Status)
	}
	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action:     DecisionActionApproveContinue,
		DecisionID: run.LatestDecision.ID,
	})
	if err != nil {
		t.Fatalf("HandleDecision approve_continue: %v", err)
	}
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed after approval, got %q", run.Status)
	}
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.PauseCount != 1 {
		t.Fatalf("expected pause_count=1, got %d", metrics.PauseCount)
	}
	if metrics.PauseRate != 1 {
		t.Fatalf("expected pause_rate=1, got %f", metrics.PauseRate)
	}
	if metrics.ApprovalCount != 1 {
		t.Fatalf("expected approval_count=1, got %d", metrics.ApprovalCount)
	}
	if metrics.ApprovalLatencyAvgMS < 0 || metrics.ApprovalLatencyMaxMS < metrics.ApprovalLatencyAvgMS {
		t.Fatalf("unexpected approval latency metrics: avg=%d max=%d", metrics.ApprovalLatencyAvgMS, metrics.ApprovalLatencyMaxMS)
	}
	if metrics.InterventionCauses["pre_commit_approval_required"] != 1 {
		t.Fatalf("expected pre_commit_approval intervention cause, got %#v", metrics.InterventionCauses)
	}
}

func TestRunLifecycleTelemetryCanBeDisabled(t *testing.T) {
	service := NewRunService(Config{Enabled: true}, WithTelemetryEnabled(false))
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.Enabled {
		t.Fatalf("expected telemetry disabled")
	}
	if metrics.RunsStarted != 0 || metrics.PauseCount != 0 || metrics.ApprovalCount != 0 {
		t.Fatalf("expected zeroed metrics when telemetry disabled: %#v", metrics)
	}
}

func TestRunLifecycleMaxActiveRunsGuardrail(t *testing.T) {
	service := NewRunService(Config{Enabled: true}, WithMaxActiveRuns(1))
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun first: %v", err)
	}
	if _, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	}); !errors.Is(err, ErrRunLimitExceeded) {
		t.Fatalf("expected ErrRunLimitExceeded, got %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	for i := 0; i < 32 && run.Status == WorkflowRunStatusRunning; i++ {
		run, err = service.AdvanceRun(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("AdvanceRun %d: %v", i, err)
		}
	}
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed run, got %q", run.Status)
	}
	if _, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	}); err != nil {
		t.Fatalf("expected create to succeed after active run completed, got %v", err)
	}
}

func TestRunLifecycleTelemetryPersistenceRoundTrip(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:       true,
			RunsStarted:   4,
			RunsCompleted: 3,
			PauseCount:    2,
			InterventionCauses: map[string]int{
				"pre_commit_approval_required": 2,
			},
		},
	}
	service := NewRunService(Config{Enabled: true}, WithRunMetricsStore(store))
	initial, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics initial: %v", err)
	}
	if initial.RunsStarted != 4 || initial.RunsCompleted != 3 || initial.PauseCount != 2 {
		t.Fatalf("expected persisted metrics to be restored, got %#v", initial)
	}
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if len(store.saved) == 0 {
		t.Fatalf("expected metrics persistence on run start")
	}
	last := store.saved[len(store.saved)-1]
	if last.RunsStarted != 5 {
		t.Fatalf("expected runs_started to persist as 5, got %d", last.RunsStarted)
	}
	if last.InterventionCauses["pre_commit_approval_required"] != 2 {
		t.Fatalf("expected persisted intervention causes to be preserved, got %#v", last.InterventionCauses)
	}
}

func TestRunLifecycleTelemetryPersistenceSkippedWhenDisabled(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:     true,
			RunsStarted: 7,
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTelemetryEnabled(false),
		WithRunMetricsStore(store),
	)
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.RunsStarted != 0 || metrics.Enabled {
		t.Fatalf("expected telemetry-disabled service to ignore persisted counters, got %#v", metrics)
	}
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("expected no persistence writes when telemetry disabled, got %d", len(store.saved))
	}
}

func TestRunLifecycleResetMetrics(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:       true,
			RunsStarted:   4,
			RunsCompleted: 3,
			PauseCount:    1,
		},
	}
	service := NewRunService(Config{Enabled: true}, WithRunMetricsStore(store))
	snapshot, err := service.ResetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("ResetRunMetrics: %v", err)
	}
	if !snapshot.Enabled {
		t.Fatalf("expected reset snapshot enabled=true")
	}
	if snapshot.RunsStarted != 0 || snapshot.RunsCompleted != 0 || snapshot.PauseCount != 0 || snapshot.ApprovalCount != 0 {
		t.Fatalf("expected zeroed reset snapshot, got %#v", snapshot)
	}
	if len(snapshot.InterventionCauses) != 0 {
		t.Fatalf("expected no intervention causes after reset, got %#v", snapshot.InterventionCauses)
	}
	if len(store.saved) == 0 {
		t.Fatalf("expected reset to persist metrics snapshot")
	}
	last := store.saved[len(store.saved)-1]
	if last.RunsStarted != 0 || last.PauseCount != 0 {
		t.Fatalf("expected persisted reset snapshot to be zeroed, got %#v", last)
	}
}

func TestRunLifecycleResetMetricsPersistsWhenTelemetryDisabled(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:       true,
			RunsStarted:   8,
			RunsCompleted: 7,
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTelemetryEnabled(false),
		WithRunMetricsStore(store),
	)
	snapshot, err := service.ResetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("ResetRunMetrics: %v", err)
	}
	if snapshot.Enabled {
		t.Fatalf("expected reset snapshot enabled=false when telemetry is disabled")
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected reset to force one persistence write, got %d", len(store.saved))
	}
	if store.saved[0].RunsStarted != 0 || store.saved[0].RunsCompleted != 0 {
		t.Fatalf("expected forced reset persistence to zero counters, got %#v", store.saved[0])
	}
}

func TestRunLifecycleListRunsSortedByRecentActivity(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	first, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun first: %v", err)
	}
	second, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	})
	if err != nil {
		t.Fatalf("CreateRun second: %v", err)
	}
	if _, err := service.StartRun(context.Background(), first.ID); err != nil {
		t.Fatalf("StartRun first: %v", err)
	}
	runs, err := service.ListRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected two runs, got %d", len(runs))
	}
	if runs[0].ID != first.ID {
		t.Fatalf("expected first run to sort first after recent activity, got %q", runs[0].ID)
	}
	if runs[1].ID != second.ID {
		t.Fatalf("expected second run to sort second, got %q", runs[1].ID)
	}
}

func TestRunLifecycleDismissAndUndismissRun(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	dismissed, err := service.DismissRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("DismissRun: %v", err)
	}
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set")
	}

	defaultRuns, err := service.ListRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(defaultRuns) != 0 {
		t.Fatalf("expected dismissed run to be hidden from default list, got %d entries", len(defaultRuns))
	}

	allRuns, err := service.ListRunsIncludingDismissed(context.Background())
	if err != nil {
		t.Fatalf("ListRunsIncludingDismissed: %v", err)
	}
	if len(allRuns) != 1 || allRuns[0].ID != run.ID {
		t.Fatalf("expected dismissed run in include_dismissed list, got %#v", allRuns)
	}

	undismissed, err := service.UndismissRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("UndismissRun: %v", err)
	}
	if undismissed.DismissedAt != nil {
		t.Fatalf("expected dismissed_at to clear after undismiss")
	}
}
