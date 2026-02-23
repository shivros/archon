package guidedworkflows

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

type RunService interface {
	RunLifecycleService
	TemplateCatalog
}

type RunLifecycleService interface {
	CreateRun(ctx context.Context, req CreateRunRequest) (*WorkflowRun, error)
	ListRuns(ctx context.Context) ([]*WorkflowRun, error)
	ListRunsIncludingDismissed(ctx context.Context) ([]*WorkflowRun, error)
	RenameRun(ctx context.Context, runID, name string) (*WorkflowRun, error)
	StartRun(ctx context.Context, runID string) (*WorkflowRun, error)
	PauseRun(ctx context.Context, runID string) (*WorkflowRun, error)
	ResumeRun(ctx context.Context, runID string) (*WorkflowRun, error)
	ResumeFailedRun(ctx context.Context, runID string, req ResumeFailedRunRequest) (*WorkflowRun, error)
	DismissRun(ctx context.Context, runID string) (*WorkflowRun, error)
	UndismissRun(ctx context.Context, runID string) (*WorkflowRun, error)
	AdvanceRun(ctx context.Context, runID string) (*WorkflowRun, error)
	HandleDecision(ctx context.Context, runID string, req DecisionActionRequest) (*WorkflowRun, error)
	GetRun(ctx context.Context, runID string) (*WorkflowRun, error)
	GetRunTimeline(ctx context.Context, runID string) ([]RunTimelineEvent, error)
}

type TemplateCatalog interface {
	ListTemplates(ctx context.Context) ([]WorkflowTemplate, error)
}

type RunMetricsProvider interface {
	GetRunMetrics(ctx context.Context) (RunMetricsSnapshot, error)
}

type RunMetricsResetter interface {
	ResetRunMetrics(ctx context.Context) (RunMetricsSnapshot, error)
}

type RunMetricsStore interface {
	LoadRunMetrics(ctx context.Context) (RunMetricsSnapshot, error)
	SaveRunMetrics(ctx context.Context, snapshot RunMetricsSnapshot) error
}

type RunSnapshotStore interface {
	ListWorkflowRuns(ctx context.Context) ([]RunStatusSnapshot, error)
	UpsertWorkflowRun(ctx context.Context, snapshot RunStatusSnapshot) error
}

type MissingRunDismissalContext struct {
	WorkspaceID string
	WorktreeID  string
	SessionID   string
}

type MissingRunContextResolver interface {
	ResolveMissingRunContext(ctx context.Context, runID string) (MissingRunDismissalContext, bool, error)
}

type MissingRunTombstoneFactory interface {
	BuildMissingRunTombstone(
		runID string,
		dismissalContext MissingRunDismissalContext,
		contextResolved bool,
		now time.Time,
	) *WorkflowRun
}

// TurnEventProcessor allows daemon adapters to advance active runs from turn completion events.
type TurnEventProcessor interface {
	OnTurnCompleted(ctx context.Context, signal TurnSignal) ([]*WorkflowRun, error)
}

type TemplateProvider interface {
	ListWorkflowTemplates(ctx context.Context) ([]WorkflowTemplate, error)
}

type TemplateConfigPresenceProvider interface {
	HasWorkflowTemplateConfig(ctx context.Context) (bool, error)
}

type InMemoryRunService struct {
	cfg                    Config
	engine                 *Engine
	templates              map[string]WorkflowTemplate
	templateProvider       TemplateProvider
	stepDispatcher         StepPromptDispatcher
	turnMatcher            TurnSignalMatcher
	dispatchClassifier     DispatchErrorClassifier
	dispatchRetryPolicy    DispatchRetryPolicy
	dispatchRetryScheduler DispatchRetryScheduler

	mu        sync.RWMutex
	runs      map[string]*WorkflowRun
	timelines map[string][]RunTimelineEvent
	turnSeen  map[string]struct{}
	actions   map[string]struct{}

	maxActiveRuns    int
	telemetryEnabled bool
	metrics          runServiceMetrics
	metricsStore     RunMetricsStore
	runStore         RunSnapshotStore
	contextResolver  MissingRunContextResolver
	tombstoneFactory MissingRunTombstoneFactory
}

type runServiceMetrics struct {
	runsStarted          int
	runsCompleted        int
	runsFailed           int
	pauseCount           int
	approvalCount        int
	approvalLatencyTotal int64
	approvalLatencyMax   int64
	interventionCauses   map[string]int
}

type RunServiceOption func(*InMemoryRunService)

func WithEngine(engine *Engine) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || engine == nil {
			return
		}
		s.engine = engine
	}
}

func WithTemplate(template WorkflowTemplate) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || strings.TrimSpace(template.ID) == "" {
			return
		}
		if s.templates == nil {
			s.templates = map[string]WorkflowTemplate{}
		}
		s.templates[template.ID] = cloneTemplate(template)
	}
}

func WithTemplateProvider(provider TemplateProvider) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || provider == nil {
			return
		}
		s.templateProvider = provider
	}
}

func WithStepPromptDispatcher(dispatcher StepPromptDispatcher) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || dispatcher == nil {
			return
		}
		s.stepDispatcher = dispatcher
	}
}

func WithTurnSignalMatcher(matcher TurnSignalMatcher) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || matcher == nil {
			return
		}
		s.turnMatcher = matcher
	}
}

func WithDispatchErrorClassifier(classifier DispatchErrorClassifier) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || classifier == nil {
			return
		}
		s.dispatchClassifier = classifier
	}
}

func WithDispatchRetryPolicy(policy DispatchRetryPolicy) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || policy == nil {
			return
		}
		s.dispatchRetryPolicy = policy
	}
}

func WithDispatchRetryScheduler(scheduler DispatchRetryScheduler) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || scheduler == nil {
			return
		}
		s.dispatchRetryScheduler = scheduler
	}
}

func WithRunExecutionControls(controls ExecutionControls) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil {
			return
		}
		if s.engine == nil {
			s.engine = NewEngine()
		}
		s.engine.controls = NormalizeExecutionControls(controls)
		if s.engine.runner == nil {
			s.engine.runner = noopExecutionRunner{}
		}
	}
}

func WithRunExecutionRunner(runner ExecutionRunner) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || runner == nil {
			return
		}
		if s.engine == nil {
			s.engine = NewEngine()
		}
		s.engine.runner = runner
	}
}

func WithMaxActiveRuns(limit int) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil {
			return
		}
		if limit <= 0 {
			s.maxActiveRuns = 0
			return
		}
		s.maxActiveRuns = limit
	}
}

func WithTelemetryEnabled(enabled bool) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil {
			return
		}
		s.telemetryEnabled = enabled
	}
}

func WithRunMetricsStore(store RunMetricsStore) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || store == nil {
			return
		}
		s.metricsStore = store
	}
}

func WithRunSnapshotStore(store RunSnapshotStore) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || store == nil {
			return
		}
		s.runStore = store
	}
}

func WithMissingRunContextResolver(resolver MissingRunContextResolver) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || resolver == nil {
			return
		}
		s.contextResolver = resolver
	}
}

func WithMissingRunTombstoneFactory(factory MissingRunTombstoneFactory) RunServiceOption {
	return func(s *InMemoryRunService) {
		if s == nil || factory == nil {
			return
		}
		s.tombstoneFactory = factory
	}
}

