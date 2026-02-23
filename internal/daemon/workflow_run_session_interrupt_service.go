package daemon

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/types"
)

type workflowRunSessionInterruptGateway interface {
	Get(ctx context.Context, id string) (*types.Session, error)
	InterruptTurn(ctx context.Context, id string) error
}

type workflowRunSessionTargetResolver interface {
	ResolveWorkflowRunSessionIDs(ctx context.Context, run *guidedworkflows.WorkflowRun) []string
}

type workflowRunSessionInterruptExecutor interface {
	InterruptSessions(ctx context.Context, runID string, sessionIDs []string) (workflowRunSessionInterruptExecution, error)
}

type workflowRunSessionInterruptExecution struct {
	Interrupted int
	Skipped     int
}

type workflowRunSessionInterruptService struct {
	resolver workflowRunSessionTargetResolver
	executor workflowRunSessionInterruptExecutor
	logger   logging.Logger
}

func newWorkflowRunSessionInterruptService(
	manager *SessionManager,
	stores *Stores,
	live *CodexLiveManager,
	logger logging.Logger,
) WorkflowRunSessionInterruptService {
	if manager == nil {
		return nil
	}
	sessionService := NewSessionService(manager, stores, live, logger)
	if sessionService == nil {
		return nil
	}
	var sessionMeta SessionMetaStore
	if stores != nil {
		sessionMeta = stores.SessionMeta
	}
	return &workflowRunSessionInterruptService{
		resolver: &workflowRunSessionTargetResolverService{
			sessionMeta: sessionMeta,
		},
		executor: &workflowRunSessionInterruptExecutorService{
			sessions: sessionService,
			logger:   logger,
			timeout:  5 * time.Second,
		},
		logger: logger,
	}
}

func (s *workflowRunSessionInterruptService) InterruptWorkflowRunSessions(
	ctx context.Context,
	run *guidedworkflows.WorkflowRun,
) error {
	if s == nil || s.resolver == nil || s.executor == nil || run == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runID := strings.TrimSpace(run.ID)
	if runID == "" {
		return nil
	}
	sessionIDs := s.resolver.ResolveWorkflowRunSessionIDs(ctx, run)
	if len(sessionIDs) == 0 {
		return nil
	}
	sort.Strings(sessionIDs)

	if s.logger != nil {
		s.logger.Info("guided_workflow_run_session_interrupt_started",
			logging.F("run_id", runID),
			logging.F("run_status", strings.TrimSpace(string(run.Status))),
			logging.F("targets", len(sessionIDs)),
			logging.F("target_session_ids", sessionIDs),
		)
	}

	execution, err := s.executor.InterruptSessions(ctx, runID, sessionIDs)
	if s.logger != nil {
		s.logger.Info("guided_workflow_run_session_interrupt_completed",
			logging.F("run_id", runID),
			logging.F("targets", len(sessionIDs)),
			logging.F("interrupted", execution.Interrupted),
			logging.F("skipped", execution.Skipped),
			logging.F("has_errors", err != nil),
		)
	}
	return err
}

type workflowRunSessionTargetResolverService struct {
	sessionMeta SessionMetaStore
}

func (s *workflowRunSessionTargetResolverService) ResolveWorkflowRunSessionIDs(
	ctx context.Context,
	run *guidedworkflows.WorkflowRun,
) []string {
	if s == nil || run == nil {
		return nil
	}
	runID := strings.TrimSpace(run.ID)
	if runID == "" {
		return nil
	}
	ids := map[string]struct{}{}
	add := func(sessionID string) {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return
		}
		ids[sessionID] = struct{}{}
	}

	add(run.SessionID)
	for _, phase := range run.Phases {
		for _, step := range phase.Steps {
			if step.Execution != nil {
				add(step.Execution.SessionID)
			}
			for _, attempt := range step.ExecutionAttempts {
				add(attempt.SessionID)
			}
		}
	}
	if s.sessionMeta != nil {
		meta, err := s.sessionMeta.List(ctx)
		if err == nil {
			for _, item := range meta {
				if item == nil || strings.TrimSpace(item.WorkflowRunID) != runID {
					continue
				}
				add(item.SessionID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for sessionID := range ids {
		out = append(out, sessionID)
	}
	return out
}

type workflowRunSessionInterruptExecutorService struct {
	sessions workflowRunSessionInterruptGateway
	logger   logging.Logger
	timeout  time.Duration
}

func (s *workflowRunSessionInterruptExecutorService) InterruptSessions(
	ctx context.Context,
	runID string,
	sessionIDs []string,
) (workflowRunSessionInterruptExecution, error) {
	result := workflowRunSessionInterruptExecution{}
	if s == nil || s.sessions == nil || len(sessionIDs) == 0 {
		result.Skipped = len(sessionIDs)
		return result, nil
	}
	var joined error
	for _, sessionID := range sessionIDs {
		session, err := s.sessions.Get(ctx, sessionID)
		if err != nil {
			if isWorkflowRunSessionInterruptNotFound(err) {
				result.Skipped++
				continue
			}
			joined = errors.Join(joined, err)
			if s.logger != nil {
				s.logger.Warn("guided_workflow_run_session_interrupt_lookup_failed",
					logging.F("run_id", runID),
					logging.F("session_id", sessionID),
					logging.F("error", err),
				)
			}
			continue
		}
		if session == nil || !isWorkflowRunSessionInterruptActive(session.Status) {
			result.Skipped++
			continue
		}
		interruptCtx := ctx
		cancel := func() {}
		if s.timeout > 0 {
			interruptCtx, cancel = context.WithTimeout(ctx, s.timeout)
		}
		err = s.sessions.InterruptTurn(interruptCtx, sessionID)
		cancel()
		if err != nil {
			joined = errors.Join(joined, err)
			if s.logger != nil {
				s.logger.Warn("guided_workflow_run_session_interrupt_failed",
					logging.F("run_id", runID),
					logging.F("session_id", sessionID),
					logging.F("provider", strings.TrimSpace(session.Provider)),
					logging.F("status", strings.TrimSpace(string(session.Status))),
					logging.F("error", err),
				)
			}
			continue
		}
		result.Interrupted++
		if s.logger != nil {
			s.logger.Info("guided_workflow_run_session_interrupted",
				logging.F("run_id", runID),
				logging.F("session_id", sessionID),
				logging.F("provider", strings.TrimSpace(session.Provider)),
				logging.F("status", strings.TrimSpace(string(session.Status))),
			)
		}
	}
	return result, joined
}

func isWorkflowRunSessionInterruptNotFound(err error) bool {
	if errors.Is(err, ErrSessionNotFound) {
		return true
	}
	var svcErr *ServiceError
	return errors.As(err, &svcErr) && svcErr != nil && svcErr.Kind == ServiceErrorNotFound
}

func isWorkflowRunSessionInterruptActive(status types.SessionStatus) bool {
	switch status {
	case types.SessionStatusCreated, types.SessionStatusStarting, types.SessionStatusRunning:
		return true
	default:
		return false
	}
}
