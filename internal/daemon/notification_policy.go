package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"control/internal/config"
	"control/internal/logging"
	"control/internal/types"
)

const defaultNotificationLookupTimeout = 750 * time.Millisecond

type defaultNotificationPolicyResolver struct {
	defaults      types.NotificationSettings
	stores        *Stores
	logger        logging.Logger
	lookupTimeout time.Duration
}

func NewNotificationPolicyResolver(defaults types.NotificationSettings, stores *Stores, logger logging.Logger) NotificationPolicyResolver {
	if logger == nil {
		logger = logging.Nop()
	}
	return &defaultNotificationPolicyResolver{
		defaults:      types.NormalizeNotificationSettings(defaults),
		stores:        stores,
		logger:        logger,
		lookupTimeout: defaultNotificationLookupTimeout,
	}
}

func (r *defaultNotificationPolicyResolver) Resolve(ctx context.Context, event types.NotificationEvent) types.NotificationSettings {
	settings := types.CloneNotificationSettings(r.defaults)
	if r == nil || r.stores == nil {
		return settings
	}
	lookupCtx, cancel := r.lookupContext(ctx)
	defer cancel()

	workspaceID := strings.TrimSpace(event.WorkspaceID)
	worktreeID := strings.TrimSpace(event.WorktreeID)
	var sessionPatch *types.NotificationSettingsPatch
	if sessionID := strings.TrimSpace(event.SessionID); sessionID != "" && r.stores.SessionMeta != nil {
		meta, ok, err := r.stores.SessionMeta.Get(lookupCtx, sessionID)
		if err != nil {
			r.logResolveError("notification_resolve_session_meta_failed", err,
				logging.F("session_id", sessionID),
			)
		} else if ok && meta != nil {
			if workspaceID == "" {
				workspaceID = strings.TrimSpace(meta.WorkspaceID)
			}
			if worktreeID == "" {
				worktreeID = strings.TrimSpace(meta.WorktreeID)
			}
			sessionPatch = meta.NotificationOverrides
		}
	}

	if workspaceID != "" && worktreeID != "" && r.stores.Worktrees != nil {
		worktrees, err := r.stores.Worktrees.ListWorktrees(lookupCtx, workspaceID)
		if err != nil {
			r.logResolveError("notification_resolve_worktree_failed", err,
				logging.F("workspace_id", workspaceID),
				logging.F("worktree_id", worktreeID),
			)
		} else {
			for _, worktree := range worktrees {
				if worktree != nil && strings.TrimSpace(worktree.ID) == worktreeID {
					settings = types.MergeNotificationSettings(settings, worktree.NotificationOverrides)
					break
				}
			}
		}
	}

	settings = types.MergeNotificationSettings(settings, sessionPatch)
	return settings
}

func (r *defaultNotificationPolicyResolver) lookupContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := r.lookupTimeout
	if timeout <= 0 {
		timeout = defaultNotificationLookupTimeout
	}
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}

func (r *defaultNotificationPolicyResolver) logResolveError(msg string, err error, fields ...logging.Field) {
	if r == nil || r.logger == nil || err == nil {
		return
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		r.logger.Debug(msg, append(fields, logging.F("error", err))...)
		return
	}
	r.logger.Warn(msg, append(fields, logging.F("error", err))...)
}

func notificationDefaultsFromCoreConfig(cfg config.CoreConfig) types.NotificationSettings {
	out := types.DefaultNotificationSettings()
	out.Enabled = cfg.NotificationsEnabled()
	out.Triggers = nil
	for _, raw := range cfg.NotificationTriggers() {
		trigger, ok := types.NormalizeNotificationTrigger(raw)
		if ok {
			out.Triggers = append(out.Triggers, trigger)
		}
	}
	out.Methods = nil
	for _, raw := range cfg.NotificationMethods() {
		method, ok := types.NormalizeNotificationMethod(raw)
		if ok {
			out.Methods = append(out.Methods, method)
		}
	}
	out.ScriptCommands = cfg.NotificationScriptCommands()
	out.ScriptTimeoutSeconds = cfg.NotificationScriptTimeoutSeconds()
	out.DedupeWindowSeconds = cfg.NotificationDedupeWindowSeconds()
	return types.NormalizeNotificationSettings(out)
}