func NewRunService(cfg Config, opts ...RunServiceOption) *InMemoryRunService {
	service := &InMemoryRunService{
		cfg:                 NormalizeConfig(cfg),
		engine:              NewEngine(),
		templates:           BuiltinTemplates(),
		runs:                map[string]*WorkflowRun{},
		timelines:           map[string][]RunTimelineEvent{},
		turnSeen:            map[string]struct{}{},
		actions:             map[string]struct{}{},
		telemetryEnabled:    true,
		tombstoneFactory:    defaultMissingRunTombstoneFactory{},
		turnMatcher:         StrictSessionTurnSignalMatcher{},
		dispatchClassifier:  defaultDispatchErrorClassifier{},
		dispatchRetryPolicy: defaultDispatchRetryPolicy(),
		metrics: runServiceMetrics{
			interventionCauses: map[string]int{},
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	if service.dispatchClassifier == nil {
		service.dispatchClassifier = defaultDispatchErrorClassifier{}
	}
	if service.dispatchRetryPolicy == nil {
		service.dispatchRetryPolicy = defaultDispatchRetryPolicy()
	}
	if service.dispatchRetryScheduler == nil {
		service.dispatchRetryScheduler = NewDispatchRetryScheduler(service.dispatchRetryPolicy, service.retryDeferredDispatch)
	}
	service.restoreMetrics(context.Background())
	service.restoreRuns(context.Background())
	return service
}

func (s *InMemoryRunService) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	scheduler := s.dispatchRetryScheduler
	s.dispatchRetryScheduler = nil
	s.mu.Unlock()
	if scheduler != nil {
		scheduler.Close()
	}
}

func (s *InMemoryRunService) CreateRun(ctx context.Context, req CreateRunRequest) (*WorkflowRun, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: run service is nil", ErrInvalidTransition)
	}
	if !s.cfg.Enabled {
		return nil, ErrDisabled
	}
	if strings.TrimSpace(req.WorkspaceID) == "" && strings.TrimSpace(req.WorktreeID) == "" {
		return nil, ErrMissingContext
	}
	templates := s.resolveTemplates(ctx)
	templateID := strings.TrimSpace(req.TemplateID)
	if templateID == "" {
		templateID = defaultTemplateID(templates)
	}
	template, ok := templates[templateID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTemplateNotFound, templateID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maxActiveRuns > 0 && s.activeRunsLocked() >= s.maxActiveRuns {
		return nil, fmt.Errorf("%w: max_active_runs=%d", ErrRunLimitExceeded, s.maxActiveRuns)
	}
	runID := newWorkflowRunID()
	now := s.engine.now()

	run := &WorkflowRun{
		ID:           runID,
		TemplateID:   template.ID,
		TemplateName: template.Name,
		DefaultAccessLevel: func() types.AccessLevel {
			level, ok := NormalizeTemplateAccessLevel(template.DefaultAccessLevel)
			if !ok {
				return ""
			}
			return level
		}(),
		WorkspaceID:       strings.TrimSpace(req.WorkspaceID),
		WorktreeID:        strings.TrimSpace(req.WorktreeID),
		SessionID:         strings.TrimSpace(req.SessionID),
		TaskID:            strings.TrimSpace(req.TaskID),
		UserPrompt:        strings.TrimSpace(req.UserPrompt),
		Mode:              s.cfg.Mode,
		CheckpointStyle:   s.cfg.CheckpointStyle,
		Policy:            MergeCheckpointPolicy(s.cfg.Policy, req.PolicyOverrides),
		PolicyOverrides:   cloneCheckpointPolicyOverride(req.PolicyOverrides),
		Status:            WorkflowRunStatusCreated,
		CreatedAt:         now,
		CurrentPhaseIndex: 0,
		CurrentStepIndex:  0,
		Phases:            instantiatePhases(template),
	}
	controls := s.engine.executionControls()
	if controls.Enabled && controls.Commit.RequireApproval {
		run.Policy.HardGates.PreCommitApproval = true
		run.Policy.ConditionalGates.PreCommitApproval = true
		run.Policy = NormalizeCheckpointPolicy(run.Policy)
	}
	s.runs[runID] = run
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "run",
		Action:  "run_created",
		Outcome: "created",
		Detail:  "template=" + template.ID,
	})
	s.timelines[runID] = append(s.timelines[runID], RunTimelineEvent{
		At:      now,
		Type:    "run_created",
		RunID:   runID,
		Message: "workflow run created",
	})
	s.persistRunSnapshotLocked(ctx, runID)
	return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) ListTemplates(ctx context.Context) ([]WorkflowTemplate, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: run service is nil", ErrInvalidTransition)
	}
	resolved := s.resolveTemplates(ctx)
	out := make([]WorkflowTemplate, 0, len(resolved))
	for _, tpl := range resolved {
		out = append(out, cloneTemplate(tpl))
	}
	sort.Slice(out, func(i, j int) bool {
		leftName := strings.ToLower(strings.TrimSpace(out[i].Name))
		rightName := strings.ToLower(strings.TrimSpace(out[j].Name))
		if leftName != rightName {
			return leftName < rightName
		}
		leftID := strings.ToLower(strings.TrimSpace(out[i].ID))
		rightID := strings.ToLower(strings.TrimSpace(out[j].ID))
		return leftID < rightID
	})
	return out, nil
}

func defaultTemplateID(templates map[string]WorkflowTemplate) string {
	if len(templates) == 0 {
		return ""
	}
	if _, ok := templates[TemplateIDSolidPhaseDelivery]; ok {
		return TemplateIDSolidPhaseDelivery
	}
	ids := make([]string, 0, len(templates))
	for id := range templates {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return ""
	}
	sort.Strings(ids)
	return ids[0]
}

func (s *InMemoryRunService) resolveTemplates(ctx context.Context) map[string]WorkflowTemplate {
	defaults := make(map[string]WorkflowTemplate, len(s.templates))
	for id, tpl := range s.templates {
		defaults[id] = cloneTemplate(tpl)
	}
	if s.templateProvider == nil {
		return defaults
	}
	templates, err := s.templateProvider.ListWorkflowTemplates(ctx)
	if err != nil {
		return defaults
	}
	resolved := map[string]WorkflowTemplate{}
	for _, tpl := range templates {
		id := strings.TrimSpace(tpl.ID)
		if id == "" {
			continue
		}
		if !templateHasSteps(tpl) {
			continue
		}
		resolved[id] = cloneTemplate(tpl)
	}
	hasExplicitConfig := len(resolved) > 0
	if awareProvider, ok := s.templateProvider.(TemplateConfigPresenceProvider); ok {
		if configured, cfgErr := awareProvider.HasWorkflowTemplateConfig(ctx); cfgErr == nil {
			hasExplicitConfig = configured || len(resolved) > 0
		}
	}
	if hasExplicitConfig {
		return resolved
	}
	return defaults
}

func templateHasSteps(template WorkflowTemplate) bool {
	for _, phase := range template.Phases {
		if len(phase.Steps) > 0 {
			return true
		}
	}
	return false
}

func (s *InMemoryRunService) StartRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	run, err := s.transitionAndAdvance(ctx, runID, "start")
	s.persistMetrics(ctx)
	return run, err
}

func (s *InMemoryRunService) PauseRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	s.mu.Lock()
	defer s.persistMetrics(ctx)
	defer s.mu.Unlock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}
	if run.Status != WorkflowRunStatusRunning {
		return nil, invalidTransitionError("pause", run.Status)
	}
	s.setRunPausedLocked(run, "run_paused", "manual pause")
	s.persistRunSnapshotLocked(ctx, run.ID)
	return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) ResumeRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	run, err := s.transitionAndAdvance(ctx, runID, "resume")
	s.persistMetrics(ctx)
	return run, err
}

func (s *InMemoryRunService) ResumeFailedRun(ctx context.Context, runID string, req ResumeFailedRunRequest) (*WorkflowRun, error) {
	s.mu.Lock()
	defer s.persistMetrics(ctx)
	defer s.mu.Unlock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}
	defer s.persistRunSnapshotLocked(ctx, run.ID)
	if run.Status != WorkflowRunStatusFailed {
		return nil, invalidTransitionError("resume_failed", run.Status)
	}
	phaseIndex, stepIndex, ok := findResumeTargetStep(run)
	if !ok {
		return nil, fmt.Errorf("%w: no resumable workflow step found", ErrInvalidTransition)
	}
	now := s.engine.now()
	phase := &run.Phases[phaseIndex]
	step := &phase.Steps[stepIndex]
	markStepExecutionInterrupted(step, now)
	resetStepForResume(step)
	if phase.Status != PhaseRunStatusRunning {
		phase.Status = PhaseRunStatusRunning
		if phase.StartedAt == nil {
			phase.StartedAt = &now
		}
	}
	phase.CompletedAt = nil
	run.Status = WorkflowRunStatusRunning
	run.PausedAt = nil
	run.CompletedAt = nil
	run.LastError = ""
	run.CurrentPhaseIndex = phaseIndex
	run.CurrentStepIndex = stepIndex
	if sessionID := firstNonEmpty(resumeStepSessionID(step), strings.TrimSpace(run.SessionID)); sessionID != "" {
		run.SessionID = sessionID
	}
	message := normalizeResumeFailedRunMessage(req.Message)
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "run",
		Action:  "run_resumed_after_failure",
		PhaseID: phase.ID,
		StepID:  step.ID,
		Outcome: "running",
		Detail:  message,
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "run_resumed_after_failure",
		RunID:   run.ID,
		PhaseID: phase.ID,
		StepID:  step.ID,
		Message: message,
	})
	dispatched, err := s.dispatchPromptForStepLocked(ctx, run, phaseIndex, stepIndex, message)
	if err != nil {
		return nil, err
	}
	if !dispatched {
		cause := fmt.Errorf("%w: resume dispatch was not attempted", ErrStepDispatch)
		s.failRunForStepDispatchLocked(run, phaseIndex, stepIndex, cause)
		return nil, cause
	}
	return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) AdvanceRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	s.mu.Lock()
	defer s.persistMetrics(ctx)
	defer s.mu.Unlock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}
	if run.Status != WorkflowRunStatusRunning {
		return nil, invalidTransitionError("advance", run.Status)
	}
	if err := s.advanceOnceLocked(ctx, run); err != nil {
		return nil, err
	}
	s.persistRunSnapshotLocked(ctx, run.ID)
	return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) HandleDecision(ctx context.Context, runID string, req DecisionActionRequest) (*WorkflowRun, error) {
	s.mu.Lock()
	defer s.persistMetrics(ctx)
	defer s.mu.Unlock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}
	action, ok := normalizeDecisionAction(req.Action)
	if !ok {
		return nil, fmt.Errorf("%w: unknown decision action %q", ErrInvalidTransition, strings.TrimSpace(string(req.Action)))
	}
	decisionID := strings.TrimSpace(req.DecisionID)
	if decisionID == "" && run.LatestDecision != nil {
		decisionID = strings.TrimSpace(run.LatestDecision.ID)
	}
	decisionRef := s.lookupDecisionLocked(run, decisionID)
	key := decisionActionReceiptKey(run.ID, decisionID, action)
	if _, seen := s.actions[key]; seen {
		return cloneWorkflowRun(run), nil
	}
	switch action {
	case DecisionActionApproveContinue:
		if run.Status != WorkflowRunStatusPaused {
			return nil, invalidTransitionError(string(action), run.Status)
		}
		if err := s.resumeAndAdvanceWithoutPolicyLocked(ctx, run, strings.TrimSpace(req.Note)); err != nil {
			return nil, err
		}
		s.recordApprovalLatencyLocked(decisionRef)
	case DecisionActionRequestRevision:
		switch run.Status {
		case WorkflowRunStatusPaused:
			// Keep paused but register the user decision in the timeline.
		case WorkflowRunStatusRunning:
			s.setRunPausedLocked(run, "run_paused_by_decision", "pause requested by user decision")
		default:
			return nil, invalidTransitionError(string(action), run.Status)
		}
		s.recordInterventionCauseLocked("user_request_revision")
		s.appendDecisionTimelineLocked(run, "decision_revision_requested", decisionID, req.Note)
	case DecisionActionPauseRun:
		switch run.Status {
		case WorkflowRunStatusPaused:
			// idempotent
		case WorkflowRunStatusRunning:
			s.setRunPausedLocked(run, "run_paused_by_decision", "pause requested by user decision")
		default:
			return nil, invalidTransitionError(string(action), run.Status)
		}
		s.recordInterventionCauseLocked("user_pause_run")
		s.appendDecisionTimelineLocked(run, "decision_pause_requested", decisionID, req.Note)
	default:
		return nil, fmt.Errorf("%w: unknown decision action %q", ErrInvalidTransition, action)
	}
	phaseID, stepID := currentRunPosition(run)
	appendRunAudit(run, RunAuditEntry{
		At:      s.engine.now(),
		Scope:   "decision",
		Action:  "decision_action",
		PhaseID: phaseID,
		StepID:  stepID,
		Outcome: string(action),
		Detail:  strings.TrimSpace(req.Note),
	})
	s.actions[key] = struct{}{}
	s.persistRunSnapshotLocked(ctx, run.ID)
	return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) OnTurnCompleted(ctx context.Context, signal TurnSignal) ([]*WorkflowRun, error) {
	s.mu.Lock()
	defer s.persistMetrics(ctx)
	defer s.mu.Unlock()
	changedRunIDs := map[string]struct{}{}
	defer func() {
		for runID := range changedRunIDs {
			s.persistRunSnapshotLocked(ctx, runID)
		}
	}()
	normalized := normalizeTurnSignal(signal)
	if normalized.SessionID == "" {
		return nil, nil
	}
	updated := make([]*WorkflowRun, 0, 1)
	for _, run := range s.runs {
		if run == nil || run.Status != WorkflowRunStatusRunning {
			continue
		}
		if !s.turnMatcher.Matches(run, normalized) {
			continue
		}
		if normalized.TurnID != "" {
			receipt := turnReceiptKey(run.ID, normalized.TurnID)
			if _, seen := s.turnSeen[receipt]; seen {
				continue
			}
			s.turnSeen[receipt] = struct{}{}
		}
		beforeStatus := run.Status
		if _, err := s.completeAwaitingTurnStepLocked(run, normalized); err != nil {
			changedRunIDs[run.ID] = struct{}{}
			return nil, err
		}
		if run.Status != WorkflowRunStatusRunning {
			s.recordTerminalTransitionLocked(beforeStatus, run.Status)
			changedRunIDs[run.ID] = struct{}{}
			updated = append(updated, cloneWorkflowRun(run))
			continue
		}
		if err := s.advanceOnceLocked(ctx, run); err != nil {
			changedRunIDs[run.ID] = struct{}{}
			return nil, err
		}
		changedRunIDs[run.ID] = struct{}{}
		updated = append(updated, cloneWorkflowRun(run))
	}
	return updated, nil
}

