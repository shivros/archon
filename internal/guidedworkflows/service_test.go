package guidedworkflows

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	mu            sync.RWMutex
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
	started   chan struct{}
	release   <-chan struct{}
	startOnce sync.Once
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

type stubGateCoordinator struct {
	hasActive         bool
	hasDeferred       bool
	pendingResolution GateResolution
	pendingErr        error
	signalResolution  GateResolution
	signalErr         error
	pendingCtx        context.Context
	signalCtx         context.Context
}

type stubGateSignalMatcher struct {
	matches bool
	calls   int
}

type stubGateSignalAdapter struct {
	signal GateSignal
	calls  int
	last   TurnSignal
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

func (s *stubGateCoordinator) HasActiveGate(_ *WorkflowRun) bool {
	if s == nil {
		return false
	}
	return s.hasActive
}

func (s *stubGateCoordinator) HasDeferredGate(_ *WorkflowRun) bool {
	if s == nil {
		return false
	}
	return s.hasDeferred
}

func (s *stubGateCoordinator) ResolvePendingGate(ctx context.Context, _ *WorkflowRun) (GateResolution, error) {
	if s != nil {
		s.pendingCtx = ctx
	}
	if s == nil {
		return GateResolution{}, nil
	}
	return s.pendingResolution, s.pendingErr
}

func (s *stubGateCoordinator) ResolveSignal(ctx context.Context, _ *WorkflowRun, _ GateSignal) (GateResolution, error) {
	if s != nil {
		s.signalCtx = ctx
	}
	if s == nil {
		return GateResolution{}, nil
	}
	return s.signalResolution, s.signalErr
}

func (s *stubGateSignalMatcher) Matches(_ *WorkflowGateRun, _ GateSignal) bool {
	if s == nil {
		return true
	}
	s.calls++
	return s.matches
}

func (s *stubGateSignalAdapter) FromTurnSignal(signal TurnSignal) GateSignal {
	if s == nil {
		return GateSignal{}
	}
	s.calls++
	s.last = signal
	return s.signal
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
	if s.started != nil {
		s.startOnce.Do(func() {
			close(s.started)
		})
	}
	if s.release != nil {
		<-s.release
	}
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

func (s *stubStepPromptDispatcher) DispatchGate(ctx context.Context, req GateDispatchRequest) (GateDispatchResult, error) {
	stepResult, err := s.DispatchStepPrompt(ctx, StepPromptDispatchRequest{
		RunID:                  req.RunID,
		TemplateID:             req.TemplateID,
		DefaultAccessLevel:     req.DefaultAccessLevel,
		SelectedProvider:       req.SelectedProvider,
		SelectedRuntimeOptions: types.CloneRuntimeOptions(req.SelectedRuntimeOptions),
		WorkspaceID:            req.WorkspaceID,
		WorktreeID:             req.WorktreeID,
		SessionID:              req.SessionID,
		PhaseID:                req.PhaseID,
		GateID:                 req.GateID,
		GateKind:               req.GateKind,
		Boundary:               req.Boundary,
		Prompt:                 req.Prompt,
	})
	if err != nil {
		return GateDispatchResult{}, err
	}
	return GateDispatchResult{
		Dispatched: stepResult.Dispatched,
		Transport:  "session_turn",
		SessionID:  stepResult.SessionID,
		SignalID:   stepResult.TurnID,
		Provider:   stepResult.Provider,
		Model:      stepResult.Model,
	}, nil
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
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.savedByRunID == nil {
		s.savedByRunID = map[string]RunStatusSnapshot{}
	}
	runID := strings.TrimSpace(snapshot.Run.ID)
	snapshot.Run.ID = runID
	s.savedByRunID[runID] = cloneRunSnapshotForTest(snapshot)
	return nil
}

func (s *stubRunSnapshotStore) SavedCount() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.savedByRunID)
}

func (s *stubRunSnapshotStore) SavedByRunID(runID string) (RunStatusSnapshot, bool) {
	if s == nil {
		return RunStatusSnapshot{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, ok := s.savedByRunID[strings.TrimSpace(runID)]
	if !ok {
		return RunStatusSnapshot{}, false
	}
	return cloneRunSnapshotForTest(snapshot), true
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

func TestRunLifecycleListTemplatesFailsFastWhenExplicitConfigIsInvalid(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			err:            errors.New("invalid workflow templates file"),
			explicitConfig: true,
		}),
	)

	_, err := service.ListTemplates(context.Background())
	if !errors.Is(err, ErrTemplateConfigInvalid) {
		t.Fatalf("expected ErrTemplateConfigInvalid, got %v", err)
	}
}

func TestRunLifecycleCreateRunFailsFastWhenExplicitConfigIsInvalid(t *testing.T) {
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			err:            errors.New("invalid workflow templates file"),
			explicitConfig: true,
		}),
	)

	_, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if !errors.Is(err, ErrTemplateConfigInvalid) {
		t.Fatalf("expected ErrTemplateConfigInvalid, got %v", err)
	}
}

func TestRunLifecycleListTemplatesPreservesUnderlyingExplicitConfigError(t *testing.T) {
	rootErr := errors.New("template provider parse failed")
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			err:            rootErr,
			explicitConfig: true,
		}),
	)

	_, err := service.ListTemplates(context.Background())
	if !errors.Is(err, ErrTemplateConfigInvalid) {
		t.Fatalf("expected ErrTemplateConfigInvalid, got %v", err)
	}
	if !errors.Is(err, rootErr) {
		t.Fatalf("expected wrapped root error, got %v", err)
	}
}

