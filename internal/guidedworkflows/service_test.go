package guidedworkflows

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"control/internal/types"
)

type stubRunMetricsStore struct {
	loadSnapshot RunMetricsSnapshot
	loadErr      error
	saved        []RunMetricsSnapshot
}

type stubRunSnapshotStore struct {
	loadSnapshots []RunStatusSnapshot
	loadErr       error
	savedByRunID  map[string]RunStatusSnapshot
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
	errs      []error
}

type stubMissingRunContextResolver struct {
	contextByRunID map[string]MissingRunDismissalContext
	err            error
}

type missingRunTombstoneFactoryCall struct {
	runID           string
	contextResolved bool
	context         MissingRunDismissalContext
}

type stubMissingRunTombstoneFactory struct {
	runByID map[string]*WorkflowRun
	calls   []missingRunTombstoneFactoryCall
}

type stubDispatchErrorClassifier struct {
	disposition DispatchErrorDisposition
}

type stubDispatchRetryScheduler struct {
	enqueued   []string
	closeCalls int
}

type countingDispatchRetryPolicy struct {
	delay time.Duration
	calls atomic.Int32
}

type stubDispatchProviderPolicy struct {
	normalized  string
	validateErr error
}

type stubStepOutcomeEvaluator struct {
	evaluation StepOutcomeEvaluation
	inputs     []StepOutcomeEvaluationInput
}

func (s stubDispatchErrorClassifier) Classify(error) DispatchErrorDisposition {
	return s.disposition
}

func (s *stubStepOutcomeEvaluator) EvaluateStepOutcome(_ context.Context, input StepOutcomeEvaluationInput) StepOutcomeEvaluation {
	if s != nil {
		s.inputs = append(s.inputs, input)
	}
	if s == nil || strings.TrimSpace(string(s.evaluation.Decision)) == "" {
		return StepOutcomeEvaluation{Decision: StepOutcomeDecisionSuccess}
	}
	return s.evaluation
}

func (s *stubDispatchRetryScheduler) Enqueue(runID string) {
	if s == nil {
		return
	}
	s.enqueued = append(s.enqueued, strings.TrimSpace(runID))
}

func (s *stubDispatchRetryScheduler) Close() {
	if s == nil {
		return
	}
	s.closeCalls++
}

func (p *countingDispatchRetryPolicy) NextDelay(_ int) (time.Duration, bool) {
	if p == nil {
		return 0, false
	}
	p.calls.Add(1)
	delay := p.delay
	if delay <= 0 {
		delay = 1 * time.Millisecond
	}
	return delay, true
}

func (s stubDispatchProviderPolicy) Normalize(provider string) string {
	if strings.TrimSpace(s.normalized) != "" {
		return strings.TrimSpace(s.normalized)
	}
	return strings.ToLower(strings.TrimSpace(provider))
}

func (s stubDispatchProviderPolicy) SupportsDispatch(provider string) bool {
	return strings.TrimSpace(provider) != ""
}

