package daemon

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"control/internal/logging"
	"control/internal/types"
)

type API struct {
	Version          string
	Manager          *SessionManager
	Stores           *Stores
	Shutdown         func(context.Context) error
	Syncer           SessionSyncer
	LiveCodex        *CodexLiveManager
	CodexHistoryPool CodexHistoryPool
	Logger           logging.Logger
}

type StartSessionRequest struct {
	Provider       string                       `json:"provider"`
	Cmd            string                       `json:"cmd,omitempty"`
	Cwd            string                       `json:"cwd,omitempty"`
	Args           []string                     `json:"args,omitempty"`
	Env            []string                     `json:"env,omitempty"`
	Title          string                       `json:"title,omitempty"`
	Tags           []string                     `json:"tags,omitempty"`
	WorkspaceID    string                       `json:"workspace_id,omitempty"`
	WorktreeID     string                       `json:"worktree_id,omitempty"`
	Text           string                       `json:"text,omitempty"`
	RuntimeOptions *types.SessionRuntimeOptions `json:"runtime_options,omitempty"`
}

type UpdateSessionRequest struct {
	Title          string                       `json:"title,omitempty"`
	RuntimeOptions *types.SessionRuntimeOptions `json:"runtime_options,omitempty"`
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
	return parseBoolQueryValue(r.URL.Query().Get("follow"))
}

func isRefreshRequest(r *http.Request) bool {
	return parseBoolQueryValue(r.URL.Query().Get("refresh"))
}

func parseBoolQueryValue(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return value == "1" || value == "true" || value == "yes"
}

func (a *API) newSessionService() *SessionService {
	opts := make([]SessionServiceOption, 0, 1)
	if a != nil && a.CodexHistoryPool != nil {
		opts = append(opts, WithCodexHistoryPool(a.CodexHistoryPool))
	}
	return NewSessionService(a.Manager, a.Stores, a.LiveCodex, a.Logger, opts...)
}