func TestRunLifecycleCreateRunPreservesUnderlyingExplicitConfigError(t *testing.T) {
	rootErr := errors.New("template provider parse failed")
	service := NewRunService(
		Config{Enabled: true},
		WithTemplateProvider(&stubTemplateProvider{
			err:            rootErr,
			explicitConfig: true,
		}),
	)

	_, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if !errors.Is(err, ErrTemplateConfigInvalid) {
		t.Fatalf("expected ErrTemplateConfigInvalid, got %v", err)
	}
	if !errors.Is(err, rootErr) {
		t.Fatalf("expected wrapped root error, got %v", err)
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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

func TestRunLifecyclePhaseEndLLMJudgePassesAndAdvancesToNextPhase(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_pass",
		Name: "Phase Gate Pass",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{
							Prompt: "Judge whether phase 1 succeeded.",
						},
					},
				},
			},
			{
				ID:   "phase_2",
				Name: "Phase 2",
				Steps: []WorkflowTemplateStep{
					{ID: "step_2", Name: "step 2", Prompt: "prompt 2"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-gate-1"},
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-2"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
		Output:    "implemented phase 1",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted step 1: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after phase 1 step, got %d", len(updated))
	}
	current := updated[0]
	if len(current.Phases[0].Gates) != 1 || current.Phases[0].Gates[0].Status != WorkflowGateStatusAwaitingSignal {
		t.Fatalf("expected llm_judge gate to await turn, got %#v", current.Phases[0].Gates)
	}
	if got := dispatcher.calls[1].GateID; got != "gate_1" {
		t.Fatalf("expected gate_id gate_1, got %q", got)
	}

	updated, err = service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-gate-1",
		Status:    "completed",
		Terminal:  true,
		Output:    `{"passed":true,"reason":"phase looks good"}`,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted judge: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after judge, got %d", len(updated))
	}
	current = updated[0]
	if current.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected run to keep running after passing gate, got %q", current.Status)
	}
	if len(current.Phases[0].Gates) != 1 || current.Phases[0].Gates[0].Status != WorkflowGateStatusPassed {
		t.Fatalf("expected passed phase gate, got %#v", current.Phases[0].Gates)
	}
	if current.Phases[1].Steps[0].Status != StepRunStatusRunning || !current.Phases[1].Steps[0].AwaitingTurn {
		t.Fatalf("expected next phase step to dispatch after gate pass, got %#v", current.Phases[1].Steps[0])
	}
}

func TestRunLifecyclePhaseEndLLMJudgeFailurePausesRun(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_fail",
		Name: "Phase Gate Fail",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{
							Prompt: "Judge whether phase 1 succeeded.",
						},
					},
				},
			},
			{
				ID:   "phase_2",
				Name: "Phase 2",
				Steps: []WorkflowTemplateStep{
					{ID: "step_2", Name: "step 2", Prompt: "prompt 2"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-gate-1"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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
	if _, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
		Output:    "implemented phase 1",
	}); err != nil {
		t.Fatalf("OnTurnCompleted step 1: %v", err)
	}

	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-gate-1",
		Status:    "completed",
		Terminal:  true,
		Output:    `{"passed":false,"reason":"missing tests and validation"}`,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted judge: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after judge failure, got %d", len(updated))
	}
	current := updated[0]
	if current.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused run after judge failure, got %q", current.Status)
	}
	if current.LatestDecision == nil {
		t.Fatalf("expected latest decision after gate failure")
	}
	if current.LatestDecision.Source != "gate" {
		t.Fatalf("expected gate decision source, got %q", current.LatestDecision.Source)
	}
	if current.LatestDecision.Metadata.Action != CheckpointActionPause {
		t.Fatalf("expected pause decision action, got %q", current.LatestDecision.Metadata.Action)
	}
	if len(current.LatestDecision.Metadata.Reasons) != 1 || current.LatestDecision.Metadata.Reasons[0].Code != reasonGateLLMJudgeFailed {
		t.Fatalf("expected gate failure reason, got %#v", current.LatestDecision.Metadata.Reasons)
	}
	if current.LatestDecision.GateID != "gate_1" {
		t.Fatalf("expected gate_id in decision, got %#v", current.LatestDecision)
	}
	if current.LatestDecision.StepID != "" {
		t.Fatalf("expected empty step_id for gate decision, got %q", current.LatestDecision.StepID)
	}
	if len(current.Phases[0].Gates) != 1 || current.Phases[0].Gates[0].Status != WorkflowGateStatusFailed {
		t.Fatalf("expected failed phase gate state, got %#v", current.Phases[0].Gates)
	}
	if current.Phases[1].Steps[0].Status != StepRunStatusPending {
		t.Fatalf("expected next phase not to dispatch after gate failure, got %#v", current.Phases[1].Steps[0])
	}
}

func TestRunLifecyclePhaseEndManualReviewPausesImmediately(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_manual",
		Name: "Phase Gate Manual",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "manual_1",
						Kind: WorkflowGateKindManualReview,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						ManualReviewConfig: &ManualReviewConfig{
							Reason: "manual review required",
						},
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
		Output:    "phase complete",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after manual gate pause, got %d", len(updated))
	}
	current := updated[0]
	if current.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused run, got %q", current.Status)
	}
	if current.LatestDecision == nil {
		t.Fatalf("expected latest decision")
	}
	if current.LatestDecision.GateID != "manual_1" {
		t.Fatalf("expected gate_id manual_1, got %#v", current.LatestDecision)
	}
	if len(current.LatestDecision.Metadata.Reasons) != 1 || current.LatestDecision.Metadata.Reasons[0].Code != reasonGateManualReviewRequired {
		t.Fatalf("expected manual review reason code, got %#v", current.LatestDecision.Metadata.Reasons)
	}
	if len(dispatcher.calls) != 1 {
		t.Fatalf("expected only step dispatch call, got %d", len(dispatcher.calls))
	}
}

