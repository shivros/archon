package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

const defaultSessionLifecycleLookupTimeout = 500 * time.Millisecond

type defaultSessionLifecycleEmitter struct {
	notifier NotificationPublisher
	meta     SessionMetaStore
	logger   logging.Logger
}

func NewSessionLifecycleEmitter(notifier NotificationPublisher, meta SessionMetaStore, logger logging.Logger) SessionLifecycleEmitter {
	if notifier == nil {
		return nil
	}
	if logger == nil {
		logger = logging.Nop()
	}
	return &defaultSessionLifecycleEmitter{
		notifier: notifier,
		meta:     meta,
		logger:   logger,
	}
}

func (e *defaultSessionLifecycleEmitter) EmitSessionLifecycleEvent(ctx context.Context, session *types.Session, cfg StartSessionConfig, status types.SessionStatus, source string) {
	if e == nil || session == nil || e.notifier == nil {
		return
	}
	trigger, ok := notificationTriggerForStatus(status)
	if !ok {
		return
	}
	event := notificationEventFromSession(session, trigger, source)
	workspaceID := strings.TrimSpace(cfg.WorkspaceID)
	worktreeID := strings.TrimSpace(cfg.WorktreeID)
	if e.meta != nil && (workspaceID == "" || worktreeID == "") {
		if ctx == nil {
			ctx = context.Background()
		}
		lookupCtx, cancel := context.WithTimeout(ctx, defaultSessionLifecycleLookupTimeout)
		meta, ok, err := e.meta.Get(lookupCtx, session.ID)
		cancel()
		if err == nil && ok && meta != nil {
			if workspaceID == "" {
				workspaceID = strings.TrimSpace(meta.WorkspaceID)
			}
			if worktreeID == "" {
				worktreeID = strings.TrimSpace(meta.WorktreeID)
			}
		} else if err != nil && e.logger != nil {
			e.logger.Debug("notification_session_lifecycle_meta_lookup_failed",
				logging.F("session_id", session.ID),
				logging.F("error", err),
			)
		}
	}
	event.WorkspaceID = workspaceID
	event.WorktreeID = worktreeID
	e.notifier.Publish(event)
}

func notificationDedupeKey(event types.NotificationEvent) string {
	parts := []string{string(event.Trigger), strings.TrimSpace(event.SessionID)}
	if strings.TrimSpace(event.TurnID) != "" {
		parts = append(parts, strings.TrimSpace(event.TurnID))
	} else {
		parts = append(parts, strings.TrimSpace(event.Status), strings.TrimSpace(event.Source))
	}
	return strings.Join(parts, "|")
}

func notificationTitleBody(event types.NotificationEvent) (string, string) {
	name := strings.TrimSpace(event.Title)
	if name == "" {
		name = strings.TrimSpace(event.SessionID)
	}
	if name == "" {
		name = "session"
	}
	provider := strings.TrimSpace(event.Provider)
	if provider == "" {
		provider = "unknown"
	}
	summary := "Archon"
	body := ""
	switch event.Trigger {
	case types.NotificationTriggerTurnCompleted:
		summary = "Archon turn completed"
		body = name + " (" + provider + ")"
	case types.NotificationTriggerSessionFailed:
		summary = "Archon session failed"
		body = name + " (" + provider + ")"
	case types.NotificationTriggerSessionKilled:
		summary = "Archon session killed"
		body = name + " (" + provider + ")"
	case types.NotificationTriggerSessionExited:
		summary = "Archon session exited"
		body = name + " (" + provider + ")"
	default:
		summary = "Archon notification"
		body = name + " (" + provider + ")"
	}
	if status := strings.TrimSpace(event.Status); status != "" {
		body = body + " - status: " + status
	}
	return summary, body
}

func notificationScriptEnv(event types.NotificationEvent) []string {
	return []string{
		"ARCHON_EVENT=" + string(event.Trigger),
		"ARCHON_SESSION_ID=" + strings.TrimSpace(event.SessionID),
		"ARCHON_WORKSPACE_ID=" + strings.TrimSpace(event.WorkspaceID),
		"ARCHON_WORKTREE_ID=" + strings.TrimSpace(event.WorktreeID),
		"ARCHON_PROVIDER=" + strings.TrimSpace(event.Provider),
		"ARCHON_STATUS=" + strings.TrimSpace(event.Status),
		"ARCHON_TURN_ID=" + strings.TrimSpace(event.TurnID),
		"ARCHON_CWD=" + strings.TrimSpace(event.Cwd),
		"ARCHON_NOTIFICATION_AT=" + strings.TrimSpace(event.OccurredAt),
	}
}

func normalizeNotificationEvent(event types.NotificationEvent) types.NotificationEvent {
	if event.OccurredAt == "" {
		event.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if normalized, ok := types.NormalizeNotificationTrigger(string(event.Trigger)); ok {
		event.Trigger = normalized
	}
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.WorkspaceID = strings.TrimSpace(event.WorkspaceID)
	event.WorktreeID = strings.TrimSpace(event.WorktreeID)
	event.Provider = strings.TrimSpace(event.Provider)
	event.Title = strings.TrimSpace(event.Title)
	event.Status = strings.TrimSpace(event.Status)
	event.TurnID = strings.TrimSpace(event.TurnID)
	event.Cwd = strings.TrimSpace(event.Cwd)
	event.Source = strings.TrimSpace(event.Source)
	return event
}

func notificationTriggerForStatus(status types.SessionStatus) (types.NotificationTrigger, bool) {
	switch status {
	case types.SessionStatusExited:
		return types.NotificationTriggerSessionExited, true
	case types.SessionStatusFailed:
		return types.NotificationTriggerSessionFailed, true
	case types.SessionStatusKilled:
		return types.NotificationTriggerSessionKilled, true
	default:
		return "", false
	}
}

func notificationEventFromSession(session *types.Session, trigger types.NotificationTrigger, source string) types.NotificationEvent {
	event := types.NotificationEvent{
		Trigger:    trigger,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		Source:     strings.TrimSpace(source),
	}
	if session == nil {
		return event
	}
	event.SessionID = strings.TrimSpace(session.ID)
	event.Provider = strings.TrimSpace(session.Provider)
	event.Title = strings.TrimSpace(session.Title)
	event.Cwd = strings.TrimSpace(session.Cwd)
	event.Status = strings.TrimSpace(string(session.Status))
	return event
}

func parseTurnIDFromEventParams(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if turn, ok := payload["turn"].(map[string]any); ok {
		if id := strings.TrimSpace(asString(turn["id"])); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(asString(payload["turn_id"])); id != "" {
		return id
	}
	return ""
}
