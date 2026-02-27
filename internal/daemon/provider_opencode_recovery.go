package daemon

import (
	"context"
	"strings"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

type openCodeHistorySyncResult struct {
	items      []map[string]any
	backfilled []map[string]any
}

type openCodeHistoryReconciler struct {
	service *SessionService
	session *types.Session
	meta    *types.SessionMeta
}

func newOpenCodeHistoryReconciler(service *SessionService, session *types.Session, meta *types.SessionMeta) openCodeHistoryReconciler {
	return openCodeHistoryReconciler{
		service: service,
		session: session,
		meta:    meta,
	}
}

func (r openCodeHistoryReconciler) Sync(ctx context.Context, lines int) (openCodeHistorySyncResult, error) {
	start := time.Now()
	if r.session == nil {
		return openCodeHistorySyncResult{}, invalidError("session is required", nil)
	}
	providerSessionID := ""
	if r.meta != nil {
		providerSessionID = strings.TrimSpace(r.meta.ProviderSessionID)
	}
	if providerSessionID == "" {
		return openCodeHistorySyncResult{}, invalidError("provider session id not available", nil)
	}
	client, err := newOpenCodeClient(resolveOpenCodeClientConfig(r.session.Provider, loadCoreConfigOrDefault()))
	if err != nil {
		return openCodeHistorySyncResult{}, err
	}
	limit := lines
	if limit < 200 {
		limit = 200
	}
	directory := strings.TrimSpace(r.session.Cwd)
	messages, fetchErr := client.ListSessionMessages(ctx, providerSessionID, directory, limit)
	if fetchErr != nil && directory != "" {
		// Fallback for servers that reject directory scoping on history reads.
		messages, fetchErr = client.ListSessionMessages(ctx, providerSessionID, "", limit)
	}
	if fetchErr != nil {
		r.logWarn("opencode_history_sync_failed",
			append(
				append(
					openCodeSessionLogFields(r.session, r.meta),
					logging.F("requested_lines", lines),
					logging.F("limit", limit),
					logging.F("duration_ms", time.Since(start).Milliseconds()),
				),
				openCodeErrorLogFields(fetchErr)...,
			)...,
		)
		return openCodeHistorySyncResult{}, fetchErr
	}

	items := trimItemsToLimit(openCodeSessionMessagesToItems(messages), lines)
	if len(items) == 0 || r.service == nil {
		return openCodeHistorySyncResult{items: items}, nil
	}
	localItems, _, localErr := r.service.readSessionItems(r.session.ID, limit)
	if localErr != nil {
		return openCodeHistorySyncResult{items: items}, nil
	}
	missing := openCodeMissingHistoryItems(localItems, items)
	if len(missing) == 0 {
		return openCodeHistorySyncResult{items: items}, nil
	}
	if appendErr := r.service.appendSessionItems(r.session.ID, missing); appendErr != nil {
		r.logWarn("opencode_history_backfill_failed",
			append(
				append(
					openCodeSessionLogFields(r.session, r.meta),
					logging.F("missing_items", len(missing)),
					logging.F("duration_ms", time.Since(start).Milliseconds()),
				),
				openCodeErrorLogFields(appendErr)...,
			)...,
		)
		return openCodeHistorySyncResult{items: items}, nil
	}
	r.logDebug("opencode_history_sync_ok",
		append(
			openCodeSessionLogFields(r.session, r.meta),
			logging.F("requested_lines", lines),
			logging.F("returned_items", len(items)),
			logging.F("backfilled_items", len(missing)),
			logging.F("duration_ms", time.Since(start).Milliseconds()),
		)...,
	)
	return openCodeHistorySyncResult{
		items:      items,
		backfilled: missing,
	}, nil
}

func (r openCodeHistoryReconciler) logWarn(message string, fields ...logging.Field) {
	if r.service == nil || r.service.logger == nil {
		return
	}
	r.service.logger.Warn(message, fields...)
}

func (r openCodeHistoryReconciler) logDebug(message string, fields ...logging.Field) {
	if r.service == nil || r.service.logger == nil || !r.service.logger.Enabled(logging.Debug) {
		return
	}
	r.service.logger.Debug(message, fields...)
}
