package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/types"
)

type TitleGenerationQueue interface {
	EnqueueSessionTitle(req SessionTitleGenerationRequest)
	EnqueueWorkflowTitle(req WorkflowTitleGenerationRequest)
}

type SessionTitleGenerationRequest struct {
	SessionID     string
	Prompt        string
	ExpectedTitle string
}

type WorkflowTitleGenerationRequest struct {
	RunID         string
	Prompt        string
	ExpectedTitle string
}

type titleGenerator interface {
	GenerateTitle(ctx context.Context, prompt string) (string, error)
}

type generatedSessionTitleUpdater interface {
	TryUpdateGeneratedSessionTitle(ctx context.Context, sessionID, expectedTitle, generatedTitle string) (bool, error)
}

type generatedWorkflowTitleUpdater interface {
	TryUpdateGeneratedWorkflowTitle(ctx context.Context, runID, expectedTitle, generatedTitle string) (bool, error)
}

type asyncTitleGenerationService struct {
	generator       titleGenerator
	sessionUpdater  generatedSessionTitleUpdater
	workflowUpdater generatedWorkflowTitleUpdater
	logger          logging.Logger
	timeout         time.Duration

	jobs      chan titleGenerationJob
	closeOnce sync.Once
	closed    chan struct{}
	wg        sync.WaitGroup
}

type titleGenerationJobKind string

const (
	titleGenerationJobSession  titleGenerationJobKind = "session"
	titleGenerationJobWorkflow titleGenerationJobKind = "workflow"
)

type titleGenerationJob struct {
	kind          titleGenerationJobKind
	targetID      string
	prompt        string
	expectedTitle string
}

type titleGenerationWorkerOptions struct {
	Timeout time.Duration
	Buffer  int
}

func newAsyncTitleGenerationService(
	generator titleGenerator,
	sessionUpdater generatedSessionTitleUpdater,
	workflowUpdater generatedWorkflowTitleUpdater,
	logger logging.Logger,
	opts titleGenerationWorkerOptions,
) *asyncTitleGenerationService {
	if logger == nil {
		logger = logging.Nop()
	}
	service := &asyncTitleGenerationService{
		generator:       generator,
		sessionUpdater:  sessionUpdater,
		workflowUpdater: workflowUpdater,
		logger:          logger,
		jobs:            nil,
		closed:          make(chan struct{}),
		timeout:         opts.Timeout,
	}
	if opts.Buffer <= 0 {
		opts.Buffer = 128
	}
	if service.timeout <= 0 {
		service.timeout = 10 * time.Second
	}
	if service.generator == nil {
		return service
	}
	service.jobs = make(chan titleGenerationJob, opts.Buffer)
	service.wg.Add(1)
	go service.worker()
	return service
}

func (s *asyncTitleGenerationService) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		close(s.closed)
		if s.jobs != nil {
			close(s.jobs)
		}
		s.wg.Wait()
	})
}

func (s *asyncTitleGenerationService) EnqueueSessionTitle(req SessionTitleGenerationRequest) {
	if s == nil || s.jobs == nil {
		return
	}
	job := titleGenerationJob{
		kind:          titleGenerationJobSession,
		targetID:      strings.TrimSpace(req.SessionID),
		prompt:        strings.TrimSpace(req.Prompt),
		expectedTitle: strings.TrimSpace(req.ExpectedTitle),
	}
	s.enqueue(job)
}

func (s *asyncTitleGenerationService) EnqueueWorkflowTitle(req WorkflowTitleGenerationRequest) {
	if s == nil || s.jobs == nil {
		return
	}
	job := titleGenerationJob{
		kind:          titleGenerationJobWorkflow,
		targetID:      strings.TrimSpace(req.RunID),
		prompt:        strings.TrimSpace(req.Prompt),
		expectedTitle: strings.TrimSpace(req.ExpectedTitle),
	}
	s.enqueue(job)
}