func (s stubDispatchProviderPolicy) Validate(string) error {
	return s.validateErr
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
	if len(s.errs) > 0 {
		err := s.errs[0]
		if len(s.errs) == 1 {
			s.errs = s.errs[:0]
		} else {
			s.errs = s.errs[1:]
		}
		if err != nil {
			return StepPromptDispatchResult{}, err
		}
	}
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

func (s *stubMissingRunContextResolver) ResolveMissingRunContext(_ context.Context, runID string) (MissingRunDismissalContext, bool, error) {
	if s == nil {
		return MissingRunDismissalContext{}, false, nil
	}
	if s.err != nil {
		return MissingRunDismissalContext{}, false, s.err
	}
	ctx, ok := s.contextByRunID[strings.TrimSpace(runID)]
	if !ok {
		return MissingRunDismissalContext{}, false, nil
	}
	return ctx, true, nil
}

func (s *stubMissingRunTombstoneFactory) BuildMissingRunTombstone(
	runID string,
	dismissalContext MissingRunDismissalContext,
	contextResolved bool,
	now time.Time,
) *WorkflowRun {
	if s == nil {
		return nil
	}
	normalizedRunID := strings.TrimSpace(runID)
	s.calls = append(s.calls, missingRunTombstoneFactoryCall{
		runID:           normalizedRunID,
		contextResolved: contextResolved,
		context:         normalizeMissingRunDismissalContext(dismissalContext),
	})
	if s.runByID == nil {
		return nil
	}
	run, ok := s.runByID[normalizedRunID]
	if !ok {
		return nil
	}
	out := cloneWorkflowRun(run)
	if out != nil && out.CreatedAt.IsZero() {
		out.CreatedAt = now
	}
	return out
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

func (s *stubRunSnapshotStore) ListWorkflowRuns(context.Context) ([]RunStatusSnapshot, error) {
	if s == nil {
		return nil, nil
	}
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if len(s.savedByRunID) == 0 {
		out := make([]RunStatusSnapshot, len(s.loadSnapshots))
		for i := range s.loadSnapshots {
			out[i] = cloneRunSnapshotForTest(s.loadSnapshots[i])
		}
		return out, nil
	}
	out := make([]RunStatusSnapshot, 0, len(s.savedByRunID))
	for _, snapshot := range s.savedByRunID {
		out = append(out, cloneRunSnapshotForTest(snapshot))
	}
	return out, nil
}

func (s *stubRunSnapshotStore) UpsertWorkflowRun(_ context.Context, snapshot RunStatusSnapshot) error {
	if s == nil || snapshot.Run == nil || strings.TrimSpace(snapshot.Run.ID) == "" {
		return nil
	}
	if s.savedByRunID == nil {
		s.savedByRunID = map[string]RunStatusSnapshot{}
	}
	runID := strings.TrimSpace(snapshot.Run.ID)
	snapshot.Run.ID = runID
	s.savedByRunID[runID] = cloneRunSnapshotForTest(snapshot)
	return nil
}

func cloneRunSnapshotForTest(in RunStatusSnapshot) RunStatusSnapshot {
	return RunStatusSnapshot{
		Run:      cloneWorkflowRun(in.Run),
		Timeline: append([]RunTimelineEvent(nil), in.Timeline...),
	}
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

func TestRunLifecycleResumeFailedRunReusesStepSessionAndCustomMessage(t *testing.T) {
	now := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)
	startedAt := now.Add(-2 * time.Minute)
	completedAt := now.Add(-time.Minute)
	runID := "gwf-resume-failed"
	snapshotStore := &stubRunSnapshotStore{
		loadSnapshots: []RunStatusSnapshot{
			{
				Run: &WorkflowRun{
					ID:                runID,
					TemplateID:        "resume_template",
					TemplateName:      "Resume Template",
					WorkspaceID:       "ws-1",
					WorktreeID:        "wt-1",
					Status:            WorkflowRunStatusFailed,
					CreatedAt:         now.Add(-5 * time.Minute),
					StartedAt:         &startedAt,
					CompletedAt:       &completedAt,
					CurrentPhaseIndex: 0,
					CurrentStepIndex:  0,
					LastError:         "workflow run interrupted by daemon restart",
					Phases: []PhaseRun{
						{
							ID:          "phase-1",
							Name:        "Phase 1",
							Status:      PhaseRunStatusFailed,
							StartedAt:   &startedAt,
							CompletedAt: &completedAt,
							Steps: []StepRun{
								{
									ID:           "step-1",
									Name:         "Step 1",
									Prompt:       "original workflow step prompt",
									Status:       StepRunStatusRunning,
									AwaitingTurn: true,
									TurnID:       "turn-prev",
									StartedAt:    &startedAt,
									Attempts:     1,
									Outcome:      "awaiting_turn",
									Output:       "turn-prev",
									Execution: &StepExecutionRef{
										SessionID: "sess-prev",
										TurnID:    "turn-prev",
										StartedAt: &startedAt,
									},
									ExecutionAttempts: []StepExecutionRef{
										{
											SessionID: "sess-prev",
											TurnID:    "turn-prev",
											StartedAt: &startedAt,
										},
									},
									ExecutionState: StepExecutionStateLinked,
								},
							},
						},
					},
				},
				Timeline: []RunTimelineEvent{
					{
						At:      completedAt,
						Type:    "run_interrupted",
						RunID:   runID,
						Message: "workflow run interrupted by daemon restart",
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{
				Dispatched: true,
				SessionID:  "sess-prev",
				TurnID:     "turn-new",
			},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithRunSnapshotStore(snapshotStore),
		WithStepPromptDispatcher(dispatcher),
		WithEngine(&Engine{
			now:      func() time.Time { return now },
			handlers: map[string]StepHandler{},
			controls: NormalizeExecutionControls(ExecutionControls{}),
			runner:   noopExecutionRunner{},
		}),
	)

	run, err := service.ResumeFailedRun(context.Background(), runID, ResumeFailedRunRequest{
		Message: "resume from outage and continue",
	})
	if err != nil {
		t.Fatalf("ResumeFailedRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running status after failed resume, got %q", run.Status)
	}
	if run.LastError != "" {
		t.Fatalf("expected last error to clear on resume, got %q", run.LastError)
	}
	if run.CompletedAt != nil {
		t.Fatalf("expected completed_at to clear on resume")
	}
	if run.SessionID != "sess-prev" {
		t.Fatalf("expected run session to reuse previous step session, got %q", run.SessionID)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected one resume dispatch call, got %d", len(dispatcher.calls))
	}
	if dispatcher.calls[0].SessionID != "sess-prev" {
		t.Fatalf("expected resume dispatch to reuse session sess-prev, got %q", dispatcher.calls[0].SessionID)
	}
	if dispatcher.calls[0].Prompt != "resume from outage and continue" {
		t.Fatalf("expected resume dispatch prompt override, got %q", dispatcher.calls[0].Prompt)
	}
	step := run.Phases[0].Steps[0]
	if step.Status != StepRunStatusRunning || !step.AwaitingTurn {
		t.Fatalf("expected resumed step awaiting turn, got %#v", step)
	}
	if step.Prompt != "original workflow step prompt" {
		t.Fatalf("expected step prompt template to remain unchanged, got %q", step.Prompt)
	}
	if step.Execution == nil {
		t.Fatalf("expected execution metadata after resume dispatch")
	}
	if step.Execution.SessionID != "sess-prev" || step.Execution.TurnID != "turn-new" {
		t.Fatalf("unexpected resumed execution metadata: %#v", step.Execution)
	}
	if step.Attempts != 2 {
		t.Fatalf("expected resumed step attempts to increment, got %d", step.Attempts)
	}
	if len(step.ExecutionAttempts) != 2 {
		t.Fatalf("expected execution attempts history to retain prior attempt, got %d", len(step.ExecutionAttempts))
	}
	timeline, err := service.GetRunTimeline(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	foundResumeEvent := false
	for _, event := range timeline {
		if event.Type == "run_resumed_after_failure" && event.Message == "resume from outage and continue" {
			foundResumeEvent = true
			break
		}
	}
	if !foundResumeEvent {
		t.Fatalf("expected timeline to capture run_resumed_after_failure event with message")
	}
}

func TestRunLifecycleResumeFailedRunDefaultsResumeMessage(t *testing.T) {
	now := time.Date(2026, 2, 22, 11, 0, 0, 0, time.UTC)
	runID := "gwf-resume-default-msg"
	snapshotStore := &stubRunSnapshotStore{
		loadSnapshots: []RunStatusSnapshot{
			{
				Run: &WorkflowRun{
					ID:                runID,
					TemplateID:        "resume_template",
					TemplateName:      "Resume Template",
					WorkspaceID:       "ws-1",
					WorktreeID:        "wt-1",
					Status:            WorkflowRunStatusFailed,
					CreatedAt:         now.Add(-5 * time.Minute),
					CurrentPhaseIndex: 0,
					CurrentStepIndex:  0,
					LastError:         "workflow run interrupted by daemon restart",
					Phases: []PhaseRun{
						{
							ID:     "phase-1",
							Name:   "Phase 1",
							Status: PhaseRunStatusFailed,
							Steps: []StepRun{
								{
									ID:     "step-1",
									Name:   "Step 1",
									Prompt: "original workflow step prompt",
									Status: StepRunStatusFailed,
								},
							},
						},
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{
				Dispatched: true,
				SessionID:  "sess-default",
				TurnID:     "turn-default",
			},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithRunSnapshotStore(snapshotStore),
		WithStepPromptDispatcher(dispatcher),
	)

	_, err := service.ResumeFailedRun(context.Background(), runID, ResumeFailedRunRequest{})
	if err != nil {
		t.Fatalf("ResumeFailedRun: %v", err)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected one resume dispatch call, got %d", len(dispatcher.calls))
	}
	if dispatcher.calls[0].Prompt != DefaultResumeFailedRunMessage {
		t.Fatalf("expected default resume message prompt, got %q", dispatcher.calls[0].Prompt)
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
	if _, err := service.ResumeFailedRun(context.Background(), run.ID, ResumeFailedRunRequest{}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for failed resume before run failure, got %v", err)
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

func TestRunLifecycleStopRunFromCreatedAndIdempotent(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	stopped, err := service.StopRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StopRun: %v", err)
	}
	if stopped.Status != WorkflowRunStatusStopped {
		t.Fatalf("expected stopped status, got %q", stopped.Status)
	}
	if stopped.CompletedAt == nil {
		t.Fatalf("expected completed timestamp when stopped")
	}
	if strings.TrimSpace(stopped.LastError) != "workflow run stopped by user" {
		t.Fatalf("unexpected stop detail: %q", stopped.LastError)
	}

	stoppedAgain, err := service.StopRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StopRun idempotent call: %v", err)
	}
	if stoppedAgain.Status != WorkflowRunStatusStopped {
		t.Fatalf("expected stopped status on idempotent call, got %q", stoppedAgain.Status)
	}
	timeline, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	runStoppedEvents := 0
	for _, event := range timeline {
		if event.Type == "run_stopped" {
			runStoppedEvents++
		}
	}
	if runStoppedEvents != 1 {
		t.Fatalf("expected exactly one run_stopped timeline event, got %d (%#v)", runStoppedEvents, timeline)
	}
}

func TestRunLifecycleStopRunFromRunningStopsActivePhaseAndStep(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "stop_running_template",
		Name: "Stop Running Template",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase-1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step-1", Name: "Step 1", Prompt: "dispatch this step"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{
				Dispatched: true,
				SessionID:  "sess-stop",
				TurnID:     "turn-stop",
				Provider:   "codex",
			},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
	)

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "stop_running_template",
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
		t.Fatalf("expected running status before stop, got %q", run.Status)
	}
	if len(run.Phases) != 1 || len(run.Phases[0].Steps) != 1 {
		t.Fatalf("unexpected run structure: %#v", run.Phases)
	}
	if run.Phases[0].Steps[0].Status != StepRunStatusRunning || !run.Phases[0].Steps[0].AwaitingTurn {
		t.Fatalf("expected dispatched step to be running+awaiting_turn, got %#v", run.Phases[0].Steps[0])
	}

	stopped, err := service.StopRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StopRun: %v", err)
	}
	if stopped.Status != WorkflowRunStatusStopped {
		t.Fatalf("expected stopped status, got %q", stopped.Status)
	}
	phase := stopped.Phases[0]
	if phase.Status != PhaseRunStatusStopped {
		t.Fatalf("expected stopped phase, got %q", phase.Status)
	}
	if phase.CompletedAt == nil {
		t.Fatalf("expected stopped phase completion timestamp")
	}
	step := phase.Steps[0]
	if step.Status != StepRunStatusStopped {
		t.Fatalf("expected stopped step, got %q", step.Status)
	}
	if step.AwaitingTurn {
		t.Fatalf("expected stopped step to clear awaiting_turn")
	}
	if step.CompletedAt == nil {
		t.Fatalf("expected stopped step completion timestamp")
	}
	if strings.TrimSpace(step.Error) != "workflow run stopped by user" {
		t.Fatalf("expected stop reason on step error, got %q", step.Error)
	}
	if step.Execution == nil || step.Execution.CompletedAt == nil {
		t.Fatalf("expected execution reference completion timestamp on stop, got %#v", step.Execution)
	}
	if len(step.ExecutionAttempts) == 0 || step.ExecutionAttempts[len(step.ExecutionAttempts)-1].CompletedAt == nil {
		t.Fatalf("expected execution attempt completion timestamp on stop, got %#v", step.ExecutionAttempts)
	}

	timeline, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	hasRunStopped := false
	hasPhaseStopped := false
	hasStepStopped := false
	for _, event := range timeline {
		switch event.Type {
		case "run_stopped":
			hasRunStopped = true
		case "phase_stopped":
			hasPhaseStopped = true
		case "step_stopped":
			hasStepStopped = true
		}
	}
	if !hasRunStopped || !hasPhaseStopped || !hasStepStopped {
		t.Fatalf("expected stop timeline events (run/phase/step), got %#v", timeline)
	}
}

func TestRunLifecycleStopRunInvalidTransitionFromCompletedAndFailed(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	completedRun, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun completed candidate: %v", err)
	}
	completedRun, err = service.StartRun(context.Background(), completedRun.ID)
	if err != nil {
		t.Fatalf("StartRun completed candidate: %v", err)
	}
	for i := 0; i < 16 && completedRun.Status == WorkflowRunStatusRunning; i++ {
		completedRun, err = service.AdvanceRun(context.Background(), completedRun.ID)
		if err != nil {
			t.Fatalf("AdvanceRun completed candidate iteration %d: %v", i, err)
		}
	}
	if completedRun.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed run status, got %q", completedRun.Status)
	}
	if _, err := service.StopRun(context.Background(), completedRun.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition when stopping completed run, got %v", err)
	}

	engine := NewEngine(WithStepHandler("implementation", func(context.Context, *WorkflowRun, *PhaseRun, *StepRun) error {
		return errors.New("forced failure")
	}))
	failedService := NewRunService(Config{Enabled: true}, WithEngine(engine))
	failedRun, err := failedService.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun failed candidate: %v", err)
	}
	failedRun, err = failedService.StartRun(context.Background(), failedRun.ID)
	if err != nil {
		t.Fatalf("StartRun failed candidate: %v", err)
	}
	if _, err := failedService.AdvanceRun(context.Background(), failedRun.ID); err == nil {
		t.Fatalf("expected advance failure to transition run to failed")
	}
	failedRun, err = failedService.GetRun(context.Background(), failedRun.ID)
	if err != nil {
		t.Fatalf("GetRun failed candidate: %v", err)
	}
	if failedRun.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run status, got %q", failedRun.Status)
	}
	if _, err := failedService.StopRun(context.Background(), failedRun.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition when stopping failed run, got %v", err)
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
	if _, err := enabled.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:       TemplateIDSolidPhaseDelivery,
		WorkspaceID:      "ws-1",
		SelectedProvider: "gemini",
	}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("expected ErrUnsupportedProvider, got %v", err)
	}
	run, err := enabled.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:       TemplateIDSolidPhaseDelivery,
		WorkspaceID:      "ws-1",
		SelectedProvider: "opencode",
	})
	if err != nil {
		t.Fatalf("expected valid selected provider to create run, got %v", err)
	}
	if run.SelectedProvider != "opencode" {
		t.Fatalf("expected normalized selected provider opencode, got %q", run.SelectedProvider)
	}
}

func TestWithDispatchProviderPolicyOverridesNormalization(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithDispatchProviderPolicy(stubDispatchProviderPolicy{normalized: "kilocode"}),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:       TemplateIDSolidPhaseDelivery,
		WorkspaceID:      "ws-1",
		SelectedProvider: "codex",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.SelectedProvider != "kilocode" {
		t.Fatalf("expected policy normalization override, got %q", run.SelectedProvider)
	}
}

func TestWithDispatchProviderPolicyValidationErrorPropagates(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithDispatchProviderPolicy(stubDispatchProviderPolicy{
			validateErr: errors.New("policy unavailable"),
		}),
	)
	if _, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:       TemplateIDSolidPhaseDelivery,
		WorkspaceID:      "ws-1",
		SelectedProvider: "codex",
	}); err == nil || !strings.Contains(err.Error(), "policy unavailable") {
		t.Fatalf("expected custom policy error, got %v", err)
	}
}