func (s *InMemoryRunService) ListRuns(_ context.Context) ([]*WorkflowRun, error) {
	return s.listRuns(false)
}

func (s *InMemoryRunService) ListRunsIncludingDismissed(_ context.Context) ([]*WorkflowRun, error) {
	return s.listRuns(true)
}

func (s *InMemoryRunService) listRuns(includeDismissed bool) ([]*WorkflowRun, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: run service is nil", ErrInvalidTransition)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*WorkflowRun, 0, len(s.runs))
	for _, run := range s.runs {
		if run == nil {
			continue
		}
		if !includeDismissed && run.DismissedAt != nil {
			continue
		}
		out = append(out, cloneWorkflowRun(run))
	}
	sort.Slice(out, func(i, j int) bool {
		left := runListSortTime(out[i])
		right := runListSortTime(out[j])
		if left.Equal(right) {
			return strings.TrimSpace(out[i].ID) < strings.TrimSpace(out[j].ID)
		}
		return left.After(right)
	})
	return out, nil
}

func (s *InMemoryRunService) RenameRun(ctx context.Context, runID, name string) (*WorkflowRun, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: run service is nil", ErrInvalidTransition)
	}
	normalizedName := strings.TrimSpace(name)
	if normalizedName == "" {
		return nil, fmt.Errorf("%w: run name is required", ErrInvalidTransition)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(run.TemplateName) == normalizedName {
		return cloneWorkflowRun(run), nil
	}
	now := s.engine.now()
	run.TemplateName = normalizedName
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "run",
		Action:  "run_renamed",
		Outcome: "renamed",
		Detail:  "name=" + normalizedName,
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "run_renamed",
		RunID:   run.ID,
		Message: "workflow run renamed",
	})
	s.persistRunSnapshotLocked(ctx, run.ID)
	return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) DismissRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: run service is nil", ErrInvalidTransition)
	}
	normalizedRunID := strings.TrimSpace(runID)
	if normalizedRunID == "" {
		return nil, fmt.Errorf("%w: run id is required", ErrInvalidTransition)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resolvedContext, resolved := s.resolveMissingRunDismissalContext(ctx, normalizedRunID)

	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.prepareRunForDismissalLocked(normalizedRunID, resolvedContext, resolved)
	if err != nil {
		return nil, err
	}
	if run.DismissedAt != nil {
		return cloneWorkflowRun(run), nil
	}
	now := s.engine.now()
	s.applyDismissedStateLocked(run, now)
	s.persistRunSnapshotLocked(ctx, run.ID)
	return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) resolveMissingRunDismissalContext(
	ctx context.Context,
	runID string,
) (MissingRunDismissalContext, bool) {
	if s == nil || s.contextResolver == nil {
		return MissingRunDismissalContext{}, false
	}
	resolvedContext, ok, err := s.contextResolver.ResolveMissingRunContext(ctx, runID)
	if err != nil || !ok {
		return MissingRunDismissalContext{}, false
	}
	return normalizeMissingRunDismissalContext(resolvedContext), true
}

func (s *InMemoryRunService) prepareRunForDismissalLocked(
	runID string,
	resolvedContext MissingRunDismissalContext,
	contextResolved bool,
) (*WorkflowRun, error) {
	run, err := s.mustRunLocked(runID)
	if err == nil {
		return run, nil
	}
	if !errors.Is(err, ErrRunNotFound) {
		return nil, err
	}
	tombstone := s.buildMissingRunTombstone(runID, resolvedContext, contextResolved, s.engine.now())
	s.runs[runID] = tombstone
	s.ensureRunTimelineLocked(runID)
	return tombstone, nil
}

func (s *InMemoryRunService) buildMissingRunTombstone(
	runID string,
	resolvedContext MissingRunDismissalContext,
	contextResolved bool,
	now time.Time,
) *WorkflowRun {
	normalizedRunID := strings.TrimSpace(runID)
	normalizedContext := normalizeMissingRunDismissalContext(resolvedContext)
	factory := s.tombstoneFactory
	if factory == nil {
		factory = defaultMissingRunTombstoneFactory{}
	}
	tombstone := factory.BuildMissingRunTombstone(normalizedRunID, normalizedContext, contextResolved, now)
	if tombstone == nil {
		tombstone = defaultMissingRunTombstoneFactory{}.BuildMissingRunTombstone(normalizedRunID, normalizedContext, contextResolved, now)
	}
	tombstone = cloneWorkflowRun(tombstone)
	if tombstone == nil {
		tombstone = &WorkflowRun{}
	}
	tombstone.ID = normalizedRunID
	tombstone.TemplateName = strings.TrimSpace(tombstone.TemplateName)
	if tombstone.TemplateName == "" {
		tombstone.TemplateName = "Historical Guided Workflow"
	}
	tombstone.WorkspaceID = strings.TrimSpace(tombstone.WorkspaceID)
	if tombstone.WorkspaceID == "" {
		tombstone.WorkspaceID = normalizedContext.WorkspaceID
	}
	tombstone.WorktreeID = strings.TrimSpace(tombstone.WorktreeID)
	if tombstone.WorktreeID == "" {
		tombstone.WorktreeID = normalizedContext.WorktreeID
	}
	tombstone.SessionID = strings.TrimSpace(tombstone.SessionID)
	if tombstone.SessionID == "" {
		tombstone.SessionID = normalizedContext.SessionID
	}
	if tombstone.Status == "" {
		tombstone.Status = WorkflowRunStatusFailed
	}
	if tombstone.CreatedAt.IsZero() {
		tombstone.CreatedAt = now
	}
	if strings.TrimSpace(tombstone.LastError) == "" {
		tombstone.LastError = defaultMissingRunTombstoneError(contextResolved)
	}
	return tombstone
}

func (s *InMemoryRunService) ensureRunTimelineLocked(runID string) {
	if s == nil {
		return
	}
	if _, exists := s.timelines[runID]; exists {
		return
	}
	s.timelines[runID] = []RunTimelineEvent{}
}

func (s *InMemoryRunService) applyDismissedStateLocked(run *WorkflowRun, now time.Time) {
	if s == nil || run == nil {
		return
	}
	run.DismissedAt = &now
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "run",
		Action:  "run_dismissed",
		Outcome: "dismissed",
		Detail:  "run dismissed",
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "run_dismissed",
		RunID:   run.ID,
		Message: "workflow run dismissed",
	})
}

func (s *InMemoryRunService) UndismissRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}
	if run.DismissedAt == nil {
		return cloneWorkflowRun(run), nil
	}
	now := s.engine.now()
	run.DismissedAt = nil
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "run",
		Action:  "run_undismissed",
		Outcome: "visible",
		Detail:  "run restored",
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "run_undismissed",
		RunID:   run.ID,
		Message: "workflow run restored",
	})
	s.persistRunSnapshotLocked(ctx, run.ID)
	return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) GetRun(_ context.Context, runID string) (*WorkflowRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}
	return cloneWorkflowRun(run), nil
}

