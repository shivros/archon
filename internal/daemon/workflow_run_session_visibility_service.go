package daemon

import (
	"context"
	"sort"
	"strings"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/types"
)

type workflowRunSessionVisibilitySyncService struct {
	sessionMeta SessionMetaStore
	logger      logging.Logger
	async       bool
}

func newWorkflowRunSessionVisibilitySyncService(stores *Stores, logger logging.Logger) WorkflowRunSessionVisibilityService {
	if stores == nil || stores.SessionMeta == nil {
		return nil
	}
	return &workflowRunSessionVisibilitySyncService{
		sessionMeta: stores.SessionMeta,
		logger:      logger,
		async:       true,
	}
}

func (s *workflowRunSessionVisibilitySyncService) SyncWorkflowRunSessionVisibility(run *guidedworkflows.WorkflowRun, dismissed bool) {
	if s == nil || run == nil {
		return
	}
	if err := s.syncWorkflowRunPrimarySessionVisibility(context.Background(), run, dismissed); err != nil {
		if s.logger != nil {
			s.logger.Warn("guided_workflow_primary_session_visibility_sync_failed",
				logging.F("run_id", strings.TrimSpace(run.ID)),
				logging.F("run_session_id", strings.TrimSpace(run.SessionID)),
				logging.F("action", workflowVisibilityAction(dismissed)),
				logging.F("error", err),
			)
		}
	}
	runAsync := func(runCopy *guidedworkflows.WorkflowRun, dismissed bool) {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.syncWorkflowLinkedSessionDismissal(timeoutCtx, runCopy, dismissed); err != nil && s.logger != nil {
			s.logger.Warn("guided_workflow_session_visibility_sync_async_failed",
				logging.F("run_id", strings.TrimSpace(runCopy.ID)),
				logging.F("run_session_id", strings.TrimSpace(runCopy.SessionID)),
				logging.F("action", workflowVisibilityAction(dismissed)),
				logging.F("error", err),
			)
		}
	}
	if !s.async {
		runAsync(run, dismissed)
		return
	}
	go runAsync(run, dismissed)
}