func TestDispatchProviderPolicyOrDefaultHandlesNil(t *testing.T) {
	var service *InMemoryRunService
	policy := service.dispatchProviderPolicyOrDefault()
	if policy == nil {
		t.Fatalf("expected default policy for nil service receiver")
	}
}

func TestWithDispatchProviderPolicyNilGuards(t *testing.T) {
	opt := WithDispatchProviderPolicy(nil)
	service := &InMemoryRunService{}
	opt(service)
	if service.dispatchProviderPolicy != nil {
		t.Fatalf("expected nil policy option to no-op")
	}

	var nilService *InMemoryRunService
	opt = WithDispatchProviderPolicy(stubDispatchProviderPolicy{normalized: "codex"})
	opt(nilService)
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

func TestRunLifecycleOnTurnCompletedStrictSessionMatchPreventsCrossSessionAdvance(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_session_isolation",
		Name: "Prompted Session Isolation",
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
			{Dispatched: true, SessionID: "sess-a", TurnID: "turn-a"},
			{Dispatched: true, SessionID: "sess-b", TurnID: "turn-b"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
	)

	runA, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_session_isolation",
		WorkspaceID: "ws-shared",
		WorktreeID:  "wt-shared",
		SessionID:   "sess-a",
	})
	if err != nil {
		t.Fatalf("CreateRun runA: %v", err)
	}
	runB, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_session_isolation",
		WorkspaceID: "ws-shared",
		WorktreeID:  "wt-shared",
		SessionID:   "sess-b",
	})
	if err != nil {
		t.Fatalf("CreateRun runB: %v", err)
	}

	if _, err := service.StartRun(context.Background(), runA.ID); err != nil {
		t.Fatalf("StartRun runA: %v", err)
	}
	if _, err := service.StartRun(context.Background(), runB.ID); err != nil {
		t.Fatalf("StartRun runB: %v", err)
	}

	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-a",
		TurnID:    "turn-a",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted runA: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run for sess-a turn, got %d", len(updated))
	}
	if updated[0].ID != runA.ID {
		t.Fatalf("expected runA update only, got run id %q", updated[0].ID)
	}

	gotA, err := service.GetRun(context.Background(), runA.ID)
	if err != nil {
		t.Fatalf("GetRun runA: %v", err)
	}
	if gotA.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected runA completed, got %q", gotA.Status)
	}

	gotB, err := service.GetRun(context.Background(), runB.ID)
	if err != nil {
		t.Fatalf("GetRun runB: %v", err)
	}
	if gotB.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected runB to remain running, got %q", gotB.Status)
	}
	if got := gotB.Phases[0].Steps[0]; got.Status != StepRunStatusRunning || !got.AwaitingTurn {
		t.Fatalf("expected runB step to remain awaiting turn, got %#v", got)
	}
}