func runListSortTime(run *WorkflowRun) time.Time {
	if run == nil {
		return time.Time{}
	}
	latest := run.CreatedAt
	if run.StartedAt != nil && run.StartedAt.After(latest) {
		latest = *run.StartedAt
	}
	if run.PausedAt != nil && run.PausedAt.After(latest) {
		latest = *run.PausedAt
	}
	if run.CompletedAt != nil && run.CompletedAt.After(latest) {
		latest = *run.CompletedAt
	}
	if run.DismissedAt != nil && run.DismissedAt.After(latest) {
		latest = *run.DismissedAt
	}
	if n := len(run.AuditTrail); n > 0 {
		last := run.AuditTrail[n-1].At
		if last.After(latest) {
			latest = last
		}
	}
	return latest
}

func newWorkflowRunID() string {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err == nil {
		return "gwf-" + hex.EncodeToString(suffix[:])
	}
	return fmt.Sprintf("gwf-%d", time.Now().UTC().UnixNano())
}

func (s *InMemoryRunService) GetRunTimeline(_ context.Context, runID string) ([]RunTimelineEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}
	events := s.timelines[run.ID]
	out := make([]RunTimelineEvent, len(events))
	copy(out, events)
	return out, nil
}

func (s *InMemoryRunService) GetRunMetrics(_ context.Context) (RunMetricsSnapshot, error) {
	if s == nil {
		return RunMetricsSnapshot{}, fmt.Errorf("%w: run service is nil", ErrInvalidTransition)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot := RunMetricsSnapshot{
		Enabled:              s.telemetryEnabled,
		CapturedAt:           s.engine.now(),
		RunsStarted:          s.metrics.runsStarted,
		RunsCompleted:        s.metrics.runsCompleted,
		RunsFailed:           s.metrics.runsFailed,
		PauseCount:           s.metrics.pauseCount,
		ApprovalCount:        s.metrics.approvalCount,
		ApprovalLatencyMaxMS: s.metrics.approvalLatencyMax,
		InterventionCauses:   map[string]int{},
	}
	for cause, count := range s.metrics.interventionCauses {
		snapshot.InterventionCauses[cause] = count
	}
	if snapshot.RunsStarted > 0 {
		snapshot.PauseRate = float64(snapshot.PauseCount) / float64(snapshot.RunsStarted)
	}
	if snapshot.ApprovalCount > 0 {
		snapshot.ApprovalLatencyAvgMS = s.metrics.approvalLatencyTotal / int64(snapshot.ApprovalCount)
	}
	return snapshot, nil
}

func (s *InMemoryRunService) ResetRunMetrics(ctx context.Context) (RunMetricsSnapshot, error) {
	if s == nil {
		return RunMetricsSnapshot{}, fmt.Errorf("%w: run service is nil", ErrInvalidTransition)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	s.metrics = runServiceMetrics{
		interventionCauses: map[string]int{},
	}
	snapshot := RunMetricsSnapshot{
		Enabled:            s.telemetryEnabled,
		CapturedAt:         s.engine.now(),
		InterventionCauses: map[string]int{},
	}
	s.mu.Unlock()
	s.persistMetricsSnapshot(ctx, snapshot, true)
	return snapshot, nil
}

func (s *InMemoryRunService) transitionAndAdvance(ctx context.Context, runID string, action string) (*WorkflowRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.mustRunLocked(runID)
	if err != nil {
		return nil, err
	}
	defer s.persistRunSnapshotLocked(ctx, run.ID)
	now := s.engine.now()
	switch action {
	case "start":
		if run.Status != WorkflowRunStatusCreated {
			return nil, invalidTransitionError(action, run.Status)
		}
		run.Status = WorkflowRunStatusRunning
		run.StartedAt = &now
		run.PausedAt = nil
		s.recordRunStartedLocked(run)
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "run",
			Action:  "run_started",
			Outcome: "running",
			Detail:  "start requested",
		})
		s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
			At:    now,
			Type:  "run_started",
			RunID: run.ID,
		})
	case "resume":
		if run.Status != WorkflowRunStatusPaused {
			return nil, invalidTransitionError(action, run.Status)
		}
		run.Status = WorkflowRunStatusRunning
		run.PausedAt = nil
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "run",
			Action:  "run_resumed",
			Outcome: "running",
			Detail:  "resume requested",
		})
		s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
			At:    now,
			Type:  "run_resumed",
			RunID: run.ID,
		})
	default:
		return nil, fmt.Errorf("%w: unknown action %q", ErrInvalidTransition, action)
	}

	if err := s.advanceOnceLocked(ctx, run); err != nil {
		return nil, err
	}
	return cloneWorkflowRun(run), nil
}

func (s *InMemoryRunService) advanceOnceLocked(ctx context.Context, run *WorkflowRun) error {
	if run == nil {
		return fmt.Errorf("%w: run is required", ErrInvalidTransition)
	}
	if isRunAwaitingTurn(run) {
		return nil
	}
	beforeStatus := run.Status
	if paused := s.applyPolicyDecisionLocked(run, defaultPolicyEvaluationInput(run)); paused {
		return nil
	}
	if dispatched, err := s.dispatchNextStepPromptLocked(ctx, run); err != nil {
		s.recordTerminalTransitionLocked(beforeStatus, run.Status)
		return err
	} else if dispatched {
		s.recordTerminalTransitionLocked(beforeStatus, run.Status)
		return nil
	}
	timeline := s.timelines[run.ID]
	if err := s.engine.Advance(ctx, run, &timeline); err != nil {
		s.timelines[run.ID] = timeline
		s.recordTerminalTransitionLocked(beforeStatus, run.Status)
		return err
	}
	s.timelines[run.ID] = timeline
	s.recordTerminalTransitionLocked(beforeStatus, run.Status)
	return nil
}

func (s *InMemoryRunService) dispatchNextStepPromptLocked(ctx context.Context, run *WorkflowRun) (bool, error) {
	if s == nil || run == nil || s.stepDispatcher == nil {
		return false, nil
	}
	phaseIndex, stepIndex, ok := findNextPending(run)
	if !ok || phaseIndex < 0 || phaseIndex >= len(run.Phases) {
		return false, nil
	}
	phase := &run.Phases[phaseIndex]
	if stepIndex < 0 || stepIndex >= len(phase.Steps) {
		return false, nil
	}
	step := &phase.Steps[stepIndex]
	templatePrompt := strings.TrimSpace(step.Prompt)
	if templatePrompt == "" {
		return false, nil
	}
	dispatchPrompt := templatePrompt
	if shouldPrefixUserPrompt(run) {
		dispatchPrompt = composeInitialDispatchPrompt(run.UserPrompt, templatePrompt)
	}
	return s.dispatchPromptForStepLocked(ctx, run, phaseIndex, stepIndex, dispatchPrompt)
}

func (s *InMemoryRunService) dispatchPromptForStepLocked(
	ctx context.Context,
	run *WorkflowRun,
	phaseIndex int,
	stepIndex int,
	dispatchPrompt string,
) (bool, error) {
	if s == nil || run == nil || s.stepDispatcher == nil {
		return false, nil
	}
	if phaseIndex < 0 || phaseIndex >= len(run.Phases) {
		return false, nil
	}
	phase := &run.Phases[phaseIndex]
	if stepIndex < 0 || stepIndex >= len(phase.Steps) {
		return false, nil
	}
	step := &phase.Steps[stepIndex]
	dispatchPrompt = strings.TrimSpace(dispatchPrompt)
	if dispatchPrompt == "" {
		return false, nil
	}
	result, err := s.stepDispatcher.DispatchStepPrompt(ctx, StepPromptDispatchRequest{
		RunID:              strings.TrimSpace(run.ID),
		TemplateID:         strings.TrimSpace(run.TemplateID),
		DefaultAccessLevel: run.DefaultAccessLevel,
		WorkspaceID:        strings.TrimSpace(run.WorkspaceID),
		WorktreeID:         strings.TrimSpace(run.WorktreeID),
		SessionID:          strings.TrimSpace(run.SessionID),
		PhaseID:            strings.TrimSpace(phase.ID),
		StepID:             strings.TrimSpace(step.ID),
		Prompt:             dispatchPrompt,
		RuntimeOptions:     types.CloneRuntimeOptions(step.RuntimeOptions),
	})
	if err != nil {
		if s.dispatchClassifier != nil && s.dispatchClassifier.Classify(err) == DispatchErrorDispositionDeferred {
			s.deferRunForStepDispatchLocked(run, phaseIndex, stepIndex, err)
			return true, nil
		}
		s.failRunForStepDispatchLocked(run, phaseIndex, stepIndex, err)
		return false, err
	}
	if !result.Dispatched {
		cause := fmt.Errorf("%w: dispatcher did not dispatch step prompt", ErrStepDispatch)
		s.failRunForStepDispatchLocked(run, phaseIndex, stepIndex, cause)
		return false, cause
	}
	if strings.TrimSpace(result.SessionID) == "" {
		cause := fmt.Errorf("%w: dispatcher returned dispatched step without session id", ErrStepDispatch)
		s.failRunForStepDispatchLocked(run, phaseIndex, stepIndex, cause)
		return false, cause
	}
	now := s.engine.now()
	run.CurrentPhaseIndex = phaseIndex
	run.CurrentStepIndex = stepIndex
	if phase.Status == PhaseRunStatusPending {
		phase.Status = PhaseRunStatusRunning
		phase.StartedAt = &now
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "phase",
			Action:  "phase_started",
			PhaseID: phase.ID,
			Outcome: "running",
			Detail:  phase.Name,
		})
		s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
			At:      now,
			Type:    "phase_started",
			RunID:   run.ID,
			PhaseID: phase.ID,
		})
	}
	step.Status = StepRunStatusRunning
	step.AwaitingTurn = true
	step.StartedAt = &now
	step.CompletedAt = nil
	step.Error = ""
	step.Outcome = "awaiting_turn"
	step.Output = strings.TrimSpace(result.TurnID)
	step.TurnID = strings.TrimSpace(result.TurnID)
	recordStepExecutionDispatch(run, phase, step, result, dispatchPrompt, now)
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "step",
		Action:  "step_prompt_dispatched",
		PhaseID: phase.ID,
		StepID:  step.ID,
		Outcome: "awaiting_turn",
		Detail:  "session=" + strings.TrimSpace(result.SessionID),
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "step_dispatched",
		RunID:   run.ID,
		PhaseID: phase.ID,
		StepID:  step.ID,
		Message: "awaiting turn completion",
	})
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" && strings.TrimSpace(run.SessionID) != sessionID {
		run.SessionID = sessionID
	}
	return true, nil
}