func (s *workflowRunSessionVisibilitySyncService) syncWorkflowRunPrimarySessionVisibility(ctx context.Context, run *guidedworkflows.WorkflowRun, dismissed bool) error {
	if s == nil || run == nil || s.sessionMeta == nil {
		return nil
	}
	runID := strings.TrimSpace(run.ID)
	sessionID := strings.TrimSpace(run.SessionID)
	if sessionID == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	existingMeta, ok, err := s.sessionMeta.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	existingWorkflowRunID := ""
	existingDismissed := false
	existingDismissedAt := ""
	if ok && existingMeta != nil {
		existingWorkflowRunID = strings.TrimSpace(existingMeta.WorkflowRunID)
		existingDismissed = existingMeta.DismissedAt != nil
		if existingMeta.DismissedAt != nil {
			existingDismissedAt = existingMeta.DismissedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	if existingWorkflowRunID != "" && runID != "" && existingWorkflowRunID != runID {
		if s.logger != nil {
			s.logger.Info("guided_workflow_primary_session_visibility_sync_skipped",
				logging.F("run_id", runID),
				logging.F("run_session_id", sessionID),
				logging.F("action", workflowVisibilityAction(dismissed)),
				logging.F("existing_workflow_run_id", existingWorkflowRunID),
				logging.F("existing_dismissed", existingDismissed),
				logging.F("existing_dismissed_at", existingDismissedAt),
				logging.F("reason", "workflow_link_mismatch"),
			)
		}
		return nil
	}
	clear := time.Time{}
	dismissedAt := &clear
	if dismissed {
		now := time.Now().UTC()
		dismissedAt = &now
	}
	_, err = s.sessionMeta.Upsert(ctx, &types.SessionMeta{
		SessionID:   sessionID,
		DismissedAt: dismissedAt,
	})
	if err != nil {
		return err
	}
	if s.logger != nil {
		s.logger.Info("guided_workflow_primary_session_visibility_synced",
			logging.F("run_id", runID),
			logging.F("run_session_id", sessionID),
			logging.F("action", workflowVisibilityAction(dismissed)),
			logging.F("existing_workflow_run_id", existingWorkflowRunID),
			logging.F("existing_dismissed", existingDismissed),
			logging.F("existing_dismissed_at", existingDismissedAt),
			logging.F("after_dismissed", dismissed),
		)
	}
	return nil
}

func (s *workflowRunSessionVisibilitySyncService) syncWorkflowLinkedSessionDismissal(ctx context.Context, run *guidedworkflows.WorkflowRun, dismissed bool) error {
	if s == nil || run == nil || s.sessionMeta == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	action := workflowVisibilityAction(dismissed)
	opID := logging.NewRequestID()
	runID := strings.TrimSpace(run.ID)
	runSessionID := strings.TrimSpace(run.SessionID)
	targets := map[string]*workflowSessionVisibilitySyncTarget{}
	addTarget := func(sessionID string, fromRunSession bool, fromWorkflowLink bool, meta *types.SessionMeta) {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return
		}
		target, ok := targets[sessionID]
		if !ok || target == nil {
			target = &workflowSessionVisibilitySyncTarget{sessionID: sessionID}
			targets[sessionID] = target
		}
		if fromRunSession {
			target.fromRunSession = true
		}
		if fromWorkflowLink {
			target.fromWorkflowLink = true
		}
		if target.meta == nil && meta != nil {
			target.meta = meta
		}
	}
	if runSessionID != "" {
		addTarget(runSessionID, true, false, nil)
	}
	var metaBySessionID map[string]*types.SessionMeta
	if runID != "" {
		meta, err := s.sessionMeta.List(ctx)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("guided_workflow_session_visibility_sync_list_failed",
					logging.F("op_id", opID),
					logging.F("run_id", runID),
					logging.F("run_session_id", runSessionID),
					logging.F("action", action),
					logging.F("error", err),
				)
			}
			return err
		}
		metaBySessionID = make(map[string]*types.SessionMeta, len(meta))
		for _, item := range meta {
			if item == nil {
				continue
			}
			sessionID := strings.TrimSpace(item.SessionID)
			if sessionID == "" {
				continue
			}
			metaBySessionID[sessionID] = item
			if strings.TrimSpace(item.WorkflowRunID) != runID {
				continue
			}
			addTarget(sessionID, false, true, item)
		}
	}
	if runSessionID != "" && metaBySessionID != nil {
		if meta := metaBySessionID[runSessionID]; meta != nil {
			addTarget(runSessionID, true, false, meta)
		}
	}
	if len(targets) == 0 {
		if s.logger != nil {
			s.logger.Info("guided_workflow_session_visibility_sync_skipped",
				logging.F("op_id", opID),
				logging.F("run_id", runID),
				logging.F("run_status", strings.TrimSpace(string(run.Status))),
				logging.F("run_session_id", runSessionID),
				logging.F("action", action),
				logging.F("reason", "no_linked_sessions"),
			)
		}
		return nil
	}
	sessionIDs := make([]string, 0, len(targets))
	for sessionID := range targets {
		sessionIDs = append(sessionIDs, sessionID)
	}
	sort.Strings(sessionIDs)
	if s.logger != nil {
		s.logger.Info("guided_workflow_session_visibility_sync_started",
			logging.F("op_id", opID),
			logging.F("run_id", runID),
			logging.F("run_status", strings.TrimSpace(string(run.Status))),
			logging.F("run_session_id", runSessionID),
			logging.F("action", action),
			logging.F("targets", len(sessionIDs)),
			logging.F("target_session_ids", sessionIDs),
		)
	}
	clear := time.Time{}
	dismissedAt := &clear
	if dismissed {
		now := time.Now().UTC()
		dismissedAt = &now
	}
	syncedCount := 0
	skippedCount := 0
	for _, sessionID := range sessionIDs {
		target := targets[sessionID]
		targetSource := ""
		if target != nil {
			targetSource = target.source()
		}
		currentMeta, currentMetaOK, err := s.sessionMeta.Get(ctx, sessionID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("guided_workflow_session_visibility_sync_get_failed",
					logging.F("op_id", opID),
					logging.F("run_id", runID),
					logging.F("run_status", strings.TrimSpace(string(run.Status))),
					logging.F("run_session_id", runSessionID),
					logging.F("action", action),
					logging.F("session_id", sessionID),
					logging.F("target_source", targetSource),
					logging.F("error", err),
				)
			}
			return err
		}
		currentWorkflowRunID := ""
		beforeDismissed := false
		beforeDismissedAt := ""
		if currentMetaOK && currentMeta != nil {
			currentWorkflowRunID = strings.TrimSpace(currentMeta.WorkflowRunID)
			beforeDismissed = currentMeta.DismissedAt != nil
			if currentMeta.DismissedAt != nil {
				beforeDismissedAt = currentMeta.DismissedAt.UTC().Format(time.RFC3339Nano)
			}
		}
		ownsByRunID := runID != "" && currentWorkflowRunID == runID
		ownsByPrimarySession := sessionID == runSessionID && currentWorkflowRunID == ""
		if !ownsByRunID && !ownsByPrimarySession {
			skippedCount++
			if s.logger != nil {
				s.logger.Info("guided_workflow_session_visibility_sync_skipped_target",
					logging.F("op_id", opID),
					logging.F("run_id", runID),
					logging.F("run_status", strings.TrimSpace(string(run.Status))),
					logging.F("run_session_id", runSessionID),
					logging.F("action", action),
					logging.F("session_id", sessionID),
					logging.F("target_source", targetSource),
					logging.F("before_dismissed", beforeDismissed),
					logging.F("before_dismissed_at", beforeDismissedAt),
					logging.F("before_workflow_run_id", currentWorkflowRunID),
					logging.F("reason", "workflow_link_mismatch"),
				)
			}
			continue
		}
		if _, err := s.sessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:   sessionID,
			DismissedAt: dismissedAt,
		}); err != nil {
			if s.logger != nil {
				s.logger.Warn("guided_workflow_session_visibility_sync_failed",
					logging.F("op_id", opID),
					logging.F("run_id", runID),
					logging.F("run_status", strings.TrimSpace(string(run.Status))),
					logging.F("run_session_id", runSessionID),
					logging.F("action", action),
					logging.F("session_id", sessionID),
					logging.F("target_source", targetSource),
					logging.F("before_dismissed", beforeDismissed),
					logging.F("before_dismissed_at", beforeDismissedAt),
					logging.F("before_workflow_run_id", currentWorkflowRunID),
					logging.F("error", err),
				)
			}
			return err
		}
		if s.logger != nil {
			s.logger.Info("guided_workflow_session_visibility_sync_applied",
				logging.F("op_id", opID),
				logging.F("run_id", runID),
				logging.F("run_status", strings.TrimSpace(string(run.Status))),
				logging.F("run_session_id", runSessionID),
				logging.F("action", action),
				logging.F("session_id", sessionID),
				logging.F("target_source", targetSource),
				logging.F("before_dismissed", beforeDismissed),
				logging.F("before_dismissed_at", beforeDismissedAt),
				logging.F("before_workflow_run_id", currentWorkflowRunID),
				logging.F("after_dismissed", dismissed),
			)
		}
		syncedCount++
	}
	if s.logger != nil {
		s.logger.Info("guided_workflow_session_visibility_sync_completed",
			logging.F("op_id", opID),
			logging.F("run_id", runID),
			logging.F("run_status", strings.TrimSpace(string(run.Status))),
			logging.F("run_session_id", runSessionID),
			logging.F("action", action),
			logging.F("targets", len(sessionIDs)),
			logging.F("synced", syncedCount),
			logging.F("skipped", skippedCount),
		)
	}
	return nil
}

type workflowSessionVisibilitySyncTarget struct {
	sessionID        string
	fromRunSession   bool
	fromWorkflowLink bool
	meta             *types.SessionMeta
}

func (t *workflowSessionVisibilitySyncTarget) source() string {
	if t == nil {
		return ""
	}
	if t.fromRunSession && t.fromWorkflowLink {
		return "run_session+workflow_link"
	}
	if t.fromRunSession {
		return "run_session"
	}
	if t.fromWorkflowLink {
		return "workflow_link"
	}
	return "unknown"
}

func workflowVisibilityAction(dismissed bool) string {
	if dismissed {
		return "dismiss"
	}
	return "undismiss"
}