func TestRunLifecycleOnTurnCompletedIgnoresWorkspaceOnlySignal(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_session_required",
		Name: "Prompted Session Required",
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
		TemplateID:  "prompted_session_required",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		TurnID:      "turn-a",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted workspace-only: %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("expected no updates for workspace-only turn signal, got %d", len(updated))
	}

	got, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected run to stay running, got %q", got.Status)
	}
	if step := got.Phases[0].Steps[0]; step.Status != StepRunStatusRunning || !step.AwaitingTurn {
		t.Fatalf("expected first step to remain awaiting turn, got %#v", step)
	}

	updated, err = service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted session-scoped: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one update for session-scoped signal, got %d", len(updated))
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

func TestRunLifecycleOnTurnCompletedTerminalFailureFailsStepAndRun(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_turn_failure",
		Name: "Prompted Turn Failure",
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
		TemplateID:  "prompted_turn_failure",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
		Status:    "failed",
		Error:     "model unsupported for this provider",
		Terminal:  true,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted failed turn: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run, got %d", len(updated))
	}
	current := updated[0]
	if current.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run status, got %q", current.Status)
	}
	if !strings.Contains(strings.ToLower(current.LastError), "model unsupported") {
		t.Fatalf("expected run LastError to include provider failure, got %q", current.LastError)
	}
	step := current.Phases[0].Steps[0]
	if step.Status != StepRunStatusFailed {
		t.Fatalf("expected failed step status, got %q", step.Status)
	}
	if !strings.Contains(strings.ToLower(step.Error), "model unsupported") {
		t.Fatalf("expected step error to include provider failure, got %q", step.Error)
	}
	timeline, err := service.GetRunTimeline(context.Background(), current.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	hasStepFailed := false
	hasRunFailed := false
	for _, event := range timeline {
		if event.Type == "step_failed" && strings.Contains(strings.ToLower(event.Message), "model unsupported") {
			hasStepFailed = true
		}
		if event.Type == "run_failed" && strings.Contains(strings.ToLower(event.Message), "model unsupported") {
			hasRunFailed = true
		}
	}
	if !hasStepFailed {
		t.Fatalf("expected step_failed timeline event with failure detail")
	}
	if !hasRunFailed {
		t.Fatalf("expected run_failed timeline event with failure detail")
	}
}

func TestRunLifecycleOnTurnCompletedDelegatesOutcomeToEvaluator(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_turn_outcome_evaluator",
		Name: "Prompted Turn Outcome Evaluator",
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
	evaluator := &stubStepOutcomeEvaluator{
		evaluation: StepOutcomeEvaluation{
			Decision:      StepOutcomeDecisionFailure,
			FailureDetail: "classifier rejected output",
			Outcome:       "failed",
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithStepOutcomeEvaluator(evaluator),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_turn_outcome_evaluator",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
		Status:    "completed",
		Terminal:  true,
		Output:    "assistant final output",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run, got %d", len(updated))
	}
	current := updated[0]
	if current.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run from evaluator decision, got %q", current.Status)
	}
	step := current.Phases[0].Steps[0]
	if step.Status != StepRunStatusFailed {
		t.Fatalf("expected failed step from evaluator decision, got %q", step.Status)
	}
	if !strings.Contains(strings.ToLower(step.Error), "classifier rejected output") {
		t.Fatalf("expected evaluator failure detail on step error, got %q", step.Error)
	}
	if step.Output != "assistant final output" {
		t.Fatalf("expected step output to capture turn output, got %q", step.Output)
	}
	if len(evaluator.inputs) != 1 {
		t.Fatalf("expected evaluator to receive one turn input, got %d", len(evaluator.inputs))
	}
	if evaluator.inputs[0].Signal.Status != "completed" {
		t.Fatalf("expected evaluator signal status to include completed, got %q", evaluator.inputs[0].Signal.Status)
	}
}