func TestRunLifecyclePhaseEndLLMJudgeInvalidOutputPausesRun(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_invalid",
		Name: "Phase Gate Invalid Output",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-gate-1"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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
	if _, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
	}); err != nil {
		t.Fatalf("OnTurnCompleted step: %v", err)
	}
	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-gate-1",
		Status:    "completed",
		Terminal:  true,
		Output:    "phase passed",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted gate: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after invalid gate output, got %d", len(updated))
	}
	current := updated[0]
	if current.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused run, got %q", current.Status)
	}
	if current.LatestDecision == nil || len(current.LatestDecision.Metadata.Reasons) != 1 {
		t.Fatalf("expected pause reason, got %#v", current.LatestDecision)
	}
	if current.LatestDecision.Metadata.Reasons[0].Code != reasonGateLLMJudgeInvalidOutput {
		t.Fatalf("expected invalid output reason code, got %#v", current.LatestDecision.Metadata.Reasons)
	}
}

func TestRunLifecyclePhaseEndLLMJudgeIgnoresMismatchedSignalBeforeAcceptingCorrectSignal(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_signal_order",
		Name: "Phase Gate Signal Order",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
			{
				ID:   "phase_2",
				Name: "Phase 2",
				Steps: []WorkflowTemplateStep{
					{ID: "step_2", Name: "step 2", Prompt: "prompt 2"},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-gate-1"},
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-2"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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
	if _, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
	}); err != nil {
		t.Fatalf("OnTurnCompleted step: %v", err)
	}
	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-wrong",
		Status:    "completed",
		Terminal:  true,
		Output:    `{"passed":true,"reason":"ignore me"}`,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted mismatched: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after mismatch, got %d", len(updated))
	}
	current := updated[0]
	if current.Phases[0].Gates[0].Status != WorkflowGateStatusAwaitingSignal {
		t.Fatalf("expected gate to keep awaiting turn after mismatch, got %#v", current.Phases[0].Gates[0])
	}
	updated, err = service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-gate-1",
		Status:    "completed",
		Terminal:  true,
		Output:    `{"passed":true,"reason":"looks good"}`,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted gate: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after valid gate turn, got %d", len(updated))
	}
	current = updated[0]
	if current.Phases[0].Gates[0].Status != WorkflowGateStatusPassed {
		t.Fatalf("expected passed gate after valid signal, got %#v", current.Phases[0].Gates[0])
	}
	if current.Phases[1].Steps[0].Status != StepRunStatusRunning {
		t.Fatalf("expected next step running, got %#v", current.Phases[1].Steps[0])
	}
	updated, err = service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-gate-1",
		Status:    "completed",
		Terminal:  true,
		Output:    `{"passed":false,"reason":"duplicate should be ignored"}`,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted duplicate: %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("expected duplicate gate signal to be ignored, got %d updates", len(updated))
	}
	current, err = service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Phases[0].Gates[0].Status != WorkflowGateStatusPassed {
		t.Fatalf("expected duplicate signal to keep gate passed, got %#v", current.Phases[0].Gates[0])
	}
}

func TestRunLifecyclePhaseEndGateDispatchDeferredUsesScheduler(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_dispatch_deferred",
		Name: "Phase Gate Dispatch Deferred",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
		},
		errs: []error{
			nil,
			errors.New("temporary gate dispatch contention"),
		},
	}
	scheduler := &stubDispatchRetryScheduler{}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
		WithDispatchErrorClassifier(stubDispatchErrorClassifier{disposition: DispatchErrorDispositionDeferred}),
		WithDispatchRetryScheduler(scheduler),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
		Output:    "phase complete",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run, got %d", len(updated))
	}
	current := updated[0]
	if current.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected run to remain running after deferred gate dispatch, got %q", current.Status)
	}
	gate := current.Phases[0].Gates[0]
	if gate.Status != WorkflowGateStatusWaitingDispatch || gate.ExecutionState != GateExecutionStateDeferred || gate.Outcome != "waiting_dispatch" {
		t.Fatalf("expected waiting dispatch deferred gate state, got %#v", gate)
	}
	if len(scheduler.enqueued) != 1 || scheduler.enqueued[0] != run.ID {
		t.Fatalf("expected scheduler enqueue once for deferred gate dispatch, got %#v", scheduler.enqueued)
	}
}

func TestRunLifecyclePhaseEndGateDispatchFatalFailsRun(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_dispatch_fatal",
		Name: "Phase Gate Dispatch Fatal",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
		},
		errs: []error{
			nil,
			errors.New("gate transport failed"),
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
		WithDispatchErrorClassifier(stubDispatchErrorClassifier{disposition: DispatchErrorDispositionFatal}),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "gate transport failed") {
		t.Fatalf("expected gate dispatch fatal error, got %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("expected no updated runs when gate dispatch fatally fails, got %d", len(updated))
	}
	current, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Status != WorkflowRunStatusFailed || current.Phases[0].Status != PhaseRunStatusFailed {
		t.Fatalf("expected failed run/phase after gate dispatch fatal error, got run=%q phase=%q", current.Status, current.Phases[0].Status)
	}
	gate := current.Phases[0].Gates[0]
	if gate.Status != WorkflowGateStatusFailed || gate.ExecutionState != GateExecutionStateUnavailable {
		t.Fatalf("expected failed gate execution state unavailable, got %#v", gate)
	}
	if !strings.Contains(strings.ToLower(current.LastError), "gate transport failed") {
		t.Fatalf("expected run last error to include gate dispatch failure, got %q", current.LastError)
	}
}

func TestRunLifecyclePhaseEndGateDispatchUnavailableFailsRun(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_dispatch_unavailable",
		Name: "Phase Gate Dispatch Unavailable",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
			{Dispatched: false},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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

	if _, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
	}); err == nil || !errors.Is(err, ErrGateDispatch) {
		t.Fatalf("expected ErrGateDispatch for undispatched gate prompt, got %v", err)
	}
	current, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run after undispatched gate prompt, got %q", current.Status)
	}
	if !strings.Contains(strings.ToLower(current.LastError), "did not dispatch gate prompt") {
		t.Fatalf("expected gate dispatch unavailable detail in LastError, got %q", current.LastError)
	}
}

