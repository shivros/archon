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
	session *types.Session
	meta    *types.SessionMeta
	store   openCodeHistoryReconcilerStore
	logger  logging.Logger
}

type openCodeHistoryReconcilerStore struct {
	readSessionItems   func(sessionID string, lines int) ([]map[string]any, error)
	appendSessionItems func(sessionID string, items []map[string]any) error
}

func newOpenCodeHistoryReconciler(
	session *types.Session,
	meta *types.SessionMeta,
	store openCodeHistoryReconcilerStore,
	logger logging.Logger,
) openCodeHistoryReconciler {
	return openCodeHistoryReconciler{
		session: session,
		meta:    meta,
		store:   store,
		logger:  logger,
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
	if len(items) == 0 {
		return openCodeHistorySyncResult{items: items}, nil
	}
	if r.store.readSessionItems == nil {
		return openCodeHistorySyncResult{items: items}, nil
	}
	localItems, localErr := r.store.readSessionItems(r.session.ID, limit)
	if localErr != nil {
		return openCodeHistorySyncResult{items: items}, nil
	}
	missing := openCodeMissingHistoryItems(localItems, items)
	if len(missing) == 0 {
		return openCodeHistorySyncResult{items: items}, nil
	}
	if r.store.appendSessionItems == nil {
		return openCodeHistorySyncResult{items: items}, nil
	}
	if appendErr := r.store.appendSessionItems(r.session.ID, missing); appendErr != nil {
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
	if r.logger == nil {
		return
	}
	r.logger.Warn(message, fields...)
}

func (r openCodeHistoryReconciler) logDebug(message string, fields ...logging.Field) {
	if r.logger == nil || !r.logger.Enabled(logging.Debug) {
		return
	}
	r.logger.Debug(message, fields...)
}