func TestRunLifecycleOnTurnCompletedUnknownDecisionDefersOutcome(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_turn_outcome_unknown",
		Name: "Prompted Turn Outcome Unknown",
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
	evaluator := &stubStepOutcomeEvaluator{
		evaluation: StepOutcomeEvaluation{
			Decision: StepOutcomeDecision("unknown-new-value"),
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithStepOutcomeEvaluator(evaluator),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_turn_outcome_unknown",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
		Status:    "completed",
		Terminal:  true,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run, got %d", len(updated))
	}
	current := updated[0]
	if current.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected run to remain running when evaluator outcome is unknown, got %q", current.Status)
	}
	step := current.Phases[0].Steps[0]
	if step.Status != StepRunStatusRunning || !step.AwaitingTurn {
		t.Fatalf("expected step to remain awaiting turn, got %#v", step)
	}
	timeline, err := service.GetRunTimeline(context.Background(), current.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	foundDeferred := false
	for _, event := range timeline {
		if event.Type == "step_turn_outcome_deferred" && strings.Contains(strings.ToLower(event.Message), "invalid step outcome decision") {
			foundDeferred = true
			break
		}
	}
	if !foundDeferred {
		t.Fatalf("expected deferred timeline event for unknown evaluator decision")
	}
}

func TestRunLifecycleOnTurnCompletedRetainsTurnContextForFutureEvaluation(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_turn_context",
		Name: "Prompted Turn Context",
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
		TemplateID:  "prompted_turn_context",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	payload := map[string]any{
		"turn_status": "completed",
		"trace_id":    "trace-123",
		"nested": map[string]any{
			"result": "ok",
		},
		"items": []any{
			map[string]any{"k": "v"},
			"stable",
		},
	}
	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
		Status:    "completed",
		Terminal:  true,
		Provider:  "codex",
		Source:    "opencode_event",
		Output:    "done",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	payload["trace_id"] = "mutated"
	payloadNested, _ := payload["nested"].(map[string]any)
	payloadNested["result"] = "mutated"
	payloadItems, _ := payload["items"].([]any)
	payloadItems[0].(map[string]any)["k"] = "mutated"
	if len(updated) != 1 {
		t.Fatalf("expected one updated run, got %d", len(updated))
	}
	step := updated[0].Phases[0].Steps[0]
	if step.Status != StepRunStatusCompleted {
		t.Fatalf("expected completed step, got %q", step.Status)
	}
	if step.LastTurnSignal == nil {
		t.Fatalf("expected last turn signal context to be retained")
	}
	if step.LastTurnSignal.TurnID != "turn-a" || step.LastTurnSignal.Provider != "codex" {
		t.Fatalf("unexpected retained turn context: %#v", step.LastTurnSignal)
	}
	if step.LastTurnSignal.Payload["trace_id"] != "trace-123" {
		t.Fatalf("expected retained payload for future evaluator context, got %#v", step.LastTurnSignal.Payload)
	}
	nested, ok := step.LastTurnSignal.Payload["nested"].(map[string]any)
	if !ok || nested["result"] != "ok" {
		t.Fatalf("expected retained nested payload for future evaluator context, got %#v", step.LastTurnSignal.Payload)
	}
	items, ok := step.LastTurnSignal.Payload["items"].([]any)
	if !ok {
		t.Fatalf("expected retained array payload for future evaluator context, got %#v", step.LastTurnSignal.Payload)
	}
	if items[0].(map[string]any)["k"] != "v" {
		t.Fatalf("expected retained nested array payload for future evaluator context, got %#v", step.LastTurnSignal.Payload)
	}
}

func TestRunLifecycleOnTurnCompletedFailureOutcomeDefaultsWhenEvaluatorOmitsFields(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_turn_failure_defaults",
		Name: "Prompted Turn Failure Defaults",
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
	evaluator := &stubStepOutcomeEvaluator{
		evaluation: StepOutcomeEvaluation{
			Decision: StepOutcomeDecisionFailure,
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithStepOutcomeEvaluator(evaluator),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_turn_failure_defaults",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
		Status:    "completed",
		Terminal:  true,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run, got %d", len(updated))
	}
	step := updated[0].Phases[0].Steps[0]
	if step.Status != StepRunStatusFailed {
		t.Fatalf("expected failed step from evaluator decision, got %q", step.Status)
	}
	if step.Outcome != "failed" {
		t.Fatalf("expected default failed outcome, got %q", step.Outcome)
	}
	if !strings.Contains(strings.ToLower(step.Error), "turn outcome evaluator returned failure") {
		t.Fatalf("expected default failure detail, got %q", step.Error)
	}
}

func TestStepOutcomeEvaluatorOrDefaultFallback(t *testing.T) {
	var nilService *InMemoryRunService
	if _, ok := nilService.stepOutcomeEvaluatorOrDefault().(defaultStepOutcomeEvaluator); !ok {
		t.Fatalf("expected nil service to resolve default evaluator")
	}
	service := &InMemoryRunService{}
	if _, ok := service.stepOutcomeEvaluatorOrDefault().(defaultStepOutcomeEvaluator); !ok {
		t.Fatalf("expected nil evaluator to resolve default evaluator")
	}
	custom := &stubStepOutcomeEvaluator{}
	service.stepOutcomeEvaluator = custom
	if resolved := service.stepOutcomeEvaluatorOrDefault(); resolved != custom {
		t.Fatalf("expected custom evaluator to be preserved")
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

func TestRunLifecyclePromptDispatchIncludesStepRuntimeOptions(t *testing.T) {
	stepRuntimeOptions := &types.SessionRuntimeOptions{
		Model:     "gpt-5.2-codex",
		Reasoning: types.ReasoningHigh,
		Access:    types.AccessFull,
	}
	template := WorkflowTemplate{
		ID:   "prompted_with_runtime_options",
		Name: "Prompted with runtime options",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []WorkflowTemplateStep{
					{
						ID:             "step_1",
						Name:           "step 1",
						Prompt:         "prompt 1",
						RuntimeOptions: stepRuntimeOptions,
					},
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
		TemplateID:  "prompted_with_runtime_options",
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
	if step.RuntimeOptions == nil {
		t.Fatalf("expected step runtime options to be present on run step")
	}
	if step.RuntimeOptions == stepRuntimeOptions {
		t.Fatalf("expected step runtime options to be cloned for run state")
	}
	if step.RuntimeOptions.Model != "gpt-5.2-codex" || step.RuntimeOptions.Reasoning != types.ReasoningHigh || step.RuntimeOptions.Access != types.AccessFull {
		t.Fatalf("unexpected run step runtime options: %#v", step.RuntimeOptions)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected one prompt dispatch call, got %d", len(dispatcher.calls))
	}
	if dispatcher.calls[0].RuntimeOptions == nil {
		t.Fatalf("expected runtime options in dispatch request")
	}
	if dispatcher.calls[0].RuntimeOptions == stepRuntimeOptions {
		t.Fatalf("expected dispatch runtime options to be cloned")
	}
	if dispatcher.calls[0].RuntimeOptions.Model != "gpt-5.2-codex" {
		t.Fatalf("unexpected dispatch model override: %q", dispatcher.calls[0].RuntimeOptions.Model)
	}
	if dispatcher.calls[0].RuntimeOptions.Reasoning != types.ReasoningHigh {
		t.Fatalf("unexpected dispatch reasoning override: %q", dispatcher.calls[0].RuntimeOptions.Reasoning)
	}
	if dispatcher.calls[0].RuntimeOptions.Access != types.AccessFull {
		t.Fatalf("unexpected dispatch access override: %q", dispatcher.calls[0].RuntimeOptions.Access)
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

func TestRunLifecyclePromptDispatchDeferredRetriesAndSucceeds(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_deferred",
		Name: "Prompted Deferred",
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
		errs: []error{
			fmt.Errorf("%w: turn already in progress", ErrStepDispatchDeferred),
		},
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
		TemplateID:  "prompted_deferred",
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
		t.Fatalf("expected running status after deferred dispatch, got %q", run.Status)
	}
	if got := run.Phases[0].Steps[0]; got.ExecutionState != StepExecutionStateDeferred || got.Outcome != "waiting_dispatch" {
		t.Fatalf("expected first step deferred state after initial dispatch contention, got %#v", got)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := service.GetRun(context.Background(), run.ID)
		if getErr != nil {
			t.Fatalf("GetRun while waiting for retry: %v", getErr)
		}
		step := current.Phases[0].Steps[0]
		if step.Status == StepRunStatusRunning && step.AwaitingTurn && step.ExecutionState == StepExecutionStateLinked {
			run = current
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if got := run.Phases[0].Steps[0]; got.Status != StepRunStatusRunning || !got.AwaitingTurn {
		t.Fatalf("expected deferred dispatch retry to eventually dispatch step, got %#v", got)
	}
	if len(dispatcher.calls) < 2 {
		t.Fatalf("expected retry loop to re-attempt dispatch, got %d dispatch call(s)", len(dispatcher.calls))
	}
	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after turn completion, got %d", len(updated))
	}
	if updated[0].Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected run completion after retried dispatch turn completes, got %q", updated[0].Status)
	}
}

func TestRunLifecycleOnTurnCompletedIgnoresMismatchedTurnID(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_turn_match",
		Name: "Prompted Turn Match",
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
		TemplateID:  "prompted_turn_match",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-b",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted mismatched: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one running update for mismatched turn signal, got %d", len(updated))
	}
	current, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected run to stay running after mismatched turn, got %q", current.Status)
	}
	if got := current.Phases[0].Steps[0]; got.Status != StepRunStatusRunning || !got.AwaitingTurn || got.TurnID != "turn-a" {
		t.Fatalf("expected awaiting step to remain unchanged after mismatched turn, got %#v", got)
	}
	updated, err = service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-a",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted matched: %v", err)
	}
	if len(updated) != 1 || updated[0].Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completion after matching turn signal, got %#v", updated)
	}
}

func TestRunLifecycleCustomDispatchClassifierFatalFailsDeferredError(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_classifier_fatal",
		Name: "Prompted Classifier Fatal",
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
		WithStepPromptDispatcher(&stubStepPromptDispatcher{err: ErrStepDispatchDeferred}),
		WithDispatchErrorClassifier(stubDispatchErrorClassifier{disposition: DispatchErrorDispositionFatal}),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_classifier_fatal",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err == nil {
		t.Fatalf("expected start to fail when classifier marks deferred error fatal")
	} else if !errors.Is(err, ErrStepDispatchDeferred) {
		t.Fatalf("expected ErrStepDispatchDeferred, got %v", err)
	}
	current, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run with fatal classifier, got %q", current.Status)
	}
}

func TestRunLifecycleCustomDispatchClassifierDeferredUsesInjectedScheduler(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_classifier_deferred",
		Name: "Prompted Classifier Deferred",
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
	scheduler := &stubDispatchRetryScheduler{}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(&stubStepPromptDispatcher{err: errors.New("synthetic non-deferred dispatch error")}),
		WithDispatchErrorClassifier(stubDispatchErrorClassifier{disposition: DispatchErrorDispositionDeferred}),
		WithDispatchRetryScheduler(scheduler),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_classifier_deferred",
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
		t.Fatalf("expected run to stay running with deferred classifier, got %q", run.Status)
	}
	step := run.Phases[0].Steps[0]
	if step.Status != StepRunStatusPending || step.ExecutionState != StepExecutionStateDeferred || step.Outcome != "waiting_dispatch" {
		t.Fatalf("expected deferred step state, got %#v", step)
	}
	if len(scheduler.enqueued) != 1 || scheduler.enqueued[0] != run.ID {
		t.Fatalf("expected injected scheduler to enqueue run once, got %#v", scheduler.enqueued)
	}
}