func (s *InMemoryRunService) failRunForStepDispatchLocked(run *WorkflowRun, phaseIndex, stepIndex int, cause error) {
	if s == nil || run == nil {
		return
	}
	if phaseIndex < 0 || phaseIndex >= len(run.Phases) {
		return
	}
	phase := &run.Phases[phaseIndex]
	if stepIndex < 0 || stepIndex >= len(phase.Steps) {
		return
	}
	step := &phase.Steps[stepIndex]
	now := s.engine.now()
	if phase.Status == PhaseRunStatusPending {
		phase.Status = PhaseRunStatusRunning
		phase.StartedAt = &now
	}
	step.Status = StepRunStatusFailed
	step.AwaitingTurn = false
	step.CompletedAt = &now
	step.Error = strings.TrimSpace(cause.Error())
	step.Outcome = "failed"
	recordStepExecutionFailure(step, "step dispatch failed", now)
	phase.Status = PhaseRunStatusFailed
	phase.CompletedAt = &now
	run.Status = WorkflowRunStatusFailed
	run.CompletedAt = &now
	run.LastError = strings.TrimSpace(cause.Error())
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "step",
		Action:  "step_dispatch_failed",
		PhaseID: phase.ID,
		StepID:  step.ID,
		Outcome: "failed",
		Detail:  run.LastError,
	})
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "run",
		Action:  "run_failed",
		PhaseID: phase.ID,
		StepID:  step.ID,
		Outcome: "failed",
		Detail:  run.LastError,
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "step_failed",
		RunID:   run.ID,
		PhaseID: phase.ID,
		StepID:  step.ID,
		Message: run.LastError,
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "run_failed",
		RunID:   run.ID,
		Message: run.LastError,
	})
}

func (s *InMemoryRunService) deferRunForStepDispatchLocked(run *WorkflowRun, phaseIndex, stepIndex int, cause error) {
	if s == nil || run == nil {
		return
	}
	if phaseIndex < 0 || phaseIndex >= len(run.Phases) {
		return
	}
	phase := &run.Phases[phaseIndex]
	if stepIndex < 0 || stepIndex >= len(phase.Steps) {
		return
	}
	step := &phase.Steps[stepIndex]
	now := s.engine.now()
	run.CurrentPhaseIndex = phaseIndex
	run.CurrentStepIndex = stepIndex
	if phase.Status == PhaseRunStatusPending {
		phase.Status = PhaseRunStatusRunning
		phase.StartedAt = &now
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "phase",
			Action:  "phase_started",
			PhaseID: phase.ID,
			Outcome: "running",
			Detail:  phase.Name,
		})
		s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
			At:      now,
			Type:    "phase_started",
			RunID:   run.ID,
			PhaseID: phase.ID,
		})
	}
	step.Status = StepRunStatusPending
	step.AwaitingTurn = false
	step.CompletedAt = nil
	step.Error = ""
	step.Outcome = "waiting_dispatch"
	step.Output = ""
	step.TurnID = ""
	recordStepExecutionDeferred(step, strings.TrimSpace(cause.Error()), now)
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "step",
		Action:  "step_dispatch_deferred",
		PhaseID: phase.ID,
		StepID:  step.ID,
		Outcome: "waiting_dispatch",
		Detail:  strings.TrimSpace(cause.Error()),
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "step_dispatch_deferred",
		RunID:   run.ID,
		PhaseID: phase.ID,
		StepID:  step.ID,
		Message: strings.TrimSpace(cause.Error()),
	})
	if s.dispatchRetryScheduler != nil {
		s.dispatchRetryScheduler.Enqueue(run.ID)
	}
}

func (s *InMemoryRunService) retryDeferredDispatch(runID string) bool {
	if s == nil {
		return true
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[runID]
	if !ok || run == nil {
		return true
	}
	if run.Status != WorkflowRunStatusRunning || !runHasDeferredDispatch(run) {
		return true
	}
	if err := s.advanceOnceLocked(context.Background(), run); err != nil {
		s.persistRunSnapshotLocked(context.Background(), run.ID)
		return true
	}
	s.persistRunSnapshotLocked(context.Background(), run.ID)
	return run.Status != WorkflowRunStatusRunning || !runHasDeferredDispatch(run)
}

func (s *InMemoryRunService) completeAwaitingTurnStepLocked(run *WorkflowRun, signal TurnSignal) (bool, error) {
	if s == nil || run == nil {
		return false, nil
	}
	phaseIndex, stepIndex, ok := findAwaitingTurn(run)
	if !ok {
		return false, nil
	}
	if phaseIndex < 0 || phaseIndex >= len(run.Phases) {
		return false, fmt.Errorf("%w: awaiting phase index out of range", ErrInvalidTransition)
	}
	phase := &run.Phases[phaseIndex]
	if stepIndex < 0 || stepIndex >= len(phase.Steps) {
		return false, fmt.Errorf("%w: awaiting step index out of range", ErrInvalidTransition)
	}
	step := &phase.Steps[stepIndex]
	expectedTurnID := strings.TrimSpace(step.TurnID)
	signalTurnID := strings.TrimSpace(signal.TurnID)
	if expectedTurnID != "" && signalTurnID == "" {
		now := s.engine.now()
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "step",
			Action:  "step_turn_signal_ignored",
			PhaseID: phase.ID,
			StepID:  step.ID,
			Outcome: "awaiting_turn",
			Detail:  "missing turn_id while step awaits " + expectedTurnID,
		})
		s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
			At:      now,
			Type:    "step_turn_signal_ignored",
			RunID:   run.ID,
			PhaseID: phase.ID,
			StepID:  step.ID,
			Message: "missing turn_id for awaiting step",
		})
		return false, nil
	}
	if expectedTurnID != "" && signalTurnID != "" && signalTurnID != expectedTurnID {
		now := s.engine.now()
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "step",
			Action:  "step_turn_signal_ignored",
			PhaseID: phase.ID,
			StepID:  step.ID,
			Outcome: "awaiting_turn",
			Detail:  "turn_id mismatch expected=" + expectedTurnID + " got=" + signalTurnID,
		})
		s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
			At:      now,
			Type:    "step_turn_signal_ignored",
			RunID:   run.ID,
			PhaseID: phase.ID,
			StepID:  step.ID,
			Message: "turn_id mismatch for awaiting step",
		})
		return false, nil
	}
	now := s.engine.now()
	step.Status = StepRunStatusCompleted
	step.AwaitingTurn = false
	step.CompletedAt = &now
	step.Error = ""
	step.Outcome = "success"
	if strings.TrimSpace(signal.TurnID) != "" {
		step.TurnID = strings.TrimSpace(signal.TurnID)
		if strings.TrimSpace(step.Output) == "" {
			step.Output = step.TurnID
		}
	}
	recordStepExecutionCompletion(run, phase, step, signal, now)
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "step",
		Action:  "step_completed",
		PhaseID: phase.ID,
		StepID:  step.ID,
		Outcome: "success",
		Detail:  "completed by turn signal",
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "step_completed",
		RunID:   run.ID,
		PhaseID: phase.ID,
		StepID:  step.ID,
		Message: "completed by turn",
	})
	if phaseComplete(phase) {
		phase.Status = PhaseRunStatusCompleted
		phase.CompletedAt = &now
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "phase",
			Action:  "phase_completed",
			PhaseID: phase.ID,
			Outcome: "success",
			Detail:  phase.Name,
		})
		s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
			At:      now,
			Type:    "phase_completed",
			RunID:   run.ID,
			PhaseID: phase.ID,
		})
	}
	nextPhase, nextStep, hasNext := findNextPending(run)
	if hasNext {
		run.CurrentPhaseIndex = nextPhase
		run.CurrentStepIndex = nextStep
		return true, nil
	}
	run.Status = WorkflowRunStatusCompleted
	run.CompletedAt = &now
	run.LastError = ""
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "run",
		Action:  "run_completed",
		Outcome: "success",
		Detail:  "all workflow steps completed",
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:    now,
		Type:  "run_completed",
		RunID: run.ID,
	})
	return true, nil
}

func (s *InMemoryRunService) resumeAndAdvanceWithoutPolicyLocked(ctx context.Context, run *WorkflowRun, note string) error {
	if run == nil {
		return fmt.Errorf("%w: run is required", ErrInvalidTransition)
	}
	now := s.engine.now()
	run.Status = WorkflowRunStatusRunning
	run.PausedAt = nil
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "decision",
		Action:  "decision_approved_continue",
		Outcome: "running",
		Detail:  strings.TrimSpace(note),
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    "run_resumed_by_decision",
		RunID:   run.ID,
		Message: strings.TrimSpace(note),
	})
	s.appendDecisionTimelineLocked(run, "decision_approved_continue", "", note)

	timeline := s.timelines[run.ID]
	beforeStatus := run.Status
	if err := s.engine.Advance(ctx, run, &timeline); err != nil {
		s.timelines[run.ID] = timeline
		s.recordTerminalTransitionLocked(beforeStatus, run.Status)
		return err
	}
	s.timelines[run.ID] = timeline
	s.recordTerminalTransitionLocked(beforeStatus, run.Status)
	return nil
}

