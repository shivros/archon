package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"control/internal/config"
	"control/internal/types"
)

const defaultBaseURL = "http://127.0.0.1:7777"

type Client struct {
	baseURL   string
	tokenPath string
	token     string
	http      *http.Client
}

func New() (*Client, error) {
	tokenPath, err := config.TokenPath()
	if err != nil {
		return nil, err
	}
	c := &Client{
		baseURL:   defaultBaseURL,
		tokenPath: tokenPath,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	_ = c.loadToken()
	return c, nil
}

func NewWithBaseURL(baseURL, token string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		tokenPath: "",
		token:     token,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var resp HealthResponse
	if err := c.doJSON(ctx, http.MethodGet, "/health", nil, false, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListSessions(ctx context.Context) ([]*types.Session, error) {
	var resp SessionsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/sessions", nil, true, &resp); err != nil {
		return nil, err
	}
	return resp.Sessions, nil
}

func (c *Client) ListSessionsWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	var resp SessionsWithMetaResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/sessions", nil, true, &resp); err != nil {
		return nil, nil, err
	}
	return resp.Sessions, resp.SessionMeta, nil
}

func (c *Client) ListWorkspaces(ctx context.Context) ([]*types.Workspace, error) {
	var resp WorkspacesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/workspaces", nil, true, &resp); err != nil {
		return nil, err
	}
	return resp.Workspaces, nil
}

func (c *Client) ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error) {
	var resp WorktreesResponse
	path := fmt.Sprintf("/v1/workspaces/%s/worktrees", workspaceID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, err
	}
	return resp.Worktrees, nil
}

func (c *Client) ListAvailableWorktrees(ctx context.Context, workspaceID string) ([]*types.GitWorktree, error) {
	var resp AvailableWorktreesResponse
	path := fmt.Sprintf("/v1/workspaces/%s/worktrees/available", workspaceID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, err
	}
	return resp.Worktrees, nil
}