func TestRunLifecycleCustomDispatchRetryPolicyWiresDefaultScheduler(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_policy_wiring",
		Name: "Prompted Policy Wiring",
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
	policy := &countingDispatchRetryPolicy{delay: 1 * time.Millisecond}
	dispatcher := &stubStepPromptDispatcher{
		errs: []error{
			ErrStepDispatchDeferred,
		},
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-a"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithDispatchRetryPolicy(policy),
		WithStepPromptDispatcher(dispatcher),
	)
	defer service.Close()
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_policy_wiring",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && policy.calls.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if policy.calls.Load() == 0 {
		t.Fatalf("expected custom retry policy to be used by default scheduler")
	}
}

func TestRunServiceCloseClosesInjectedSchedulerOnce(t *testing.T) {
	scheduler := &stubDispatchRetryScheduler{}
	service := NewRunService(
		Config{Enabled: true},
		WithDispatchRetryScheduler(scheduler),
	)
	service.Close()
	service.Close()
	if scheduler.closeCalls != 1 {
		t.Fatalf("expected scheduler close to be idempotent through service.Close, got %d calls", scheduler.closeCalls)
	}
}

func TestRetryDeferredDispatchDefensiveBranches(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	if done := service.retryDeferredDispatch(""); !done {
		t.Fatalf("expected empty run id to return done")
	}
	if done := service.retryDeferredDispatch("missing-run"); !done {
		t.Fatalf("expected missing run id to return done")
	}
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if done := service.retryDeferredDispatch(run.ID); !done {
		t.Fatalf("expected non-running run to return done")
	}
}

func TestRetryDeferredDispatchReturnsDoneWhenAdvanceFails(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_retry_advance_fail",
		Name: "Prompted Retry Advance Fail",
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
		errs: []error{
			ErrStepDispatchDeferred,
			errors.New("fatal dispatch error"),
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithDispatchRetryScheduler(&stubDispatchRetryScheduler{}),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_retry_advance_fail",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if done := service.retryDeferredDispatch(run.ID); !done {
		t.Fatalf("expected retry to return done when advance fails")
	}
	current, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected run to fail when retry dispatch errors fatally, got %q", current.Status)
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

func TestRunLifecycleMaxActiveRunsDismissedRunDoesNotCount(t *testing.T) {
	service := NewRunService(Config{Enabled: true}, WithMaxActiveRuns(1))
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun first: %v", err)
	}
	dismissed, err := service.DismissRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("DismissRun: %v", err)
	}
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed run to set dismissed_at")
	}
	if _, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	}); err != nil {
		t.Fatalf("expected dismissed run to be excluded from active limit, got %v", err)
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

func TestRunLifecycleRenameRun(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	renamed, err := service.RenameRun(context.Background(), run.ID, "Renamed Workflow")
	if err != nil {
		t.Fatalf("RenameRun: %v", err)
	}
	if strings.TrimSpace(renamed.TemplateName) != "Renamed Workflow" {
		t.Fatalf("expected renamed workflow name, got %q", renamed.TemplateName)
	}
	loaded, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if strings.TrimSpace(loaded.TemplateName) != "Renamed Workflow" {
		t.Fatalf("expected persisted renamed workflow name, got %q", loaded.TemplateName)
	}
}

func TestRunLifecycleRenameRunValidation(t *testing.T) {
	var nilService *InMemoryRunService
	if _, err := nilService.RenameRun(context.Background(), "gwf-any", "Renamed Workflow"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition for nil service, got %v", err)
	}

	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.RenameRun(context.Background(), run.ID, "   "); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition for blank name, got %v", err)
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

func TestRunLifecycleDismissRunValidation(t *testing.T) {
	var nilService *InMemoryRunService
	if _, err := nilService.DismissRun(context.Background(), "gwf-any"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition for nil service, got %v", err)
	}

	service := NewRunService(Config{Enabled: true})
	if _, err := service.DismissRun(context.Background(), "   "); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition for blank run id, got %v", err)
	}
}

func TestRunLifecycleDismissMissingRunCreatesTombstone(t *testing.T) {
	snapshotStore := &stubRunSnapshotStore{}
	service := NewRunService(
		Config{Enabled: true},
		WithRunSnapshotStore(snapshotStore),
		WithMissingRunContextResolver(&stubMissingRunContextResolver{
			contextByRunID: map[string]MissingRunDismissalContext{
				"gwf-missing": {
					WorkspaceID: "ws-1",
					WorktreeID:  "wt-1",
					SessionID:   "sess-1",
				},
			},
		}),
	)

	dismissed, err := service.DismissRun(context.Background(), "gwf-missing")
	if err != nil {
		t.Fatalf("DismissRun missing: %v", err)
	}
	if dismissed.ID != "gwf-missing" {
		t.Fatalf("expected run id gwf-missing, got %q", dismissed.ID)
	}
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set")
	}
	if dismissed.WorkspaceID != "ws-1" || dismissed.WorktreeID != "wt-1" || dismissed.SessionID != "sess-1" {
		t.Fatalf("expected resolver context to be applied, got %#v", dismissed)
	}
	if len(snapshotStore.savedByRunID) != 1 {
		t.Fatalf("expected dismissed tombstone to be persisted, got %d", len(snapshotStore.savedByRunID))
	}
	saved := snapshotStore.savedByRunID["gwf-missing"]
	if saved.Run == nil || saved.Run.DismissedAt == nil {
		t.Fatalf("expected persisted tombstone with dismissed_at, got %#v", saved.Run)
	}

	visible, err := service.ListRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("expected missing dismissed run hidden from default list, got %#v", visible)
	}

	restarted := NewRunService(Config{Enabled: true}, WithRunSnapshotStore(snapshotStore))
	afterRestartVisible, err := restarted.ListRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRuns after restart: %v", err)
	}
	if len(afterRestartVisible) != 0 {
		t.Fatalf("expected dismissed tombstone to stay hidden after restart, got %#v", afterRestartVisible)
	}
	includingDismissed, err := restarted.ListRunsIncludingDismissed(context.Background())
	if err != nil {
		t.Fatalf("ListRunsIncludingDismissed after restart: %v", err)
	}
	if len(includingDismissed) != 1 || includingDismissed[0].ID != "gwf-missing" || includingDismissed[0].DismissedAt == nil {
		t.Fatalf("expected dismissed tombstone after restart, got %#v", includingDismissed)
	}

	undismissed, err := restarted.UndismissRun(context.Background(), "gwf-missing")
	if err != nil {
		t.Fatalf("UndismissRun missing tombstone: %v", err)
	}
	if undismissed.DismissedAt != nil {
		t.Fatalf("expected dismissed_at to clear after explicit undismiss")
	}
	afterUndismiss, err := restarted.ListRuns(context.Background())
	if err != nil {
		t.Fatalf("ListRuns after undismiss: %v", err)
	}
	if len(afterUndismiss) != 1 || afterUndismiss[0].ID != "gwf-missing" {
		t.Fatalf("expected undismissed tombstone to become visible, got %#v", afterUndismiss)
	}
}