func (s *InMemoryRunService) setRunPausedLocked(run *WorkflowRun, eventType, detail string) {
	if run == nil {
		return
	}
	if run.Status == WorkflowRunStatusPaused {
		return
	}
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = "run paused"
	}
	now := s.engine.now()
	run.Status = WorkflowRunStatusPaused
	run.PausedAt = &now
	s.recordPauseLocked()
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "run",
		Action:  strings.TrimSpace(eventType),
		Outcome: "paused",
		Detail:  detail,
	})
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:    now,
		Type:  strings.TrimSpace(eventType),
		RunID: run.ID,
	})
}

func (s *InMemoryRunService) appendDecisionTimelineLocked(run *WorkflowRun, eventType, decisionID, note string) {
	if run == nil {
		return
	}
	message := strings.TrimSpace(note)
	if message == "" {
		message = strings.TrimSpace(decisionID)
	}
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      s.engine.now(),
		Type:    strings.TrimSpace(eventType),
		RunID:   run.ID,
		Message: message,
	})
}

func normalizeDecisionAction(raw DecisionAction) (DecisionAction, bool) {
	value := strings.ToLower(strings.TrimSpace(string(raw)))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	switch value {
	case "approve", "continue", "approve_continue":
		return DecisionActionApproveContinue, true
	case "request_revision", "revise", "revision":
		return DecisionActionRequestRevision, true
	case "pause", "pause_run":
		return DecisionActionPauseRun, true
	default:
		return "", false
	}
}

func decisionActionReceiptKey(runID, decisionID string, action DecisionAction) string {
	return strings.Join([]string{
		strings.TrimSpace(runID),
		strings.TrimSpace(decisionID),
		strings.TrimSpace(string(action)),
	}, "|")
}

func turnReceiptKey(runID, turnID string) string {
	return strings.Join([]string{
		strings.TrimSpace(runID),
		strings.TrimSpace(turnID),
	}, "|")
}

func normalizeTurnSignal(signal TurnSignal) TurnSignal {
	signal.SessionID = strings.TrimSpace(signal.SessionID)
	signal.WorkspaceID = strings.TrimSpace(signal.WorkspaceID)
	signal.WorktreeID = strings.TrimSpace(signal.WorktreeID)
	signal.TurnID = strings.TrimSpace(signal.TurnID)
	return signal
}

func runMatchesTurnSignal(run *WorkflowRun, signal TurnSignal) bool {
	if run == nil {
		return false
	}
	sessionID := strings.TrimSpace(run.SessionID)
	workspaceID := strings.TrimSpace(run.WorkspaceID)
	worktreeID := strings.TrimSpace(run.WorktreeID)
	if signal.SessionID != "" && sessionID == signal.SessionID {
		return true
	}
	if signal.WorktreeID != "" && worktreeID == signal.WorktreeID {
		return true
	}
	if signal.WorkspaceID != "" && workspaceID == signal.WorkspaceID {
		return true
	}
	return false
}

func runSessionScope(run *WorkflowRun) string {
	if run == nil {
		return ""
	}
	if strings.TrimSpace(run.WorktreeID) != "" {
		return "worktree"
	}
	if strings.TrimSpace(run.WorkspaceID) != "" {
		return "workspace"
	}
	if strings.TrimSpace(run.SessionID) != "" {
		return "session"
	}
	return ""
}

func stepTraceID(run *WorkflowRun, phase *PhaseRun, step *StepRun, attempt int) string {
	if run == nil {
		return ""
	}
	parts := []string{
		strings.TrimSpace(run.ID),
	}
	if phase != nil {
		parts = append(parts, strings.TrimSpace(phase.ID))
	}
	if step != nil {
		parts = append(parts, strings.TrimSpace(step.ID))
	}
	if attempt > 0 {
		parts = append(parts, fmt.Sprintf("attempt-%d", attempt))
	}
	return strings.Join(parts, ":")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func recordStepExecutionDispatch(
	run *WorkflowRun,
	phase *PhaseRun,
	step *StepRun,
	result StepPromptDispatchResult,
	prompt string,
	now time.Time,
) {
	if step == nil {
		return
	}
	step.Attempts++
	step.ExecutionState = StepExecutionStateLinked
	step.ExecutionMessage = ""
	execution := StepExecutionRef{
		TraceID:        stepTraceID(run, phase, step, step.Attempts),
		SessionID:      strings.TrimSpace(result.SessionID),
		SessionScope:   runSessionScope(run),
		Provider:       strings.TrimSpace(result.Provider),
		Model:          strings.TrimSpace(result.Model),
		TurnID:         strings.TrimSpace(result.TurnID),
		PromptSnapshot: strings.TrimSpace(prompt),
		StartedAt:      &now,
	}
	step.Execution = &execution
	step.ExecutionAttempts = append(step.ExecutionAttempts, execution)
}

func recordStepExecutionFailure(step *StepRun, message string, now time.Time) {
	if step == nil {
		return
	}
	step.ExecutionState = StepExecutionStateUnavailable
	step.ExecutionMessage = strings.TrimSpace(message)
	if step.Execution != nil {
		step.Execution.CompletedAt = &now
	}
	if len(step.ExecutionAttempts) > 0 {
		step.ExecutionAttempts[len(step.ExecutionAttempts)-1].CompletedAt = &now
	}
}

func recordStepExecutionDeferred(step *StepRun, message string, now time.Time) {
	if step == nil {
		return
	}
	step.ExecutionState = StepExecutionStateDeferred
	step.ExecutionMessage = strings.TrimSpace(message)
	if step.Execution != nil {
		step.Execution.CompletedAt = &now
	}
	if len(step.ExecutionAttempts) > 0 {
		step.ExecutionAttempts[len(step.ExecutionAttempts)-1].CompletedAt = &now
	}
}

func recordStepExecutionCompletion(run *WorkflowRun, phase *PhaseRun, step *StepRun, signal TurnSignal, now time.Time) {
	if step == nil {
		return
	}
	step.ExecutionState = StepExecutionStateLinked
	step.ExecutionMessage = ""
	if step.Execution == nil {
		execution := StepExecutionRef{
			TraceID:      stepTraceID(run, phase, step, max(1, step.Attempts)),
			SessionID:    firstNonEmpty(strings.TrimSpace(signal.SessionID), strings.TrimSpace(run.SessionID)),
			SessionScope: runSessionScope(run),
			TurnID:       strings.TrimSpace(step.TurnID),
			StartedAt:    step.StartedAt,
			CompletedAt:  &now,
		}
		step.Execution = &execution
		step.ExecutionAttempts = append(step.ExecutionAttempts, execution)
		return
	}
	step.Execution.CompletedAt = &now
	if signalTurnID := strings.TrimSpace(signal.TurnID); signalTurnID != "" {
		step.Execution.TurnID = signalTurnID
	}
	if strings.TrimSpace(step.Execution.TurnID) == "" {
		step.Execution.TurnID = strings.TrimSpace(step.TurnID)
	}
	if strings.TrimSpace(step.Execution.SessionID) == "" {
		step.Execution.SessionID = firstNonEmpty(strings.TrimSpace(signal.SessionID), strings.TrimSpace(run.SessionID))
	}
	if len(step.ExecutionAttempts) > 0 {
		step.ExecutionAttempts[len(step.ExecutionAttempts)-1] = *step.Execution
	}
}

func shouldPrefixUserPrompt(run *WorkflowRun) bool {
	if run == nil || strings.TrimSpace(run.UserPrompt) == "" {
		return false
	}
	for _, phase := range run.Phases {
		for _, step := range phase.Steps {
			if step.Attempts > 0 || len(step.ExecutionAttempts) > 0 {
				return false
			}
		}
	}
	return true
}

func composeInitialDispatchPrompt(userPrompt, templatePrompt string) string {
	userPrompt = strings.TrimSpace(userPrompt)
	templatePrompt = strings.TrimSpace(templatePrompt)
	if userPrompt == "" {
		return templatePrompt
	}
	if templatePrompt == "" {
		return userPrompt
	}
	return userPrompt + "\n\n" + templatePrompt
}

func normalizeResumeFailedRunMessage(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultResumeFailedRunMessage
	}
	return trimmed
}

func findResumeTargetStep(run *WorkflowRun) (phaseIndex int, stepIndex int, ok bool) {
	if run == nil {
		return 0, 0, false
	}
	if p, s, hasCurrent := currentRunStepIndex(run); hasCurrent {
		step := run.Phases[p].Steps[s]
		if step.Status != StepRunStatusCompleted {
			return p, s, true
		}
	}
	lastPhase := -1
	lastStep := -1
	for pIndex, phase := range run.Phases {
		for sIndex, step := range phase.Steps {
			if step.Status == StepRunStatusFailed || step.Status == StepRunStatusRunning {
				lastPhase = pIndex
				lastStep = sIndex
			}
		}
	}
	if lastPhase >= 0 {
		return lastPhase, lastStep, true
	}
	if p, s, hasPending := findNextPending(run); hasPending {
		return p, s, true
	}
	return 0, 0, false
}

func currentRunStepIndex(run *WorkflowRun) (phaseIndex int, stepIndex int, ok bool) {
	if run == nil || len(run.Phases) == 0 {
		return 0, 0, false
	}
	phaseIndex = run.CurrentPhaseIndex
	if phaseIndex < 0 || phaseIndex >= len(run.Phases) {
		return 0, 0, false
	}
	stepIndex = run.CurrentStepIndex
	if stepIndex < 0 || stepIndex >= len(run.Phases[phaseIndex].Steps) {
		return 0, 0, false
	}
	return phaseIndex, stepIndex, true
}

func markStepExecutionInterrupted(step *StepRun, now time.Time) {
	if step == nil {
		return
	}
	if step.Execution != nil && step.Execution.CompletedAt == nil {
		step.Execution.CompletedAt = &now
	}
	if len(step.ExecutionAttempts) == 0 {
		return
	}
	lastIdx := len(step.ExecutionAttempts) - 1
	if step.ExecutionAttempts[lastIdx].CompletedAt == nil {
		step.ExecutionAttempts[lastIdx].CompletedAt = &now
	}
}