func (c *Client) AddWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error) {
	if worktree == nil {
		return nil, errors.New("worktree is required")
	}
	var resp types.Worktree
	path := fmt.Sprintf("/v1/workspaces/%s/worktrees", workspaceID)
	if err := c.doJSON(ctx, http.MethodPost, path, worktree, true, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreateWorktree(ctx context.Context, workspaceID string, req CreateWorktreeRequest) (*types.Worktree, error) {
	var resp types.Worktree
	path := fmt.Sprintf("/v1/workspaces/%s/worktrees/create", workspaceID)
	if err := c.doJSON(ctx, http.MethodPost, path, req, true, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreateWorkspace(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error) {
	if workspace == nil {
		return nil, errors.New("workspace is required")
	}
	var resp types.Workspace
	if err := c.doJSON(ctx, http.MethodPost, "/v1/workspaces", workspace, true, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetAppState(ctx context.Context) (*types.AppState, error) {
	var state types.AppState
	if err := c.doJSON(ctx, http.MethodGet, "/v1/state", nil, true, &state); err != nil {
		if apiErr := asAPIError(err); apiErr != nil && apiErr.StatusCode == http.StatusNotFound {
			return &types.AppState{}, nil
		}
		return nil, err
	}
	return &state, nil
}

func (c *Client) UpdateAppState(ctx context.Context, state *types.AppState) (*types.AppState, error) {
	if state == nil {
		return nil, errors.New("state is required")
	}
	var resp types.AppState
	if err := c.doJSON(ctx, http.MethodPatch, "/v1/state", state, true, &resp); err != nil {
		if apiErr := asAPIError(err); apiErr != nil && apiErr.StatusCode == http.StatusNotFound {
			return state, nil
		}
		return nil, err
	}
	return &resp, nil
}

func (c *Client) StartSession(ctx context.Context, req StartSessionRequest) (*types.Session, error) {
	var session types.Session
	if err := c.doJSON(ctx, http.MethodPost, "/v1/sessions", req, true, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *Client) StartWorkspaceSession(ctx context.Context, workspaceID, worktreeID string, req StartSessionRequest) (*types.Session, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, errors.New("workspace id is required")
	}
	path := "/v1/workspaces/" + strings.TrimSpace(workspaceID) + "/sessions"
	if strings.TrimSpace(worktreeID) != "" {
		path = "/v1/workspaces/" + strings.TrimSpace(workspaceID) + "/worktrees/" + strings.TrimSpace(worktreeID) + "/sessions"
	}
	var session types.Session
	if err := c.doJSON(ctx, http.MethodPost, path, req, true, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *Client) GetSession(ctx context.Context, id string) (*types.Session, error) {
	var session types.Session
	if err := c.doJSON(ctx, http.MethodGet, "/v1/sessions/"+id, nil, true, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *Client) KillSession(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/sessions/"+id+"/kill", nil, true, nil)
}

func (c *Client) MarkSessionExited(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/sessions/"+id+"/exit", nil, true, nil)
}

func (c *Client) TailItems(ctx context.Context, id string, lines int) (*TailItemsResponse, error) {
	path := fmt.Sprintf("/v1/sessions/%s/tail?lines=%d", id, lines)
	var resp TailItemsResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) History(ctx context.Context, id string, lines int) (*TailItemsResponse, error) {
	path := fmt.Sprintf("/v1/sessions/%s/history?lines=%d", id, lines)
	var resp TailItemsResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) SendMessage(ctx context.Context, id string, req SendSessionRequest) (*SendSessionResponse, error) {
	path := fmt.Sprintf("/v1/sessions/%s/send", strings.TrimSpace(id))
	var resp SendSessionResponse
	if err := c.doJSON(ctx, http.MethodPost, path, req, true, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListApprovals(ctx context.Context, id string) ([]*types.Approval, error) {
	path := fmt.Sprintf("/v1/sessions/%s/approvals", strings.TrimSpace(id))
	var resp ApprovalsResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, true, &resp); err != nil {
		return nil, err
	}
	return resp.Approvals, nil
}

func (c *Client) ApproveSession(ctx context.Context, id string, req ApproveSessionRequest) error {
	path := fmt.Sprintf("/v1/sessions/%s/approval", strings.TrimSpace(id))
	return c.doJSON(ctx, http.MethodPost, path, req, true, nil)
}

func (c *Client) InterruptSession(ctx context.Context, id string) error {
	path := fmt.Sprintf("/v1/sessions/%s/interrupt", strings.TrimSpace(id))
	return c.doJSON(ctx, http.MethodPost, path, nil, true, nil)
}

func (c *Client) EnsureDaemon(ctx context.Context) error {
	return c.ensureDaemon(ctx, "", false)
}

func (c *Client) EnsureDaemonVersion(ctx context.Context, expectedVersion string, restart bool) error {
	return c.ensureDaemon(ctx, expectedVersion, restart)
}

func (c *Client) ShutdownDaemon(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/shutdown", nil, true, nil)
}

func (c *Client) ensureDaemon(ctx context.Context, expectedVersion string, restart bool) error {
	resp, err := c.Health(ctx)
	if err == nil && resp.OK {
		if expectedVersion == "" || resp.Version == expectedVersion {
			return nil
		}
		if !restart {
			return fmt.Errorf("daemon version mismatch: %s (expected %s)", resp.Version, expectedVersion)
		}
		if err := c.ShutdownDaemon(ctx); err != nil {
			apiErr := asAPIError(err)
			if apiErr == nil || apiErr.StatusCode != http.StatusNotFound {
				return err
			}
			if resp.PID <= 0 {
				return err
			}
			if killErr := killProcess(resp.PID); killErr != nil {
				return fmt.Errorf("failed to stop stale daemon (pid %d): %w", resp.PID, killErr)
			}
		}
		shutdownDeadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(shutdownDeadline) {
			if _, err := c.Health(ctx); err != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	if err := StartBackgroundDaemon(); err != nil {
		return err
	}

	deadline := time.Now().Add(4 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := c.Health(ctx)
		if err == nil && resp.OK {
			if expectedVersion == "" || resp.Version == expectedVersion {
				_ = c.loadToken()
				return nil
			}
			lastErr = fmt.Errorf("daemon version mismatch: %s (expected %s)", resp.Version, expectedVersion)
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = errors.New("daemon not healthy after start")
	}
	return lastErr
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, requireAuth bool, out any) error {
	return c.doJSONWithClient(ctx, method, path, body, requireAuth, out, c.http)
}

func (c *Client) doJSONWithTimeout(ctx context.Context, method, path string, body any, requireAuth bool, out any, timeout time.Duration) error {
	client := c.http
	if timeout > 0 {
		client = &http.Client{
			Timeout:   timeout,
			Transport: c.http.Transport,
		}
	}
	return c.doJSONWithClient(ctx, method, path, body, requireAuth, out, client)
}

func (c *Client) doJSONWithClient(ctx context.Context, method, path string, body any, requireAuth bool, out any, httpClient *http.Client) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if requireAuth {
		if err := c.ensureToken(); err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) ensureToken() error {
	if strings.TrimSpace(c.token) == "" {
		if err := c.loadToken(); err != nil {
			return err
		}
	}
	if strings.TrimSpace(c.token) == "" {
		return errors.New("token not found; is the daemon running?")
	}
	return nil
}

func (c *Client) loadToken() error {
	data, err := os.ReadFile(c.tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.token = ""
			return nil
		}
		return err
	}
	c.token = strings.TrimSpace(string(data))
	return nil
}

func decodeAPIError(resp *http.Response) error {
	type errorPayload struct {
		Error string `json:"error"`
	}
	var payload errorPayload
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if payload.Error != "" {
		return &APIError{StatusCode: resp.StatusCode, Message: payload.Error}
	}
	return &APIError{StatusCode: resp.StatusCode, Message: resp.Status}
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("api error (%d): %s", e.StatusCode, e.Message)
}

func asAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}

var killProcess = terminateProcess

func terminateProcess(pid int) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return proc.Kill()
	}
	return proc.Signal(syscall.SIGTERM)
}
