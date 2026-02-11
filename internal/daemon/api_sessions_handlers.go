package daemon

import (
	"encoding/json"
	"net/http"
	"strings"

	"control/internal/logging"
	"control/internal/types"
)

func (a *API) Sessions(w http.ResponseWriter, r *http.Request) {
	service := NewSessionService(a.Manager, a.Stores, a.LiveCodex, a.Logger)

	switch r.Method {
	case http.MethodGet:
		if isRefreshRequest(r) && a.Syncer != nil {
			var err error
			workspaceID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
			if workspaceID != "" {
				err = a.Syncer.SyncWorkspace(r.Context(), workspaceID)
			} else {
				err = a.Syncer.SyncAll(r.Context())
			}
			if err != nil {
				writeServiceError(w, unavailableError("session sync failed", err))
				return
			}
		}
		includeDismissed := parseBoolQueryValue(r.URL.Query().Get("include_dismissed"))
		var (
			sessions []*types.Session
			meta     []*types.SessionMeta
			err      error
		)
		if includeDismissed {
			sessions, meta, err = service.ListWithMetaIncludingDismissed(r.Context())
		} else {
			sessions, meta, err = service.ListWithMeta(r.Context())
		}
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"sessions":     sessions,
			"session_meta": meta,
		})
		return
	case http.MethodPost:
		var req StartSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid json body",
			})
			return
		}
		session, err := service.Start(r.Context(), req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, session)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
			"error": "method not allowed",
		})
	}
}

func (a *API) SessionByID(w http.ResponseWriter, r *http.Request) {
	service := NewSessionService(a.Manager, a.Stores, a.LiveCodex, a.Logger)

	path := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := parts[0]

	if len(parts) == 1 {
		if r.Method == http.MethodGet {
			session, err := service.Get(r.Context(), id)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, session)
			return
		}
		if r.Method == http.MethodPatch {
			var req UpdateSessionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "invalid json body",
				})
				return
			}
			if err := service.Update(r.Context(), id, req); err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
	}

	switch parts[1] {
	case "dismiss":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		if err := service.Dismiss(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case "undismiss":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		if err := service.Undismiss(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case "kill":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		if err := service.Kill(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case "exit":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		if err := service.MarkExited(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case "tail":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		if isFollowRequest(r) {
			a.streamTail(w, r, id)
			return
		}
		lines := parseLines(r.URL.Query().Get("lines"))
		items, err := service.TailItems(r.Context(), id, lines)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, TailItemsResponse{Items: items})
		return
	case "history":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		lines := parseLines(r.URL.Query().Get("lines"))
		items, err := service.History(r.Context(), id, lines)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, TailItemsResponse{Items: items})
		return
	case "approvals":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		approvals, err := service.ListApprovals(r.Context(), id)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"approvals": approvals})
		return
	case "send":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		var req SendSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid json body",
			})
			return
		}
		input := req.Input
		if len(input) == 0 {
			if strings.TrimSpace(req.Text) != "" {
				input = []map[string]any{{"type": "text", "text": req.Text}}
			}
		}
		if a.Logger != nil {
			a.Logger.Info("send_request",
				logging.F("session_id", id),
				logging.F("input_items", len(input)),
				logging.F("text_len", len(req.Text)),
			)
		}
		turnID, err := service.SendMessage(r.Context(), id, input)
		if err != nil {
			if a.Logger != nil {
				a.Logger.Error("send_error", logging.F("session_id", id), logging.F("error", err))
			}
			writeServiceError(w, err)
			return
		}
		if a.Logger != nil {
			a.Logger.Info("send_ok", logging.F("session_id", id), logging.F("turn_id", turnID))
		}
		writeJSON(w, http.StatusOK, SendSessionResponse{OK: true, TurnID: turnID})
		return
	case "interrupt":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		if err := service.InterruptTurn(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case "events":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		if !isFollowRequest(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "follow=1 is required",
			})
			return
		}
		a.streamEvents(w, r, id)
		return
	case "items":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		if !isFollowRequest(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "follow=1 is required",
			})
			return
		}
		a.streamItems(w, r, id)
		return
	case "approval":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"error": "method not allowed",
			})
			return
		}
		var req ApproveSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid json body",
			})
			return
		}
		if err := service.Approve(r.Context(), id, req.RequestID, req.Decision, req.Responses, req.AcceptSettings); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case "pins":
		a.SessionPins(w, r, id)
		return
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}