func resetStepForResume(step *StepRun) {
	if step == nil {
		return
	}
	step.Status = StepRunStatusPending
	step.AwaitingTurn = false
	step.TurnID = ""
	step.StartedAt = nil
	step.CompletedAt = nil
	step.Outcome = ""
	step.Output = ""
	step.Error = ""
	step.Execution = nil
	step.ExecutionState = StepExecutionStateNone
	step.ExecutionMessage = ""
}

func resumeStepSessionID(step *StepRun) string {
	if step == nil {
		return ""
	}
	if step.Execution != nil {
		if sessionID := strings.TrimSpace(step.Execution.SessionID); sessionID != "" {
			return sessionID
		}
	}
	for i := len(step.ExecutionAttempts) - 1; i >= 0; i-- {
		if sessionID := strings.TrimSpace(step.ExecutionAttempts[i].SessionID); sessionID != "" {
			return sessionID
		}
	}
	return ""
}

func (s *InMemoryRunService) mustRunLocked(runID string) (*WorkflowRun, error) {
	id := strings.TrimSpace(runID)
	if id == "" {
		return nil, fmt.Errorf("%w: run id is required", ErrInvalidTransition)
	}
	run, ok := s.runs[id]
	if !ok || run == nil {
		return nil, fmt.Errorf("%w: %s", ErrRunNotFound, id)
	}
	return run, nil
}

func invalidTransitionError(action string, status WorkflowRunStatus) error {
	message := fmt.Sprintf("%s is not allowed while run is %s", strings.TrimSpace(action), status)
	return fmt.Errorf("%w: %s", ErrInvalidTransition, message)
}

func instantiatePhases(template WorkflowTemplate) []PhaseRun {
	phases := make([]PhaseRun, 0, len(template.Phases))
	for _, phase := range template.Phases {
		steps := make([]StepRun, 0, len(phase.Steps))
		for _, step := range phase.Steps {
			steps = append(steps, StepRun{
				ID:             step.ID,
				Name:           step.Name,
				Prompt:         strings.TrimSpace(step.Prompt),
				RuntimeOptions: types.CloneRuntimeOptions(step.RuntimeOptions),
				Status:         StepRunStatusPending,
				ExecutionState: StepExecutionStateNone,
			})
		}
		phases = append(phases, PhaseRun{
			ID:     phase.ID,
			Name:   phase.Name,
			Status: PhaseRunStatusPending,
			Steps:  steps,
		})
	}
	return phases
}

func cloneTemplate(in WorkflowTemplate) WorkflowTemplate {
	out := in
	out.Phases = make([]WorkflowTemplatePhase, len(in.Phases))
	for i, phase := range in.Phases {
		out.Phases[i] = phase
		out.Phases[i].Steps = append([]WorkflowTemplateStep{}, phase.Steps...)
		for j := range out.Phases[i].Steps {
			out.Phases[i].Steps[j].RuntimeOptions = types.CloneRuntimeOptions(out.Phases[i].Steps[j].RuntimeOptions)
		}
	}
	return out
}

func cloneWorkflowRun(in *WorkflowRun) *WorkflowRun {
	if in == nil {
		return nil
	}
	out := *in
	out.Phases = make([]PhaseRun, len(in.Phases))
	for i, phase := range in.Phases {
		out.Phases[i] = phase
		out.Phases[i].Steps = append([]StepRun{}, phase.Steps...)
		for j := range out.Phases[i].Steps {
			step := &out.Phases[i].Steps[j]
			step.RuntimeOptions = types.CloneRuntimeOptions(step.RuntimeOptions)
			if step.Execution != nil {
				execution := *step.Execution
				step.Execution = &execution
			}
			if len(step.ExecutionAttempts) > 0 {
				step.ExecutionAttempts = append([]StepExecutionRef{}, step.ExecutionAttempts...)
			}
		}
	}
	if in.PolicyOverrides != nil {
		out.PolicyOverrides = cloneCheckpointPolicyOverride(in.PolicyOverrides)
	}
	if in.LatestDecision != nil {
		decision := *in.LatestDecision
		decision.Metadata.Reasons = append([]CheckpointReason{}, in.LatestDecision.Metadata.Reasons...)
		out.LatestDecision = &decision
	}
	out.CheckpointDecisions = append([]CheckpointDecision{}, in.CheckpointDecisions...)
	out.AuditTrail = append([]RunAuditEntry{}, in.AuditTrail...)
	for i := range out.CheckpointDecisions {
		out.CheckpointDecisions[i].Metadata.Reasons = append([]CheckpointReason{}, out.CheckpointDecisions[i].Metadata.Reasons...)
	}
	return &out
}

func IsRunNotFound(err error) bool {
	return errors.Is(err, ErrRunNotFound)
}

func IsInvalidTransition(err error) bool {
	return errors.Is(err, ErrInvalidTransition)
}

func (s *InMemoryRunService) applyPolicyDecisionLocked(run *WorkflowRun, input PolicyEvaluationInput) bool {
	if run == nil {
		return false
	}
	now := s.engine.now()
	phaseID, stepID := "", ""
	if pIndex, sIndex, ok := findNextPending(run); ok {
		phaseID = run.Phases[pIndex].ID
		stepID = run.Phases[pIndex].Steps[sIndex].ID
	}
	metadata := EvaluateCheckpointPolicy(run.Policy, input, now)
	decision := CheckpointDecision{
		ID:          fmt.Sprintf("cd-%d", len(run.CheckpointDecisions)+1),
		RunID:       run.ID,
		PhaseID:     phaseID,
		StepID:      stepID,
		Decision:    string(metadata.Action),
		Reason:      summarizeDecisionReasons(metadata.Reasons),
		Source:      "policy_engine",
		RequestedAt: now,
		DecidedAt:   &now,
		Metadata:    metadata,
	}
	run.CheckpointDecisions = append(run.CheckpointDecisions, decision)
	copy := decision
	run.LatestDecision = &copy
	eventType := "policy_continue"
	if metadata.Action == CheckpointActionPause {
		eventType = "policy_pause"
	}
	message := string(metadata.Action) + " | " + string(metadata.Severity) + " | " + string(metadata.Tier)
	s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
		At:      now,
		Type:    eventType,
		RunID:   run.ID,
		PhaseID: phaseID,
		StepID:  stepID,
		Message: message,
	})
	if metadata.Action == CheckpointActionPause {
		for _, reason := range metadata.Reasons {
			cause := strings.TrimSpace(reason.Code)
			if cause == "" {
				cause = strings.TrimSpace(reason.Message)
			}
			s.recordInterventionCauseLocked(cause)
		}
		run.Status = WorkflowRunStatusPaused
		run.PausedAt = &now
		s.recordPauseLocked()
		appendRunAudit(run, RunAuditEntry{
			At:      now,
			Scope:   "decision",
			Action:  "policy_pause",
			PhaseID: phaseID,
			StepID:  stepID,
			Outcome: string(metadata.Severity),
			Detail:  decision.Reason,
		})
		s.timelines[run.ID] = append(s.timelines[run.ID], RunTimelineEvent{
			At:      now,
			Type:    "checkpoint_requested",
			RunID:   run.ID,
			PhaseID: phaseID,
			StepID:  stepID,
			Message: decision.Reason,
		})
		return true
	}
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "decision",
		Action:  "policy_continue",
		PhaseID: phaseID,
		StepID:  stepID,
		Outcome: string(metadata.Severity),
		Detail:  decision.Reason,
	})
	return false
}

func defaultPolicyEvaluationInput(run *WorkflowRun) PolicyEvaluationInput {
	input := PolicyEvaluationInput{}
	confidence := 0.90
	input.Confidence = &confidence
	if run == nil {
		return input
	}
	if pIndex, sIndex, ok := findNextPending(run); ok {
		step := run.Phases[pIndex].Steps[sIndex]
		if strings.EqualFold(step.ID, "commit") {
			input.PreCommitApprovalRequired = true
		}
	}
	return input
}

func summarizeDecisionReasons(reasons []CheckpointReason) string {
	if len(reasons) == 0 {
		return "no policy triggers"
	}
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		value := strings.TrimSpace(reason.Code)
		if value == "" {
			value = strings.TrimSpace(reason.Message)
		}
		if value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return "policy reason available"
	}
	return strings.Join(parts, ",")
}

func currentRunPosition(run *WorkflowRun) (phaseID string, stepID string) {
	if run == nil || len(run.Phases) == 0 {
		return "", ""
	}
	pIdx := run.CurrentPhaseIndex
	if pIdx < 0 || pIdx >= len(run.Phases) {
		return "", ""
	}
	phase := run.Phases[pIdx]
	phaseID = strings.TrimSpace(phase.ID)
	if len(phase.Steps) == 0 {
		return phaseID, ""
	}
	sIdx := run.CurrentStepIndex
	if sIdx < 0 || sIdx >= len(phase.Steps) {
		return phaseID, ""
	}
	stepID = strings.TrimSpace(phase.Steps[sIdx].ID)
	return phaseID, stepID
}

func isRunAwaitingTurn(run *WorkflowRun) bool {
	_, _, ok := findAwaitingTurn(run)
	return ok
}

func runHasDeferredDispatch(run *WorkflowRun) bool {
	if run == nil {
		return false
	}
	for _, phase := range run.Phases {
		for _, step := range phase.Steps {
			if step.Status != StepRunStatusPending {
				continue
			}
			if step.ExecutionState == StepExecutionStateDeferred || strings.EqualFold(strings.TrimSpace(step.Outcome), "waiting_dispatch") {
				return true
			}
		}
	}
	return false
}

