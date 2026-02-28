package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"control/internal/logging"
	"control/internal/types"
)

type ClaudeDebugWriter interface {
	WriteSessionDebug(id, stream string, data []byte) error
}

type claudeTurnExecutor interface {
	ExecutePreparedTurn(
		ctx context.Context,
		sendCtx claudeSendContext,
		session *types.Session,
		meta *types.SessionMeta,
		prepared claudePreparedTurn,
	) error
}

type ClaudeTurnFailureReporter interface {
	Report(job claudeTurnJob, turnErr error)
}

type ClaudeTurnScheduler interface {
	Enqueue(job claudeTurnJob) error
	Close()
}

type claudeTurnJob struct {
	sendCtx  claudeSendContext
	session  *types.Session
	meta     *types.SessionMeta
	prepared claudePreparedTurn
}

type claudeTurnScheduler struct {
	mu              sync.Mutex
	queue           chan claudeTurnJob
	executor        claudeTurnExecutor
	failureReporter ClaudeTurnFailureReporter
	onStart         func(turnID string)
	onDone          func(turnID string)
	closed          bool
}

func newClaudeTurnScheduler(
	queueSize int,
	executor claudeTurnExecutor,
	failureReporter ClaudeTurnFailureReporter,
	onStart func(turnID string),
	onDone func(turnID string),
) *claudeTurnScheduler {
	if queueSize <= 0 {
		queueSize = 1
	}
	s := &claudeTurnScheduler{
		queue:           make(chan claudeTurnJob, queueSize),
		executor:        executor,
		failureReporter: failureReporter,
		onStart:         onStart,
		onDone:          onDone,
	}
	go s.run()
	return s
}

func (s *claudeTurnScheduler) Enqueue(job claudeTurnJob) error {
	if s == nil {
		return unavailableError("claude turn scheduler is not available", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return invalidError("session is closed", nil)
	}
	select {
	case s.queue <- job:
		return nil
	default:
		return unavailableError("claude turn queue is full; retry send", nil)
	}
}

func (s *claudeTurnScheduler) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	queue := s.queue
	s.queue = nil
	s.mu.Unlock()
	if queue != nil {
		close(queue)
	}
}

func (s *claudeTurnScheduler) run() {
	if s == nil {
		return
	}
	for job := range s.queue {
		turnID := strings.TrimSpace(job.prepared.TurnID)
		if turnID != "" && s.onStart != nil {
			s.onStart(turnID)
		}
		if s.executor != nil {
			err := s.executor.ExecutePreparedTurn(context.Background(), job.sendCtx, job.session, job.meta, job.prepared)
			if err != nil && s.failureReporter != nil {
				s.failureReporter.Report(job, err)
			}
		}
		if s.onDone != nil {
			s.onDone(turnID)
		}
	}
}

type defaultClaudeTurnFailureReporter struct {
	sessionID    string
	providerName string
	logger       logging.Logger
	debugWriter  ClaudeDebugWriter
	repository   TurnArtifactRepository
	notifier     TurnCompletionNotifier
}

func (r defaultClaudeTurnFailureReporter) Report(job claudeTurnJob, turnErr error) {
	if turnErr == nil {
		return
	}
	sessionID := strings.TrimSpace(r.sessionID)
	if job.session != nil && strings.TrimSpace(job.session.ID) != "" {
		sessionID = strings.TrimSpace(job.session.ID)
	}
	provider := strings.TrimSpace(r.providerName)
	if provider == "" {
		provider = "claude"
	}
	if job.session != nil {
		if p := strings.TrimSpace(job.session.Provider); p != "" {
			provider = p
		}
	}
	turnID := strings.TrimSpace(job.prepared.TurnID)
	message := fmt.Sprintf("Claude turn failed (%s): %v", turnID, turnErr)

	if r.logger != nil {
		r.logger.Error("claude_turn_failed_async",
			logging.F("session_id", sessionID),
			logging.F("turn_id", turnID),
			logging.F("provider", provider),
			logging.F("error", turnErr),
		)
	}
	if r.debugWriter != nil {
		_ = r.debugWriter.WriteSessionDebug(sessionID, "stderr", []byte(message+"\n"))
	}
	if r.repository != nil {
		_ = r.repository.AppendItems(sessionID, []map[string]any{
			{
				"type":    "log",
				"turn_id": turnID,
				"text":    message,
			},
		})
	}
	if r.notifier == nil {
		return
	}
	event := TurnCompletionEvent{
		SessionID: sessionID,
		TurnID:    turnID,
		Provider:  provider,
		Source:    "claude_async_send_failed",
		Status:    "failed",
		Error:     strings.TrimSpace(turnErr.Error()),
		Payload: map[string]any{
			"turn_status": "failed",
			"turn_error":  strings.TrimSpace(turnErr.Error()),
		},
	}
	if job.meta != nil {
		event.WorkspaceID = strings.TrimSpace(job.meta.WorkspaceID)
		event.WorktreeID = strings.TrimSpace(job.meta.WorktreeID)
	}
	r.notifier.NotifyTurnCompletedEvent(context.Background(), event)
}
