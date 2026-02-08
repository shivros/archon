package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

type API struct {
	Version   string
	Manager   *SessionManager
	Stores    *Stores
	Shutdown  func(context.Context) error
	Syncer    SessionSyncer
	LiveCodex *CodexLiveManager
	Logger    logging.Logger
}

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": a.Version,
		"pid":     os.Getpid(),
	})
}

func (a *API) Sessions(w http.ResponseWriter, r *http.Request) {
	service := NewSessionService(a.Manager, a.Stores, a.LiveCodex, a.Logger)

	switch r.Method {
	case http.MethodGet:
		sessions, meta, err := service.ListWithMeta(r.Context())
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
			if err := service.UpdateTitle(r.Context(), id, req.Title); err != nil {
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
			if err != nil {
				if a.Logger != nil {
					a.Logger.Error("send_error", logging.F("session_id", id), logging.F("error", err))
				}
				writeServiceError(w, err)
				return
			}
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
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (a *API) CodexThreadDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	cwd := strings.TrimSpace(r.URL.Query().Get("cwd"))
	threadID := strings.TrimSpace(r.URL.Query().Get("id"))
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	if cwd == "" || threadID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cwd and id are required"})
		return
	}
	workspacePath := ""
	if workspaceID != "" && a.Stores != nil && a.Stores.Workspaces != nil {
		if ws, ok, err := a.Stores.Workspaces.Get(r.Context(), workspaceID); err == nil && ok && ws != nil {
			workspacePath = ws.RepoPath
		}
	}
	codexHome := resolveCodexHome(cwd, workspacePath)
	client, err := startCodexAppServer(r.Context(), cwd, codexHome, a.Logger)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer client.Close()

	thread, err := client.ReadThread(r.Context(), threadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cwds := detectStringsByKey(thread, "cwd")
	writeJSON(w, http.StatusOK, map[string]any{
		"thread": thread,
		"cwds":   cwds,
	})
}

type StartSessionRequest struct {
	Provider    string   `json:"provider"`
	Cmd         string   `json:"cmd,omitempty"`
	Cwd         string   `json:"cwd,omitempty"`
	Args        []string `json:"args,omitempty"`
	Env         []string `json:"env,omitempty"`
	Title       string   `json:"title,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	WorkspaceID string   `json:"workspace_id,omitempty"`
	WorktreeID  string   `json:"worktree_id,omitempty"`
	Text        string   `json:"text,omitempty"`
}

type UpdateSessionRequest struct {
	Title string `json:"title"`
}

type TailItemsResponse struct {
	Items []map[string]any `json:"items"`
}

type SendSessionRequest struct {
	Text  string           `json:"text,omitempty"`
	Input []map[string]any `json:"input,omitempty"`
}

type SendSessionResponse struct {
	OK     bool   `json:"ok"`
	TurnID string `json:"turn_id,omitempty"`
}

type ApproveSessionRequest struct {
	RequestID      int            `json:"request_id"`
	Decision       string         `json:"decision"`
	Responses      []string       `json:"responses,omitempty"`
	AcceptSettings map[string]any `json:"accept_settings,omitempty"`
}

func parseLines(raw string) int {
	if raw == "" {
		return 200
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return 200
	}
	return val
}

func isFollowRequest(r *http.Request) bool {
	follow := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("follow")))
	return follow == "1" || follow == "true" || follow == "yes"
}

func (a *API) Workspaces(w http.ResponseWriter, r *http.Request) {
	service := NewWorkspaceSyncService(a.Stores, a.Syncer)

	switch r.Method {
	case http.MethodGet:
		workspaces, err := service.List(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces})
		return
	case http.MethodPost:
		var req types.Workspace
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		ws, err := service.Create(r.Context(), &req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, ws)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) WorkspaceGroups(w http.ResponseWriter, r *http.Request) {
	service := NewWorkspaceGroupService(a.Stores)

	switch r.Method {
	case http.MethodGet:
		groups, err := service.List(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
		return
	case http.MethodPost:
		var req types.WorkspaceGroup
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		group, err := service.Create(r.Context(), &req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, group)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) WorkspaceGroupByID(w http.ResponseWriter, r *http.Request) {
	service := NewWorkspaceGroupService(a.Stores)
	path := strings.TrimPrefix(r.URL.Path, "/v1/workspace-groups/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			var req types.WorkspaceGroup
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			group, err := service.Update(r.Context(), id, &req)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, group)
			return
		case http.MethodDelete:
			if err := service.Delete(r.Context(), id); err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (a *API) WorkspaceByID(w http.ResponseWriter, r *http.Request) {
	service := NewWorkspaceSyncService(a.Stores, a.Syncer)
	path := strings.TrimPrefix(r.URL.Path, "/v1/workspaces/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			var req types.Workspace
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			ws, err := service.Update(r.Context(), id, &req)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, ws)
			return
		case http.MethodDelete:
			if err := service.Delete(r.Context(), id); err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
	}

	if parts[1] == "worktrees" {
		a.Worktrees(w, r, id, parts)
		return
	}
	if parts[1] == "sessions" {
		a.startSessionForWorkspace(w, r, id, "")
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (a *API) Worktrees(w http.ResponseWriter, r *http.Request, workspaceID string, parts []string) {
	service := NewWorkspaceSyncService(a.Stores, a.Syncer)

	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			worktrees, err := service.ListWorktrees(r.Context(), workspaceID)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"worktrees": worktrees})
			return
		case http.MethodPost:
			var req types.Worktree
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			wt, err := service.AddWorktree(r.Context(), workspaceID, &req)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, wt)
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
	}

	if len(parts) == 3 {
		switch parts[2] {
		case "available":
			if r.Method != http.MethodGet {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
			worktrees, err := service.ListAvailableWorktrees(r.Context(), workspaceID)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"worktrees": worktrees})
			return
		case "create":
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
			var req CreateWorktreeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			wt, err := service.CreateWorktree(r.Context(), workspaceID, &req)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, wt)
			return
		default:
			if r.Method == http.MethodDelete {
				worktreeID := parts[2]
				if err := service.DeleteWorktree(r.Context(), workspaceID, worktreeID); err != nil {
					writeServiceError(w, err)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"ok": true})
				return
			}
		}
	}
	if len(parts) == 4 && parts[3] == "sessions" {
		a.startSessionForWorkspace(w, r, workspaceID, parts[2])
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (a *API) startSessionForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, worktreeID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := NewSessionService(a.Manager, a.Stores, a.LiveCodex, a.Logger)
	var req StartSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	req.Provider = provider
	req.WorkspaceID = workspaceID
	req.WorktreeID = worktreeID
	session, err := service.Start(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (a *API) AppState(w http.ResponseWriter, r *http.Request) {
	service := NewAppStateService(a.Stores)
	switch r.Method {
	case http.MethodGet:
		state, err := service.Get(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	case http.MethodPatch:
		var req types.AppState
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if err := service.Update(r.Context(), &req); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, req)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) Keymap(w http.ResponseWriter, r *http.Request) {
	service := NewKeymapService(a.Stores)
	switch r.Method {
	case http.MethodGet:
		keymap, err := service.Get(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, keymap)
		return
	case http.MethodPatch:
		var req types.Keymap
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if err := service.Update(r.Context(), &req); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, req)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) ShutdownDaemon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if a.Shutdown == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "shutdown not available"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	}()
}

// Keep existing streamTail after this block.
func (a *API) streamTail(w http.ResponseWriter, r *http.Request, id string) {
	stream := r.URL.Query().Get("stream")
	service := NewSessionService(a.Manager, a.Stores, a.LiveCodex, a.Logger)
	ch, cancel, err := service.Subscribe(r.Context(), id, stream)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer cancel()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

func (a *API) streamEvents(w http.ResponseWriter, r *http.Request, id string) {
	service := NewSessionService(a.Manager, a.Stores, a.LiveCodex, a.Logger)
	reqID := logging.NewRequestID()
	if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
		a.Logger.Debug("events_stream_open",
			logging.F("req_id", reqID),
			logging.F("session_id", id),
		)
	}
	ch, cancel, err := service.SubscribeEvents(r.Context(), id)
	if err != nil {
		if a.Logger != nil {
			a.Logger.Warn("events_stream_subscribe_error",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("error", err),
			)
		}
		writeServiceError(w, err)
		return
	}
	defer cancel()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()
	var count int
	firstMethod := ""
	reason := "unknown"
	defer func() {
		if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
			a.Logger.Debug("events_stream_close",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("count", count),
				logging.F("first_method", firstMethod),
				logging.F("reason", reason),
			)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			reason = "ctx_done"
			return
		case event, ok := <-ch:
			if !ok {
				reason = "channel_closed"
				return
			}
			if count == 0 {
				firstMethod = event.Method
			}
			count++
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
			if event.Method == "turn/completed" {
				reason = "turn_completed"
				return
			}
		}
	}
}

func (a *API) streamItems(w http.ResponseWriter, r *http.Request, id string) {
	service := NewSessionService(a.Manager, a.Stores, a.LiveCodex, a.Logger)
	reqID := logging.NewRequestID()
	lines := parseLines(r.URL.Query().Get("lines"))
	snapshot, _, snapErr := service.readSessionItems(id, lines)
	ch, cancel, err := service.SubscribeItems(r.Context(), id)
	if err != nil && (snapErr != nil || len(snapshot) == 0) {
		if a.Logger != nil {
			a.Logger.Warn("items_stream_subscribe_error",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("error", err),
			)
		}
		writeServiceError(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
		a.Logger.Debug("items_stream_open",
			logging.F("req_id", reqID),
			logging.F("session_id", id),
			logging.F("snapshot_items", len(snapshot)),
			logging.F("lines", lines),
			logging.F("subscribe_error", err != nil),
		)
	}

	if len(snapshot) > 0 {
		for _, item := range snapshot {
			data, err := json.Marshal(item)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
		}
		flusher.Flush()
	}
	if err != nil {
		return
	}
	defer cancel()

	ctx := r.Context()
	var count int
	firstType := ""
	reason := "unknown"
	defer func() {
		if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
			a.Logger.Debug("items_stream_close",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("count", count),
				logging.F("first_type", firstType),
				logging.F("reason", reason),
			)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			reason = "ctx_done"
			return
		case item, ok := <-ch:
			if !ok {
				reason = "channel_closed"
				return
			}
			if count == 0 {
				if typ, _ := item["type"].(string); typ != "" {
					firstType = typ
				}
			}
			count++
			data, err := json.Marshal(item)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