func findAwaitingTurn(run *WorkflowRun) (phaseIndex int, stepIndex int, ok bool) {
	if run == nil {
		return 0, 0, false
	}
	for pIndex, phase := range run.Phases {
		for sIndex, step := range phase.Steps {
			if step.Status != StepRunStatusRunning {
				continue
			}
			if step.AwaitingTurn || strings.EqualFold(strings.TrimSpace(step.Outcome), "awaiting_turn") {
				return pIndex, sIndex, true
			}
		}
	}
	return 0, 0, false
}

func (s *InMemoryRunService) activeRunsLocked() int {
	count := 0
	for _, run := range s.runs {
		if run == nil {
			continue
		}
		if run.DismissedAt != nil {
			continue
		}
		switch run.Status {
		case WorkflowRunStatusCreated, WorkflowRunStatusRunning, WorkflowRunStatusPaused:
			count++
		}
	}
	return count
}

func (s *InMemoryRunService) recordRunStartedLocked(run *WorkflowRun) {
	if s == nil || !s.telemetryEnabled || run == nil {
		return
	}
	s.metrics.runsStarted++
}

func (s *InMemoryRunService) recordTerminalTransitionLocked(before, after WorkflowRunStatus) {
	if s == nil || !s.telemetryEnabled {
		return
	}
	if before != WorkflowRunStatusCompleted && after == WorkflowRunStatusCompleted {
		s.metrics.runsCompleted++
	}
	if before != WorkflowRunStatusFailed && after == WorkflowRunStatusFailed {
		s.metrics.runsFailed++
	}
}

func (s *InMemoryRunService) recordPauseLocked() {
	if s == nil || !s.telemetryEnabled {
		return
	}
	s.metrics.pauseCount++
}

func (s *InMemoryRunService) recordInterventionCauseLocked(cause string) {
	if s == nil || !s.telemetryEnabled {
		return
	}
	cause = strings.TrimSpace(cause)
	if cause == "" {
		cause = "unknown"
	}
	if s.metrics.interventionCauses == nil {
		s.metrics.interventionCauses = map[string]int{}
	}
	s.metrics.interventionCauses[cause]++
}

func (s *InMemoryRunService) lookupDecisionLocked(run *WorkflowRun, decisionID string) *CheckpointDecision {
	if run == nil {
		return nil
	}
	decisionID = strings.TrimSpace(decisionID)
	if decisionID != "" {
		for i := range run.CheckpointDecisions {
			if strings.TrimSpace(run.CheckpointDecisions[i].ID) == decisionID {
				return &run.CheckpointDecisions[i]
			}
		}
	}
	if run.LatestDecision != nil {
		return run.LatestDecision
	}
	return nil
}

func (s *InMemoryRunService) recordApprovalLatencyLocked(decision *CheckpointDecision) {
	if s == nil || !s.telemetryEnabled || decision == nil {
		return
	}
	if decision.Metadata.Action != CheckpointActionPause {
		return
	}
	startedAt := decision.RequestedAt
	if startedAt.IsZero() {
		return
	}
	latencyMS := s.engine.now().Sub(startedAt).Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}
	s.metrics.approvalCount++
	s.metrics.approvalLatencyTotal += latencyMS
	if latencyMS > s.metrics.approvalLatencyMax {
		s.metrics.approvalLatencyMax = latencyMS
	}
}

func (s *InMemoryRunService) persistMetrics(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	snapshot, err := s.GetRunMetrics(ctx)
	if err != nil {
		return
	}
	s.persistMetricsSnapshot(ctx, snapshot, false)
}

func (s *InMemoryRunService) persistMetricsSnapshot(ctx context.Context, snapshot RunMetricsSnapshot, force bool) {
	if s == nil || s.metricsStore == nil {
		return
	}
	if !force && !s.telemetryEnabled {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = s.metricsStore.SaveRunMetrics(ctx, snapshot)
}

func (s *InMemoryRunService) restoreMetrics(ctx context.Context) {
	if s == nil || s.metricsStore == nil || !s.telemetryEnabled {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	snapshot, err := s.metricsStore.LoadRunMetrics(ctx)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics.runsStarted = sanitizeCounter(snapshot.RunsStarted)
	s.metrics.runsCompleted = sanitizeCounter(snapshot.RunsCompleted)
	s.metrics.runsFailed = sanitizeCounter(snapshot.RunsFailed)
	s.metrics.pauseCount = sanitizeCounter(snapshot.PauseCount)
	s.metrics.approvalCount = sanitizeCounter(snapshot.ApprovalCount)
	s.metrics.approvalLatencyMax = sanitizeInt64Counter(snapshot.ApprovalLatencyMaxMS)
	if s.metrics.approvalCount > 0 {
		avg := sanitizeInt64Counter(snapshot.ApprovalLatencyAvgMS)
		s.metrics.approvalLatencyTotal = avg * int64(s.metrics.approvalCount)
	}
	if len(snapshot.InterventionCauses) == 0 {
		s.metrics.interventionCauses = map[string]int{}
		return
	}
	s.metrics.interventionCauses = make(map[string]int, len(snapshot.InterventionCauses))
	for cause, count := range snapshot.InterventionCauses {
		cause = strings.TrimSpace(cause)
		if cause == "" {
			continue
		}
		s.metrics.interventionCauses[cause] = sanitizeCounter(count)
	}
}

func (s *InMemoryRunService) restoreRuns(ctx context.Context) {
	if s == nil || s.runStore == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	snapshots, err := s.runStore.ListWorkflowRuns(ctx)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	recoveredRunIDs := make([]string, 0)
	for _, snapshot := range snapshots {
		if snapshot.Run == nil {
			continue
		}
		runID := strings.TrimSpace(snapshot.Run.ID)
		if runID == "" {
			continue
		}
		run := cloneWorkflowRun(snapshot.Run)
		if run == nil {
			continue
		}
		run.ID = runID
		timeline := append([]RunTimelineEvent(nil), snapshot.Timeline...)
		for i := range timeline {
			timeline[i].RunID = strings.TrimSpace(timeline[i].RunID)
			if timeline[i].RunID == "" {
				timeline[i].RunID = runID
			}
		}
		if s.recoverInterruptedRunLocked(run, &timeline) {
			recoveredRunIDs = append(recoveredRunIDs, runID)
		}
		s.runs[runID] = run
		s.timelines[runID] = timeline
		s.hydrateTurnReceiptsLocked(run)
	}
	for _, runID := range recoveredRunIDs {
		s.persistRunSnapshotLocked(ctx, runID)
	}
}

func (s *InMemoryRunService) hydrateTurnReceiptsLocked(run *WorkflowRun) {
	if s == nil || run == nil {
		return
	}
	runID := strings.TrimSpace(run.ID)
	if runID == "" {
		return
	}
	for _, phase := range run.Phases {
		for _, step := range phase.Steps {
			turnID := strings.TrimSpace(step.TurnID)
			if turnID == "" {
				continue
			}
			s.turnSeen[turnReceiptKey(runID, turnID)] = struct{}{}
		}
	}
}

func (s *InMemoryRunService) recoverInterruptedRunLocked(run *WorkflowRun, timeline *[]RunTimelineEvent) bool {
	if s == nil || run == nil {
		return false
	}
	switch run.Status {
	case WorkflowRunStatusCreated, WorkflowRunStatusRunning:
		// These states represent in-flight work that cannot be resumed safely
		// after daemon restart because turn progression is event-driven.
	default:
		return false
	}
	now := s.engine.now()
	run.Status = WorkflowRunStatusFailed
	run.CompletedAt = &now
	run.PausedAt = nil
	run.LastError = "workflow run interrupted by daemon restart"
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "run",
		Action:  "run_interrupted",
		Outcome: "failed",
		Detail:  run.LastError,
	})
	appendTimelineEvent(timeline, RunTimelineEvent{
		At:      now,
		Type:    "run_interrupted",
		RunID:   strings.TrimSpace(run.ID),
		Message: run.LastError,
	})
	return true
}

// persistRunSnapshotLocked snapshots a run and timeline while service state is locked.
func (s *InMemoryRunService) persistRunSnapshotLocked(ctx context.Context, runID string) {
	if s == nil || s.runStore == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	run, ok := s.runs[runID]
	if !ok || run == nil {
		return
	}
	snapshot := RunStatusSnapshot{
		Run:      cloneWorkflowRun(run),
		Timeline: append([]RunTimelineEvent(nil), s.timelines[runID]...),
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = s.runStore.UpsertWorkflowRun(ctx, snapshot)
}

func sanitizeCounter(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeMissingRunDismissalContext(in MissingRunDismissalContext) MissingRunDismissalContext {
	return MissingRunDismissalContext{
		WorkspaceID: strings.TrimSpace(in.WorkspaceID),
		WorktreeID:  strings.TrimSpace(in.WorktreeID),
		SessionID:   strings.TrimSpace(in.SessionID),
	}
}

type defaultMissingRunTombstoneFactory struct{}

func (defaultMissingRunTombstoneFactory) BuildMissingRunTombstone(
	runID string,
	dismissalContext MissingRunDismissalContext,
	contextResolved bool,
	now time.Time,
) *WorkflowRun {
	normalizedContext := normalizeMissingRunDismissalContext(dismissalContext)
	return &WorkflowRun{
		ID:           strings.TrimSpace(runID),
		TemplateName: "Historical Guided Workflow",
		WorkspaceID:  normalizedContext.WorkspaceID,
		WorktreeID:   normalizedContext.WorktreeID,
		SessionID:    normalizedContext.SessionID,
		Status:       WorkflowRunStatusFailed,
		CreatedAt:    now,
		LastError:    defaultMissingRunTombstoneError(contextResolved),
	}
}

func defaultMissingRunTombstoneError(contextResolved bool) string {
	if contextResolved {
		return "workflow run missing; visibility state restored from dismissal"
	}
	return "workflow run missing; dismissed tombstone created"
}

func sanitizeInt64Counter(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}