func TestRunLifecycleDismissMissingRunResolverErrorFallsBackToDefaultTombstone(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithMissingRunContextResolver(&stubMissingRunContextResolver{
			err: errors.New("resolver unavailable"),
		}),
	)

	dismissed, err := service.DismissRun(context.Background(), "gwf-resolver-fallback")
	if err != nil {
		t.Fatalf("DismissRun missing with resolver error: %v", err)
	}
	if dismissed.ID != "gwf-resolver-fallback" {
		t.Fatalf("expected run id to be preserved, got %q", dismissed.ID)
	}
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set")
	}
	if dismissed.WorkspaceID != "" || dismissed.WorktreeID != "" || dismissed.SessionID != "" {
		t.Fatalf("expected unresolved context to remain empty, got %#v", dismissed)
	}
	if dismissed.LastError != "workflow run missing; dismissed tombstone created" {
		t.Fatalf("expected fallback missing-run error message, got %q", dismissed.LastError)
	}
}

func TestRunLifecycleDismissMissingRunCustomFactoryNilFallsBackToDefault(t *testing.T) {
	factory := &stubMissingRunTombstoneFactory{}
	service := NewRunService(
		Config{Enabled: true},
		WithMissingRunTombstoneFactory(factory),
		WithMissingRunContextResolver(&stubMissingRunContextResolver{
			contextByRunID: map[string]MissingRunDismissalContext{
				"gwf-custom-fallback": {
					WorkspaceID: "ws-fallback",
					WorktreeID:  "wt-fallback",
					SessionID:   "sess-fallback",
				},
			},
		}),
	)

	dismissed, err := service.DismissRun(context.Background(), "gwf-custom-fallback")
	if err != nil {
		t.Fatalf("DismissRun missing with nil custom tombstone: %v", err)
	}
	if dismissed.TemplateName != "Historical Guided Workflow" {
		t.Fatalf("expected default template name fallback, got %q", dismissed.TemplateName)
	}
	if dismissed.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed fallback status, got %q", dismissed.Status)
	}
	if dismissed.LastError != "workflow run missing; visibility state restored from dismissal" {
		t.Fatalf("expected resolved-context fallback message, got %q", dismissed.LastError)
	}
	if dismissed.WorkspaceID != "ws-fallback" || dismissed.WorktreeID != "wt-fallback" || dismissed.SessionID != "sess-fallback" {
		t.Fatalf("expected resolver context to be backfilled, got %#v", dismissed)
	}
	if len(factory.calls) != 1 {
		t.Fatalf("expected custom factory call before fallback, got %d", len(factory.calls))
	}
}

func TestRunLifecycleDismissMissingRunCustomFactoryOutputIsNormalized(t *testing.T) {
	factory := &stubMissingRunTombstoneFactory{
		runByID: map[string]*WorkflowRun{
			"gwf-normalized": {
				TemplateName: "   ",
				WorkspaceID:  " ",
				WorktreeID:   "",
				SessionID:    " ",
				LastError:    " ",
			},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithMissingRunTombstoneFactory(factory),
		WithMissingRunContextResolver(&stubMissingRunContextResolver{
			contextByRunID: map[string]MissingRunDismissalContext{
				"gwf-normalized": {
					WorkspaceID: "ws-1",
					WorktreeID:  "wt-1",
					SessionID:   "sess-1",
				},
			},
		}),
	)

	dismissed, err := service.DismissRun(context.Background(), "  gwf-normalized  ")
	if err != nil {
		t.Fatalf("DismissRun missing with sparse custom tombstone: %v", err)
	}
	if dismissed.ID != "gwf-normalized" {
		t.Fatalf("expected normalized run id, got %q", dismissed.ID)
	}
	if dismissed.TemplateName != "Historical Guided Workflow" {
		t.Fatalf("expected normalized default template name, got %q", dismissed.TemplateName)
	}
	if dismissed.WorkspaceID != "ws-1" || dismissed.WorktreeID != "wt-1" || dismissed.SessionID != "sess-1" {
		t.Fatalf("expected normalized context backfill, got %#v", dismissed)
	}
	if dismissed.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected normalized fallback status, got %q", dismissed.Status)
	}
	if dismissed.CreatedAt.IsZero() {
		t.Fatalf("expected created_at to be backfilled")
	}
	if dismissed.LastError != "workflow run missing; visibility state restored from dismissal" {
		t.Fatalf("expected normalized fallback error, got %q", dismissed.LastError)
	}
}

func TestRunLifecycleDismissMissingRunUsesCustomTombstoneFactory(t *testing.T) {
	factory := &stubMissingRunTombstoneFactory{
		runByID: map[string]*WorkflowRun{
			"gwf-custom": {
				TemplateName: "Custom Tombstone",
				LastError:    "custom fallback message",
			},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithMissingRunTombstoneFactory(factory),
		WithMissingRunContextResolver(&stubMissingRunContextResolver{
			contextByRunID: map[string]MissingRunDismissalContext{
				"gwf-custom": {
					WorkspaceID: "ws-custom",
					WorktreeID:  "wt-custom",
					SessionID:   "sess-custom",
				},
			},
		}),
	)

	dismissed, err := service.DismissRun(context.Background(), "gwf-custom")
	if err != nil {
		t.Fatalf("DismissRun missing with custom factory: %v", err)
	}
	if dismissed.ID != "gwf-custom" {
		t.Fatalf("expected run id gwf-custom, got %q", dismissed.ID)
	}
	if dismissed.TemplateName != "Custom Tombstone" {
		t.Fatalf("expected custom template name, got %q", dismissed.TemplateName)
	}
	if dismissed.LastError != "custom fallback message" {
		t.Fatalf("expected custom missing-run message, got %q", dismissed.LastError)
	}
	if dismissed.WorkspaceID != "ws-custom" || dismissed.WorktreeID != "wt-custom" || dismissed.SessionID != "sess-custom" {
		t.Fatalf("expected resolver context to backfill custom tombstone, got %#v", dismissed)
	}
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set")
	}
	if len(factory.calls) != 1 {
		t.Fatalf("expected custom factory to be called exactly once, got %d", len(factory.calls))
	}
	if factory.calls[0].runID != "gwf-custom" || !factory.calls[0].contextResolved {
		t.Fatalf("unexpected custom factory call metadata: %#v", factory.calls[0])
	}
	if factory.calls[0].context.WorkspaceID != "ws-custom" || factory.calls[0].context.WorktreeID != "wt-custom" || factory.calls[0].context.SessionID != "sess-custom" {
		t.Fatalf("unexpected custom factory context: %#v", factory.calls[0].context)
	}
}

func TestRunLifecyclePersistsSnapshotsAcrossServiceRestart(t *testing.T) {
	snapshotStore := &stubRunSnapshotStore{}
	service := NewRunService(Config{Enabled: true}, WithRunSnapshotStore(snapshotStore))

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		UserPrompt:  "persist this run",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if len(snapshotStore.savedByRunID) == 0 {
		t.Fatalf("expected run snapshots to be persisted")
	}

	restarted := NewRunService(Config{Enabled: true}, WithRunSnapshotStore(snapshotStore))
	loaded, err := restarted.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun after restart: %v", err)
	}
	if loaded == nil || loaded.ID != run.ID {
		t.Fatalf("expected persisted run %q after restart, got %#v", run.ID, loaded)
	}
	timeline, err := restarted.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline after restart: %v", err)
	}
	if len(timeline) == 0 {
		t.Fatalf("expected persisted timeline after restart")
	}
}

