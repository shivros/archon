package daemon

import (
	"context"
	"encoding/json"
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

func (r openCodeHistoryReconciler) ReconcileBestEffort(ctx context.Context, reason string) {
	if r.session == nil {
		return
	}
	reconcileCtx := ctx
	if reconcileCtx == nil {
		reconcileCtx = context.Background()
	}
	if _, hasDeadline := reconcileCtx.Deadline(); !hasDeadline {
		timeoutCtx, cancel := context.WithTimeout(reconcileCtx, 5*time.Second)
		defer cancel()
		reconcileCtx = timeoutCtx
	}
	opID := logging.NewRequestID()
	start := time.Now()
	r.logDebug("opencode_history_reconcile_start",
		append(
			openCodeSessionLogFields(r.session, r.meta),
			logging.F("op_id", opID),
			logging.F("reason", reason),
		)...,
	)
	result, err := r.Sync(reconcileCtx, 200)
	if err != nil {
		r.logWarn("opencode_history_reconcile_failed",
			append(
				append(
					openCodeSessionLogFields(r.session, r.meta),
					logging.F("op_id", opID),
					logging.F("reason", reason),
					logging.F("duration_ms", time.Since(start).Milliseconds()),
				),
				openCodeErrorLogFields(err)...,
			)...,
		)
		return
	}
	if len(result.backfilled) > 0 {
		r.logInfo("opencode_history_reconciled",
			append(
				openCodeSessionLogFields(r.session, r.meta),
				logging.F("op_id", opID),
				logging.F("reason", reason),
				logging.F("items", len(result.backfilled)),
				logging.F("duration_ms", time.Since(start).Milliseconds()),
			)...,
		)
		return
	}
	r.logDebug("opencode_history_reconcile_noop",
		append(
			openCodeSessionLogFields(r.session, r.meta),
			logging.F("op_id", opID),
			logging.F("reason", reason),
			logging.F("duration_ms", time.Since(start).Milliseconds()),
		)...,
	)
}

func (r openCodeHistoryReconciler) RecoveredEvents(ctx context.Context, sawTurnCompleted bool) []types.CodexEvent {
	reconcileCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := r.Sync(reconcileCtx, 200)
	if err != nil || len(result.backfilled) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	events := openCodeHistoryItemsToEvents(result.backfilled, now)
	if len(events) == 0 {
		return nil
	}
	if !sawTurnCompleted {
		turnParams, _ := json.Marshal(map[string]any{
			"turn": map[string]any{
				"status": "completed",
			},
		})
		events = append(events, types.CodexEvent{
			Method: "turn/completed",
			TS:     now,
			Params: turnParams,
		})
	}
	return events
}

func (r openCodeHistoryReconciler) logWarn(message string, fields ...logging.Field) {
	if r.service == nil || r.service.logger == nil {
		return
	}
	r.service.logger.Warn(message, fields...)
}

func (r openCodeHistoryReconciler) logInfo(message string, fields ...logging.Field) {
	if r.service == nil || r.service.logger == nil {
		return
	}
	r.service.logger.Info(message, fields...)
}

func (r openCodeHistoryReconciler) logDebug(message string, fields ...logging.Field) {
	if r.service == nil || r.service.logger == nil || !r.service.logger.Enabled(logging.Debug) {
		return
	}
	r.service.logger.Debug(message, fields...)
}

func openCodeHistoryItemsToEvents(items []map[string]any, ts string) []types.CodexEvent {
	if len(items) == 0 {
		return nil
	}
	events := make([]types.CodexEvent, 0, len(items)*3)
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(asString(item["type"]))) != "assistant" {
			continue
		}
		text := strings.TrimSpace(openCodeHistoryItemText(item))
		if text == "" {
			continue
		}
		itemID := strings.TrimSpace(asString(item["provider_message_id"]))
		if itemID == "" {
			itemID = "recovered_" + sanitizeIDComponent(text)
		}
		payloadItem := map[string]any{
			"id":   itemID,
			"type": "agentMessage",
			"text": text,
		}
		startedParams, _ := json.Marshal(map[string]any{"item": payloadItem})
		deltaParams, _ := json.Marshal(map[string]any{"delta": text})
		completedParams, _ := json.Marshal(map[string]any{"item": payloadItem})
		events = append(events,
			types.CodexEvent{Method: "item/started", TS: ts, Params: startedParams},
			types.CodexEvent{Method: "item/agentMessage/delta", TS: ts, Params: deltaParams},
			types.CodexEvent{Method: "item/completed", TS: ts, Params: completedParams},
		)
	}
	return events
}

func sanitizeIDComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "assistant"
	}
	builder := strings.Builder{}
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
		if builder.Len() >= 32 {
			break
		}
	}
	out := strings.Trim(builder.String(), "_")
	if out == "" {
		return "assistant"
	}
	return out
}