func (s *asyncTitleGenerationService) enqueue(job titleGenerationJob) {
	if s == nil || s.jobs == nil {
		return
	}
	if strings.TrimSpace(job.targetID) == "" || strings.TrimSpace(job.prompt) == "" {
		return
	}
	select {
	case <-s.closed:
		return
	case s.jobs <- job:
		if s.logger != nil && s.logger.Enabled(logging.Debug) {
			s.logger.Debug("title_generation_enqueued",
				logging.F("kind", string(job.kind)),
				logging.F("target_id", job.targetID),
			)
		}
	default:
		if s.logger != nil {
			s.logger.Warn("title_generation_dropped",
				logging.F("kind", string(job.kind)),
				logging.F("target_id", job.targetID),
				logging.F("reason", "queue_full"),
			)
		}
	}
}

func (s *asyncTitleGenerationService) worker() {
	defer s.wg.Done()
	for job := range s.jobs {
		s.process(job)
	}
}

func (s *asyncTitleGenerationService) process(job titleGenerationJob) {
	if s == nil || s.generator == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	title, err := s.generator.GenerateTitle(ctx, job.prompt)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("title_generation_failed",
				logging.F("kind", string(job.kind)),
				logging.F("target_id", job.targetID),
				logging.F("error", err),
			)
		}
		return
	}
	title = trimTitle(sanitizeTitle(title))
	if strings.TrimSpace(title) == "" {
		if s.logger != nil && s.logger.Enabled(logging.Debug) {
			s.logger.Debug("title_generation_skipped",
				logging.F("kind", string(job.kind)),
				logging.F("target_id", job.targetID),
				logging.F("reason", "empty_generated_title"),
			)
		}
		return
	}
	var updated bool
	switch job.kind {
	case titleGenerationJobSession:
		if s.sessionUpdater == nil {
			return
		}
		updated, err = s.sessionUpdater.TryUpdateGeneratedSessionTitle(ctx, job.targetID, job.expectedTitle, title)
	case titleGenerationJobWorkflow:
		if s.workflowUpdater == nil {
			return
		}
		updated, err = s.workflowUpdater.TryUpdateGeneratedWorkflowTitle(ctx, job.targetID, job.expectedTitle, title)
	default:
		return
	}
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("title_generation_apply_failed",
				logging.F("kind", string(job.kind)),
				logging.F("target_id", job.targetID),
				logging.F("error", err),
			)
		}
		return
	}
	if s.logger != nil && s.logger.Enabled(logging.Debug) {
		status := "updated"
		if !updated {
			status = "skipped"
		}
		s.logger.Debug("title_generation_applied",
			logging.F("kind", string(job.kind)),
			logging.F("target_id", job.targetID),
			logging.F("status", status),
		)
	}
}

type defaultGeneratedSessionTitleUpdater struct {
	manager  *SessionManager
	stores   *Stores
	metadata MetadataEventPublisher
}

func newDefaultGeneratedSessionTitleUpdater(
	manager *SessionManager,
	stores *Stores,
	metadata MetadataEventPublisher,
) generatedSessionTitleUpdater {
	if manager == nil && (stores == nil || stores.Sessions == nil) {
		return nil
	}
	return &defaultGeneratedSessionTitleUpdater{manager: manager, stores: stores, metadata: metadata}
}