func TestRunLifecycleRestoreRunningRunMarksInterrupted(t *testing.T) {
	startedAt := time.Now().UTC().Add(-5 * time.Minute)
	runID := "gwf-restart-running"
	snapshotStore := &stubRunSnapshotStore{
		loadSnapshots: []RunStatusSnapshot{
			{
				Run: &WorkflowRun{
					ID:           runID,
					TemplateID:   TemplateIDSolidPhaseDelivery,
					TemplateName: "Solid Phase Delivery",
					WorkspaceID:  "ws-1",
					WorktreeID:   "wt-1",
					Status:       WorkflowRunStatusRunning,
					CreatedAt:    startedAt.Add(-2 * time.Minute),
					StartedAt:    &startedAt,
				},
				Timeline: []RunTimelineEvent{
					{At: startedAt, Type: "run_started", RunID: runID},
				},
			},
		},
	}

	restarted := NewRunService(Config{Enabled: true}, WithRunSnapshotStore(snapshotStore))
	loaded, err := restarted.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun after restart: %v", err)
	}
	if loaded.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected running run to be marked failed after restart, got %q", loaded.Status)
	}
	if loaded.CompletedAt == nil {
		t.Fatalf("expected interrupted run to set completed_at")
	}
	if !strings.Contains(loaded.LastError, "interrupted by daemon restart") {
		t.Fatalf("expected restart interruption reason, got %q", loaded.LastError)
	}
	timeline, err := restarted.GetRunTimeline(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunTimeline after restart: %v", err)
	}
	if len(timeline) == 0 || timeline[len(timeline)-1].Type != "run_interrupted" {
		t.Fatalf("expected run_interrupted timeline event, got %#v", timeline)
	}
	saved, ok := snapshotStore.savedByRunID[runID]
	if !ok || saved.Run == nil {
		t.Fatalf("expected interrupted run snapshot to be persisted")
	}
	if saved.Run.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected persisted interrupted run status failed, got %q", saved.Run.Status)
	}
}

func TestRunLifecycleRestorePausedRunPreservesState(t *testing.T) {
	pausedAt := time.Now().UTC().Add(-3 * time.Minute)
	runID := "gwf-restart-paused"
	snapshotStore := &stubRunSnapshotStore{
		loadSnapshots: []RunStatusSnapshot{
			{
				Run: &WorkflowRun{
					ID:           runID,
					TemplateID:   TemplateIDSolidPhaseDelivery,
					TemplateName: "Solid Phase Delivery",
					WorkspaceID:  "ws-1",
					WorktreeID:   "wt-1",
					Status:       WorkflowRunStatusPaused,
					CreatedAt:    pausedAt.Add(-10 * time.Minute),
					PausedAt:     &pausedAt,
				},
				Timeline: []RunTimelineEvent{
					{At: pausedAt, Type: "run_paused", RunID: runID},
				},
			},
		},
	}

	restarted := NewRunService(Config{Enabled: true}, WithRunSnapshotStore(snapshotStore))
	loaded, err := restarted.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun after restart: %v", err)
	}
	if loaded.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused run state to be preserved, got %q", loaded.Status)
	}
	if loaded.CompletedAt != nil {
		t.Fatalf("expected paused run not to be marked completed")
	}
	timeline, err := restarted.GetRunTimeline(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRunTimeline after restart: %v", err)
	}
	for _, event := range timeline {
		if event.Type == "run_interrupted" {
			t.Fatalf("did not expect interrupted event for paused run")
		}
	}
}

func TestRunMetricsDispatchReliabilityCounters(t *testing.T) {
	tpl := WorkflowTemplate{
		ID:   "dispatch_metrics",
		Name: "Dispatch Metrics",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase-1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step-1", Name: "Step 1", Prompt: "do work"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		errs: []error{ErrStepDispatchDeferred},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(tpl),
		WithStepPromptDispatcher(dispatcher),
		WithDispatchRetryScheduler(&stubDispatchRetryScheduler{}),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  tpl.ID,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun with deferred dispatch should not fail: %v", err)
	}
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.DispatchAttempts != 1 || metrics.DispatchDeferred != 1 || metrics.DispatchFailures != 0 {
		t.Fatalf("unexpected deferred dispatch metrics: %#v", metrics)
	}

	failedDispatcher := &stubStepPromptDispatcher{err: errors.New("dispatcher crashed")}
	failedService := NewRunService(
		Config{Enabled: true},
		WithTemplate(tpl),
		WithStepPromptDispatcher(failedDispatcher),
	)
	run, err = failedService.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  tpl.ID,
		WorkspaceID: "ws-2",
		WorktreeID:  "wt-2",
	})
	if err != nil {
		t.Fatalf("CreateRun (failed path): %v", err)
	}
	if _, err := failedService.StartRun(context.Background(), run.ID); err == nil {
		t.Fatalf("expected StartRun to fail on fatal dispatch error")
	}
	metrics, err = failedService.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics (failed path): %v", err)
	}
	if metrics.DispatchAttempts != 1 || metrics.DispatchFailures != 1 {
		t.Fatalf("expected fatal dispatch counters to increment, got %#v", metrics)
	}
}

func TestRunMetricsTurnProgressionReliabilityCounters(t *testing.T) {
	tpl := WorkflowTemplate{
		ID:   "turn_metrics",
		Name: "Turn Metrics",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase-1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step-1", Name: "Step 1", Prompt: "step one"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-1", Provider: "codex"},
		},
	}
	evaluator := &stubStepOutcomeEvaluator{
		evaluation: StepOutcomeEvaluation{
			Decision:      StepOutcomeDecisionUndetermined,
			SuccessDetail: "waiting for more evidence",
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(tpl),
		WithStepPromptDispatcher(dispatcher),
		WithStepOutcomeEvaluator(evaluator),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  tpl.ID,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-mismatch",
		Status:    "completed",
		Terminal:  true,
	}); err != nil {
		t.Fatalf("OnTurnCompleted mismatched turn: %v", err)
	}
	if _, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Status:    "completed",
		Terminal:  true,
		Output:    "partial",
	}); err != nil {
		t.Fatalf("OnTurnCompleted undetermined: %v", err)
	}
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.TurnEventsReceived != 2 {
		t.Fatalf("expected 2 received turn events, got %#v", metrics)
	}
	if metrics.TurnEventsMatched != 2 {
		t.Fatalf("expected 2 matched turn events, got %#v", metrics)
	}
	if metrics.TurnEventsBlocked != 1 {
		t.Fatalf("expected one blocked turn event, got %#v", metrics)
	}
	if metrics.TurnEventsStepDone != 1 {
		t.Fatalf("expected one step-done turn event, got %#v", metrics)
	}
	if metrics.TurnEventsAdvance != 0 {
		t.Fatalf("expected zero advance-only turn events, got %#v", metrics)
	}
	if metrics.TurnEventsProgressed != 1 {
		t.Fatalf("expected one progressed turn event, got %#v", metrics)
	}
	if metrics.StepOutcomeDeferred != 1 {
		t.Fatalf("expected one deferred step outcome, got %#v", metrics)
	}
}

func TestRunMetricsRestoreLegacyProgressedCounterBackfillsAdvance(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:              true,
			TurnEventsProgressed: 5,
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithRunMetricsStore(store),
	)
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.TurnEventsProgressed != 5 {
		t.Fatalf("expected legacy progressed count to restore, got %#v", metrics)
	}
	if metrics.TurnEventsAdvance != 5 {
		t.Fatalf("expected legacy progressed to backfill advance-only counter, got %#v", metrics)
	}
	if metrics.TurnEventsStepDone != 0 {
		t.Fatalf("expected no step-done count from legacy snapshot, got %#v", metrics)
	}
}

func TestRunMetricsRestoreComputesProgressedFromExplicitCounters(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:              true,
			TurnEventsStepDone:   2,
			TurnEventsAdvance:    3,
			TurnEventsProgressed: 0,
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithRunMetricsStore(store),
	)
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.TurnEventsStepDone != 2 || metrics.TurnEventsAdvance != 3 {
		t.Fatalf("expected explicit turn semantics to restore, got %#v", metrics)
	}
	if metrics.TurnEventsProgressed != 5 {
		t.Fatalf("expected progressed to be derived from explicit counters, got %#v", metrics)
	}
}