func TestRunLifecyclePhaseEndGateDispatchMissingSessionIDUsesSignalID(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_dispatch_missing_session",
		Name: "Phase Gate Dispatch Missing Session",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
			{Dispatched: true, TurnID: "turn-gate-1"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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

	if _, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
	}); err != nil {
		t.Fatalf("expected gate dispatch without session id to succeed when signal id is present, got %v", err)
	}
	current, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running run after gate dispatch with signal id, got %q", current.Status)
	}
	gate := current.Phases[0].Gates[0]
	if gate.Status != WorkflowGateStatusAwaitingSignal {
		t.Fatalf("expected gate awaiting signal after dispatch, got %q", gate.Status)
	}
	if gate.SignalID != "turn-gate-1" {
		t.Fatalf("expected gate signal id to be retained, got %q", gate.SignalID)
	}
}

func TestRunLifecyclePhaseEndGateDispatchMissingRoutingFailsRun(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_dispatch_missing_routing",
		Name: "Phase Gate Dispatch Missing Routing",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
			{Dispatched: true},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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

	if _, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
	}); err == nil || !errors.Is(err, ErrGateDispatch) {
		t.Fatalf("expected ErrGateDispatch for gate dispatch without routing identifiers, got %v", err)
	}
	current, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed run after gate dispatch missing routing identifiers, got %q", current.Status)
	}
	if !strings.Contains(strings.ToLower(current.LastError), "requires session_id or signal_id") {
		t.Fatalf("expected missing routing detail in LastError, got %q", current.LastError)
	}
}

func TestRunLifecycleOnGateSignalCompletesAwaitingLLMJudgeGate(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "phase_gate_signal_ingress",
		Name: "Phase Gate Signal Ingress",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "step 1", Prompt: "prompt 1"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
		},
	}
	dispatcher := &stubStepPromptDispatcher{
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-step-1"},
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-gate-1"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
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
	if _, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-step-1",
		Status:    "completed",
		Terminal:  true,
	}); err != nil {
		t.Fatalf("OnTurnCompleted step: %v", err)
	}

	updated, err := service.OnGateSignal(context.Background(), GateSignal{
		Transport: "session_turn",
		SignalID:  "turn-gate-1",
		SessionID: "sess-1",
		Status:    "completed",
		Terminal:  true,
		Output:    `{"passed":true,"reason":"ok"}`,
	})
	if err != nil {
		t.Fatalf("OnGateSignal: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run from gate signal, got %d", len(updated))
	}
	if updated[0].Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected run to complete after gate signal pass, got %q", updated[0].Status)
	}
}