func (u *defaultGeneratedSessionTitleUpdater) TryUpdateGeneratedSessionTitle(
	ctx context.Context,
	sessionID,
	expectedTitle,
	generatedTitle string,
) (bool, error) {
	if u == nil {
		return false, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	expectedTitle = strings.TrimSpace(expectedTitle)
	generatedTitle = strings.TrimSpace(trimTitle(sanitizeTitle(generatedTitle)))
	if sessionID == "" || generatedTitle == "" {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if u.stores != nil && u.stores.SessionMeta != nil {
		meta, ok, err := u.stores.SessionMeta.Get(ctx, sessionID)
		if err != nil {
			return false, err
		}
		if ok && meta != nil && meta.TitleLocked {
			return false, nil
		}
	}
	currentTitle, found, err := u.currentSessionTitle(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	currentTitle = strings.TrimSpace(currentTitle)
	if expectedTitle != "" && currentTitle != expectedTitle {
		return false, nil
	}
	if currentTitle == generatedTitle {
		return false, nil
	}
	if u.manager != nil {
		if err := u.manager.UpdateGeneratedSessionTitle(sessionID, generatedTitle); err != nil {
			return false, err
		}
		return true, nil
	}
	if u.stores == nil || u.stores.Sessions == nil {
		return false, errors.New("session title update not available")
	}
	record, ok, err := u.stores.Sessions.GetRecord(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if !ok || record == nil || record.Session == nil {
		return false, nil
	}
	clone := *record.Session
	clone.Title = generatedTitle
	record.Session = &clone
	if _, err := u.stores.Sessions.UpsertRecord(ctx, record); err != nil {
		return false, err
	}
	now := time.Now().UTC()
	if u.stores.SessionMeta != nil {
		_, _ = u.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    sessionID,
			Title:        generatedTitle,
			LastActiveAt: &now,
		})
	}
	if u.metadata != nil {
		u.metadata.PublishMetadataEvent(types.MetadataEvent{
			Version: types.MetadataEventSchemaVersionV1,
			Type:    types.MetadataEventTypeSessionUpdated,
			Session: &types.MetadataEntityUpdated{
				ID:        sessionID,
				Title:     generatedTitle,
				UpdatedAt: now,
				Changed: map[string]any{
					"title": generatedTitle,
				},
			},
		})
	}
	return true, nil
}

func (u *defaultGeneratedSessionTitleUpdater) currentSessionTitle(ctx context.Context, sessionID string) (string, bool, error) {
	if u == nil {
		return "", false, nil
	}
	if u.manager != nil {
		if session, ok := u.manager.GetSession(sessionID); ok && session != nil {
			return strings.TrimSpace(session.Title), true, nil
		}
	}
	if u.stores == nil || u.stores.Sessions == nil {
		return "", false, nil
	}
	record, ok, err := u.stores.Sessions.GetRecord(ctx, sessionID)
	if err != nil {
		return "", false, err
	}
	if !ok || record == nil || record.Session == nil {
		return "", false, nil
	}
	return strings.TrimSpace(record.Session.Title), true, nil
}

type workflowRunTitleService interface {
	GetRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	RenameRun(ctx context.Context, runID, name string) (*guidedworkflows.WorkflowRun, error)
}

type defaultGeneratedWorkflowTitleUpdater struct {
	runs workflowRunTitleService
}

func newDefaultGeneratedWorkflowTitleUpdater(runs workflowRunTitleService) generatedWorkflowTitleUpdater {
	if runs == nil {
		return nil
	}
	return &defaultGeneratedWorkflowTitleUpdater{runs: runs}
}

func (u *defaultGeneratedWorkflowTitleUpdater) TryUpdateGeneratedWorkflowTitle(
	ctx context.Context,
	runID,
	expectedTitle,
	generatedTitle string,
) (bool, error) {
	if u == nil || u.runs == nil {
		return false, nil
	}
	runID = strings.TrimSpace(runID)
	expectedTitle = strings.TrimSpace(expectedTitle)
	generatedTitle = strings.TrimSpace(trimTitle(sanitizeTitle(generatedTitle)))
	if runID == "" || generatedTitle == "" {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	run, err := u.runs.GetRun(ctx, runID)
	if err != nil {
		return false, err
	}
	if run == nil {
		return false, nil
	}
	current := strings.TrimSpace(run.TemplateName)
	if expectedTitle != "" && current != expectedTitle {
		return false, nil
	}
	if current == generatedTitle {
		return false, nil
	}
	if _, err := u.runs.RenameRun(ctx, runID, generatedTitle); err != nil {
		return false, err
	}
	return true, nil
}

func titleGenerationWorkerOptionsFromCoreConfig(coreCfg CoreConfigReader) titleGenerationWorkerOptions {
	if coreCfg == nil {
		return titleGenerationWorkerOptions{Timeout: 10 * time.Second, Buffer: 128}
	}
	seconds := coreCfg.TitleGenerationTimeoutSeconds()
	if seconds <= 0 {
		seconds = 10
	}
	return titleGenerationWorkerOptions{Timeout: time.Duration(seconds) * time.Second, Buffer: 128}
}

type CoreConfigReader interface {
	TitleGenerationTimeoutSeconds() int
}

var errTitleGeneratorNotConfigured = errors.New("title generation provider not configured")

func buildTitleGeneratorFromCoreConfig(coreCfg titleGenerationProviderConfigResolver, logger logging.Logger) (titleGenerator, error) {
	bridge := newTitleProviderBridge(logger)
	generator, ok, err := bridge.Build(coreCfg)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errTitleGeneratorNotConfigured
	}
	return generator, nil
}