func TestRunLifecycleOnGateSignalNoopWithoutIdentifiers(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	updated, err := service.OnGateSignal(context.Background(), GateSignal{
		Status: "completed",
		Output: `{"passed":true,"reason":"ok"}`,
	})
	if err != nil {
		t.Fatalf("OnGateSignal: %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("expected no updated runs for signal without identifiers, got %d", len(updated))
	}
}

func TestRunLifecycleOnGateSignalFiltersByWorkspace(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run := &WorkflowRun{
		ID:          "run-workspace-filter",
		Status:      WorkflowRunStatusRunning,
		WorkspaceID: "ws-a",
		WorktreeID:  "wt-a",
		SessionID:   "sess-a",
		Phases: []PhaseRun{
			{
				ID: "phase-1",
				Gates: []WorkflowGateRun{
					{
						ID:       "gate-1",
						Kind:     WorkflowGateKindLLMJudge,
						Status:   WorkflowGateStatusAwaitingSignal,
						SignalID: "sig-a",
						Boundary: WorkflowGateBoundaryRef{Boundary: WorkflowGateBoundaryPhaseEnd, PhaseID: "phase-1"},
						LLMJudgeConfig: &LLMJudgeConfig{
							Prompt: "judge",
						},
					},
				},
			},
		},
	}
	service.mu.Lock()
	service.setRunLocked(run.ID, run)
	service.mu.Unlock()

	updated, err := service.OnGateSignal(context.Background(), GateSignal{
		WorkspaceID: "ws-b",
		SessionID:   "sess-a",
		SignalID:    "sig-a",
		Status:      "completed",
		Terminal:    true,
		Output:      `{"passed":true,"reason":"ok"}`,
	})
	if err != nil {
		t.Fatalf("OnGateSignal: %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("expected no updated runs for workspace mismatch, got %d", len(updated))
	}
}

func TestRunLifecycleOnGateSignalPropagatesCoordinatorError(t *testing.T) {
	coordinator := &stubGateCoordinator{signalErr: errors.New("gate coordinator failure")}
	service := NewRunService(Config{Enabled: true}, WithGateCoordinator(coordinator))
	run := &WorkflowRun{
		ID:        "run-gate-error",
		Status:    WorkflowRunStatusRunning,
		SessionID: "sess-1",
		Phases: []PhaseRun{
			{
				ID: "phase-1",
				Gates: []WorkflowGateRun{
					{
						ID:       "gate-1",
						Kind:     WorkflowGateKindLLMJudge,
						Status:   WorkflowGateStatusAwaitingSignal,
						Boundary: WorkflowGateBoundaryRef{Boundary: WorkflowGateBoundaryPhaseEnd, PhaseID: "phase-1"},
					},
				},
			},
		},
	}
	service.mu.Lock()
	service.setRunLocked(run.ID, run)
	service.mu.Unlock()

	if _, err := service.OnGateSignal(context.Background(), GateSignal{SessionID: "sess-1"}); err == nil ||
		!strings.Contains(strings.ToLower(err.Error()), "gate coordinator failure") {
		t.Fatalf("expected coordinator error to propagate, got %v", err)
	}
}

func TestRunLifecycleProcessGateSignalDeduplicatesSeenSignalID(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run := &WorkflowRun{
		ID:        "run-gate-dedupe",
		Status:    WorkflowRunStatusRunning,
		SessionID: "sess-1",
		Phases: []PhaseRun{
			{
				ID: "phase-1",
				Gates: []WorkflowGateRun{
					{
						ID:       "gate-1",
						Kind:     WorkflowGateKindLLMJudge,
						Status:   WorkflowGateStatusAwaitingSignal,
						SignalID: "sig-1",
						Boundary: WorkflowGateBoundaryRef{Boundary: WorkflowGateBoundaryPhaseEnd, PhaseID: "phase-1"},
						LLMJudgeConfig: &LLMJudgeConfig{
							Prompt: "judge",
						},
					},
				},
			},
		},
	}
	service.mu.Lock()
	service.setRunLocked(run.ID, run)
	service.turnSeen[turnReceiptKey(run.ID, "sig-1")] = struct{}{}
	service.mu.Unlock()

	updated, err := service.processGateSignalForRun(context.Background(), run.ID, GateSignal{
		SessionID: "sess-1",
		SignalID:  "sig-1",
		Status:    "completed",
		Terminal:  true,
		Output:    `{"passed":true,"reason":"ok"}`,
	})
	if err != nil {
		t.Fatalf("processGateSignalForRun: %v", err)
	}
	if updated != nil {
		t.Fatalf("expected duplicate signal to be ignored without run update, got %#v", updated)
	}
}

func TestRunLifecycleOnGateSignalReturnsQueueErrorWhenQueueUnavailable(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run := &WorkflowRun{
		ID:        "run-gate-queue",
		Status:    WorkflowRunStatusRunning,
		SessionID: "sess-1",
		Phases: []PhaseRun{
			{
				ID:     "phase-1",
				Status: PhaseRunStatusCompleted,
				Gates: []WorkflowGateRun{
					{
						ID:       "gate-1",
						Kind:     WorkflowGateKindLLMJudge,
						Status:   WorkflowGateStatusAwaitingSignal,
						SignalID: "sig-1",
						Boundary: WorkflowGateBoundaryRef{Boundary: WorkflowGateBoundaryPhaseEnd, PhaseID: "phase-1"},
						LLMJudgeConfig: &LLMJudgeConfig{
							Prompt: "judge",
						},
					},
				},
			},
		},
	}
	service.mu.Lock()
	service.setRunLocked(run.ID, run)
	queue := NewChannelDispatchQueue(1, 1, func(DispatchRequest) DispatchQueueResult {
		return DispatchQueueResult{Done: true}
	})
	queue.Close()
	service.dispatchQueue = queue
	service.mu.Unlock()

	_, err := service.OnGateSignal(context.Background(), GateSignal{
		SessionID: "sess-1",
		SignalID:  "sig-1",
		Status:    "completed",
		Terminal:  true,
		Output:    `{"passed":true,"reason":"ok"}`,
	})
	if err == nil || !errors.Is(err, ErrStepDispatch) {
		t.Fatalf("expected queue unavailable error to surface as ErrStepDispatch, got %v", err)
	}
}

func TestRunLifecycleWithGateSignalMatcherBlocksGateSignalProgression(t *testing.T) {
	matcher := &stubGateSignalMatcher{matches: false}
	service := NewRunService(Config{Enabled: true}, WithGateSignalMatcher(matcher))
	run := &WorkflowRun{
		ID:        "run-gate-matcher",
		Status:    WorkflowRunStatusRunning,
		SessionID: "sess-1",
		Phases: []PhaseRun{
			{
				ID: "phase-1",
				Gates: []WorkflowGateRun{
					{
						ID:       "gate-1",
						Kind:     WorkflowGateKindLLMJudge,
						Status:   WorkflowGateStatusAwaitingSignal,
						SignalID: "sig-1",
						Boundary: WorkflowGateBoundaryRef{Boundary: WorkflowGateBoundaryPhaseEnd, PhaseID: "phase-1"},
						LLMJudgeConfig: &LLMJudgeConfig{
							Prompt: "judge",
						},
					},
				},
			},
		},
	}
	service.mu.Lock()
	service.setRunLocked(run.ID, run)
	service.mu.Unlock()

	updated, err := service.OnGateSignal(context.Background(), GateSignal{
		SessionID: "sess-1",
		SignalID:  "sig-1",
		Status:    "completed",
		Terminal:  true,
		Output:    `{"passed":true,"reason":"ok"}`,
	})
	if err != nil {
		t.Fatalf("OnGateSignal: %v", err)
	}
	if matcher.calls == 0 {
		t.Fatalf("expected custom gate signal matcher to be called")
	}
	if len(updated) != 1 {
		t.Fatalf("expected run snapshot to be returned even when gate signal is ignored, got %d", len(updated))
	}
	if updated[0].Phases[0].Gates[0].Status != WorkflowGateStatusAwaitingSignal {
		t.Fatalf("expected gate to remain awaiting signal when matcher blocks, got %q", updated[0].Phases[0].Gates[0].Status)
	}
}

func TestRunLifecycleWithGateSignalAdapterUsedForTurnCompleted(t *testing.T) {
	adapter := &stubGateSignalAdapter{
		signal: GateSignal{
			Transport: "session_turn",
			SessionID: "sess-1",
			SignalID:  "sig-1",
			Status:    "completed",
			Terminal:  true,
			Output:    `{"passed":true,"reason":"ok"}`,
		},
	}
	service := NewRunService(Config{Enabled: true}, WithGateSignalAdapter(adapter))
	run := &WorkflowRun{
		ID:        "run-gate-adapter",
		Status:    WorkflowRunStatusRunning,
		SessionID: "sess-1",
		Phases: []PhaseRun{
			{
				ID:     "phase-1",
				Status: PhaseRunStatusCompleted,
				Gates: []WorkflowGateRun{
					{
						ID:       "gate-1",
						Kind:     WorkflowGateKindLLMJudge,
						Status:   WorkflowGateStatusAwaitingSignal,
						SignalID: "sig-1",
						Boundary: WorkflowGateBoundaryRef{Boundary: WorkflowGateBoundaryPhaseEnd, PhaseID: "phase-1"},
						LLMJudgeConfig: &LLMJudgeConfig{
							Prompt: "judge",
						},
					},
				},
			},
		},
	}
	service.mu.Lock()
	service.setRunLocked(run.ID, run)
	service.mu.Unlock()

	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-1",
		TurnID:    "turn-source",
		Status:    "completed",
		Terminal:  true,
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	if adapter.calls == 0 {
		t.Fatalf("expected custom gate signal adapter to be invoked")
	}
	if adapter.last.TurnID != "turn-source" {
		t.Fatalf("expected adapter to receive turn signal context, got %#v", adapter.last)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run, got %d", len(updated))
	}
}

func TestRunLifecycleStepDispatcherDoesNotImplicitlySetGateDispatcher(t *testing.T) {
	dispatcher := &stubStepPromptDispatcher{}
	service := NewRunService(Config{Enabled: true}, WithStepPromptDispatcher(dispatcher))
	if service.gateDispatcher != nil {
		t.Fatalf("expected gate dispatcher to remain nil without explicit WithGateDispatcher wiring")
	}
}

func TestValidateGateDispatchResult(t *testing.T) {
	tests := []struct {
		name    string
		in      GateDispatchResult
		wantErr bool
	}{
		{
			name:    "not dispatched",
			in:      GateDispatchResult{Dispatched: false},
			wantErr: true,
		},
		{
			name: "session turn with session id",
			in: GateDispatchResult{
				Dispatched: true,
				Transport:  "session_turn",
				SessionID:  "sess-1",
			},
			wantErr: false,
		},
		{
			name: "session turn with signal id",
			in: GateDispatchResult{
				Dispatched: true,
				Transport:  "session_turn",
				SignalID:   "sig-1",
			},
			wantErr: false,
		},
		{
			name: "session turn without routing",
			in: GateDispatchResult{
				Dispatched: true,
				Transport:  "session_turn",
			},
			wantErr: true,
		},
		{
			name: "non session transport with signal",
			in: GateDispatchResult{
				Dispatched: true,
				Transport:  "webhook",
				SignalID:   "sig-1",
			},
			wantErr: false,
		},
		{
			name: "non session transport without routing",
			in: GateDispatchResult{
				Dispatched: true,
				Transport:  "webhook",
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGateDispatchResult(tc.in)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestPrepareGateDispatchContextFailsClosedOnEmptyPrompt(t *testing.T) {
	run := &WorkflowRun{
		ID:     "run-gate-prompt-empty",
		Status: WorkflowRunStatusRunning,
		Phases: []PhaseRun{
			{
				ID:     "phase-1",
				Status: PhaseRunStatusCompleted,
				Gates: []WorkflowGateRun{
					{
						ID:     "gate-1",
						Kind:   WorkflowGateKindLLMJudge,
						Status: WorkflowGateStatusPending,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase-1",
						},
					},
				},
			},
		},
	}
	coordinator := &stubGateCoordinator{
		pendingResolution: GateResolution{
			Consumed:   true,
			PhaseIndex: 0,
			GateIndex:  0,
			GateID:     "gate-1",
			GateKind:   WorkflowGateKindLLMJudge,
			Boundary:   WorkflowGateBoundaryPhaseEnd,
			Outcome:    GateOutcomeAwaiting,
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithGateCoordinator(coordinator),
	)
	service.mu.Lock()
	_, hasDispatch, err := service.prepareGateDispatchContext(context.Background(), run)
	service.mu.Unlock()
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "gate dispatch prompt is empty") {
		t.Fatalf("expected empty gate prompt failure, got %v", err)
	}
	if hasDispatch {
		t.Fatalf("expected no dispatch context when gate prompt is empty")
	}
	if run.Status != WorkflowRunStatusFailed || run.Phases[0].Gates[0].Status != WorkflowGateStatusFailed {
		t.Fatalf("expected fail-closed run/gate state, got run=%q gate=%q", run.Status, run.Phases[0].Gates[0].Status)
	}
}

func TestPrepareGateDispatchContextUsesCallerContext(t *testing.T) {
	coordinator := &stubGateCoordinator{
		pendingResolution: GateResolution{},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithGateCoordinator(coordinator),
	)
	run := &WorkflowRun{
		ID:     "run-ctx",
		Status: WorkflowRunStatusRunning,
	}
	ctx := context.WithValue(context.Background(), "ctx_key", "ctx_value")

	service.mu.Lock()
	_, hasDispatch, err := service.prepareGateDispatchContext(ctx, run)
	service.mu.Unlock()

	if err != nil {
		t.Fatalf("prepareGateDispatchContext: %v", err)
	}
	if hasDispatch {
		t.Fatalf("expected no gate dispatch from stub coordinator")
	}
	if coordinator.pendingCtx == nil {
		t.Fatalf("expected pending gate resolution context to be captured")
	}
	if got := coordinator.pendingCtx.Value("ctx_key"); got != "ctx_value" {
		t.Fatalf("expected context value propagation, got %#v", got)
	}
}

func TestPrepareGateDispatchContextHonorsCancellation(t *testing.T) {
	coordinator := &stubGateCoordinator{
		pendingResolution: GateResolution{Consumed: true},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithGateCoordinator(coordinator),
	)
	run := &WorkflowRun{
		ID:     "run-cancel",
		Status: WorkflowRunStatusRunning,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service.mu.Lock()
	_, _, err := service.prepareGateDispatchContext(ctx, run)
	service.mu.Unlock()

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
	if coordinator.pendingCtx != nil {
		t.Fatalf("expected coordinator not to be called after cancellation")
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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

func TestRunLifecycleOnTurnCompletedTerminalFailureBeforeAwaitingTurnFailsAfterDispatchLinksTurn(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "prompted_turn_failure_before_awaiting",
		Name: "Prompted Turn Failure Before Awaiting",
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
	dispatchStarted := make(chan struct{})
	dispatchRelease := make(chan struct{})
	dispatcher := &stubStepPromptDispatcher{
		started: dispatchStarted,
		release: dispatchRelease,
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-1", TurnID: "turn-early"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "prompted_turn_failure_before_awaiting",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	startDone := make(chan error, 1)
	go func() {
		_, startErr := service.StartRun(context.Background(), run.ID)
		startDone <- startErr
	}()
	select {
	case <-dispatchStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch to start")
	}

	type turnResult struct {
		updated []*WorkflowRun
		err     error
	}
	turnDone := make(chan turnResult, 1)
	go func() {
		updated, turnErr := service.OnTurnCompleted(context.Background(), TurnSignal{
			SessionID: "sess-1",
			TurnID:    "turn-early",
			Status:    "failed",
			Error:     "model unsupported",
			Terminal:  true,
		})
		turnDone <- turnResult{updated: updated, err: turnErr}
	}()

	close(dispatchRelease)

	select {
	case startErr := <-startDone:
		if startErr != nil {
			t.Fatalf("StartRun: %v", startErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StartRun to finish")
	}

	var result turnResult
	select {
	case result = <-turnDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnTurnCompleted to finish")
	}
	if result.err != nil {
		t.Fatalf("OnTurnCompleted: %v", result.err)
	}
	if len(result.updated) != 1 {
		t.Fatalf("expected one updated run, got %d", len(result.updated))
	}
	if result.updated[0].Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed status from early terminal signal, got %q", result.updated[0].Status)
	}
	if !strings.Contains(strings.ToLower(result.updated[0].LastError), "model unsupported") {
		t.Fatalf("expected run last error to include terminal failure detail, got %q", result.updated[0].LastError)
	}

	current, err := service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if current.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected persisted run status failed, got %q", current.Status)
	}
	step := current.Phases[0].Steps[0]
	if step.Status != StepRunStatusFailed {
		t.Fatalf("expected failed step status, got %q", step.Status)
	}
	if !strings.Contains(strings.ToLower(step.Error), "model unsupported") {
		t.Fatalf("expected step error to include terminal failure detail, got %q", step.Error)
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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

func TestRunLifecycleRenameRunDoesNotBlockDuringStartDispatch(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "rename_during_dispatch_template",
		Name: "Rename During Dispatch",
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
	dispatchStarted := make(chan struct{})
	dispatchRelease := make(chan struct{})
	dispatcher := &stubStepPromptDispatcher{
		started: dispatchStarted,
		release: dispatchRelease,
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-rename", TurnID: "turn-rename"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "rename_during_dispatch_template",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	startDone := make(chan error, 1)
	go func() {
		_, startErr := service.StartRun(context.Background(), run.ID)
		startDone <- startErr
	}()

	select {
	case <-dispatchStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch to start")
	}

	renameDone := make(chan struct{})
	var renamed *WorkflowRun
	var renameErr error
	go func() {
		renamed, renameErr = service.RenameRun(context.Background(), run.ID, "Renamed During Dispatch")
		close(renameDone)
	}()
	select {
	case <-renameDone:
		if renameErr != nil {
			t.Fatalf("RenameRun: %v", renameErr)
		}
		if strings.TrimSpace(renamed.TemplateName) != "Renamed During Dispatch" {
			t.Fatalf("expected renamed workflow name, got %q", renamed.TemplateName)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("RenameRun blocked while dispatch was in-flight")
	}

	close(dispatchRelease)
	select {
	case startErr := <-startDone:
		if startErr != nil {
			t.Fatalf("StartRun: %v", startErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StartRun to finish")
	}
}

func TestRunLifecycleAdvanceRunReturnsRunNotFoundWhenRunRemovedDuringDispatch(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "advance_removed_during_dispatch_template",
		Name: "Advance Removed During Dispatch",
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
	dispatchStarted := make(chan struct{})
	dispatchRelease := make(chan struct{})
	dispatcher := &stubStepPromptDispatcher{
		started: dispatchStarted,
		release: dispatchRelease,
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-advance-removed", TurnID: "turn-advance-removed"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "advance_removed_during_dispatch_template",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	service.mu.Lock()
	current, ok := service.runs[run.ID]
	if !ok || current == nil {
		service.mu.Unlock()
		t.Fatalf("expected run to exist")
	}
	current.Status = WorkflowRunStatusRunning
	service.mu.Unlock()

	advanceDone := make(chan error, 1)
	go func() {
		_, advanceErr := service.AdvanceRun(context.Background(), run.ID)
		advanceDone <- advanceErr
	}()

	select {
	case <-dispatchStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch to start")
	}

	releaseRunLock := service.runLocks.Lock(strings.TrimSpace(run.ID))
	service.mu.Lock()
	delete(service.runs, strings.TrimSpace(run.ID))
	delete(service.timelines, strings.TrimSpace(run.ID))
	service.mu.Unlock()
	releaseRunLock()

	close(dispatchRelease)

	select {
	case advanceErr := <-advanceDone:
		if !errors.Is(advanceErr, ErrRunNotFound) {
			t.Fatalf("expected ErrRunNotFound, got %v", advanceErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for AdvanceRun to finish")
	}
}

func TestRunLifecycleAdvanceRunSkipsStaleDispatchApplyWhenPausedDuringDispatch(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "advance_stale_dispatch_template",
		Name: "Advance Stale Dispatch",
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
	dispatchStarted := make(chan struct{})
	dispatchRelease := make(chan struct{})
	dispatcher := &stubStepPromptDispatcher{
		started: dispatchStarted,
		release: dispatchRelease,
		responses: []StepPromptDispatchResult{
			{Dispatched: true, SessionID: "sess-advance-stale", TurnID: "turn-advance-stale"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "advance_stale_dispatch_template",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	service.mu.Lock()
	current, ok := service.runs[run.ID]
	if !ok || current == nil {
		service.mu.Unlock()
		t.Fatalf("expected run to exist")
	}
	current.Status = WorkflowRunStatusRunning
	service.mu.Unlock()

	advanceDone := make(chan struct{})
	var advanced *WorkflowRun
	var advanceErr error
	go func() {
		advanced, advanceErr = service.AdvanceRun(context.Background(), run.ID)
		close(advanceDone)
	}()

	select {
	case <-dispatchStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch to start")
	}

	releaseRunLock := service.runLocks.Lock(strings.TrimSpace(run.ID))
	service.mu.Lock()
	current, ok = service.runs[run.ID]
	if !ok || current == nil {
		service.mu.Unlock()
		releaseRunLock()
		t.Fatalf("expected run to exist while pausing")
	}
	service.setRunPausedLocked(current, "run_paused_for_test", "pause during dispatch to force stale result")
	service.mu.Unlock()
	releaseRunLock()

	close(dispatchRelease)

	select {
	case <-advanceDone:
		if advanceErr != nil {
			t.Fatalf("AdvanceRun: %v", advanceErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for AdvanceRun to finish")
	}

	if advanced.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused status after stale dispatch, got %q", advanced.Status)
	}
	step := advanced.Phases[0].Steps[0]
	if step.AwaitingTurn {
		t.Fatalf("expected stale dispatch result to be ignored and awaiting_turn to remain false")
	}
	if step.Execution != nil {
		t.Fatalf("expected stale dispatch result to avoid writing execution metadata")
	}
}

func TestRunLifecycleResumeFailedRunSkipsStaleDispatchApplyWhenPausedDuringDispatch(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	runID := "gwf-resume-stale-dispatch"
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
	dispatchStarted := make(chan struct{})
	dispatchRelease := make(chan struct{})
	dispatcher := &stubStepPromptDispatcher{
		started: dispatchStarted,
		release: dispatchRelease,
		responses: []StepPromptDispatchResult{
			{
				Dispatched: true,
				SessionID:  "sess-stale",
				TurnID:     "turn-stale",
			},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithRunSnapshotStore(snapshotStore),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)

	resumeDone := make(chan struct{})
	var resumed *WorkflowRun
	var resumeErr error
	go func() {
		resumed, resumeErr = service.ResumeFailedRun(context.Background(), runID, ResumeFailedRunRequest{
			Message: "resume while dispatch is blocked",
		})
		close(resumeDone)
	}()

	select {
	case <-dispatchStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch to start")
	}

	releaseRunLock := service.runLocks.Lock(strings.TrimSpace(runID))
	service.mu.Lock()
	current, ok := service.runs[runID]
	if !ok || current == nil {
		service.mu.Unlock()
		releaseRunLock()
		t.Fatalf("expected run to exist while pausing")
	}
	service.setRunPausedLocked(current, "run_paused_for_test", "pause during resume dispatch to force stale result")
	service.mu.Unlock()
	releaseRunLock()

	close(dispatchRelease)

	select {
	case <-resumeDone:
		if resumeErr != nil {
			t.Fatalf("ResumeFailedRun: %v", resumeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ResumeFailedRun to finish")
	}

	if resumed.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused status after stale resume dispatch, got %q", resumed.Status)
	}
	step := resumed.Phases[0].Steps[0]
	if step.AwaitingTurn {
		t.Fatalf("expected stale resume dispatch result to be ignored")
	}
	if step.Execution != nil {
		t.Fatalf("expected stale resume dispatch to avoid writing execution metadata")
	}
}

func TestRunLifecycleAdvanceOnceLockedCoversSharedDispatchHandoff(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "advance_once_locked_template",
		Name: "Advance Once Locked",
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
			{Dispatched: true, SessionID: "sess-once", TurnID: "turn-once"},
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTemplate(template),
		WithStepPromptDispatcher(dispatcher),
		WithGateDispatcher(dispatcher),
	)
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "advance_once_locked_template",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	service.mu.Lock()
	current, ok := service.runs[run.ID]
	if !ok || current == nil {
		service.mu.Unlock()
		t.Fatalf("expected run to exist")
	}
	current.Status = WorkflowRunStatusRunning
	if err := service.advanceOnceLocked(context.Background(), current); err != nil {
		service.mu.Unlock()
		t.Fatalf("advanceOnceLocked: %v", err)
	}
	step := current.Phases[0].Steps[0]
	if !step.AwaitingTurn || step.Status != StepRunStatusRunning {
		service.mu.Unlock()
		t.Fatalf("expected dispatched step to be running+awaiting_turn, got %#v", step)
	}
	service.mu.Unlock()
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
	service.WaitForPendingPersists()
	if snapshotStore.SavedCount() != 1 {
		t.Fatalf("expected dismissed tombstone to be persisted, got %d", snapshotStore.SavedCount())
	}
	saved, _ := snapshotStore.SavedByRunID("gwf-missing")
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
	service.WaitForPendingPersists()
	if snapshotStore.SavedCount() == 0 {
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
	saved, ok := snapshotStore.SavedByRunID(runID)
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
		WithGateDispatcher(dispatcher),
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
		WithGateDispatcher(dispatcher),
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
