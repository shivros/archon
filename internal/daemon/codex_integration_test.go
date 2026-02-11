package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/logging"
	"control/internal/store"
	"control/internal/testutil"
	"control/internal/types"
)

const (
	codexIntegrationEnv  = "ARCHON_CODEX_INTEGRATION"
	codexIntegrationSkip = "ARCHON_CODEX_SKIP"
)

func TestCodexAppServerIntegration(t *testing.T) {
	requireCodexIntegration(t)

	repoDir, codexHome := createCodexWorkspace(t)
	logger := logging.New(io.Discard, logging.Info)

	ctx, cancel := context.WithTimeout(context.Background(), codexIntegrationTimeout())
	defer cancel()

	client, err := startCodexAppServer(ctx, repoDir, codexHome, logger)
	if err != nil {
		t.Fatalf("start codex app-server: %v", err)
	}
	defer client.Close()

	model := strings.TrimSpace(os.Getenv("ARCHON_CODEX_MODEL"))
	if model == "" {
		model = defaultCodexModel
	}

	var threadResult struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := client.request(ctx, "thread/start", map[string]any{
		"model": model,
		"cwd":   repoDir,
	}, &threadResult); err != nil {
		t.Fatalf("thread/start: %v", err)
	}
	if threadResult.Thread.ID == "" {
		t.Fatalf("thread id missing")
	}

	turnID, err := client.StartTurn(ctx, threadResult.Thread.ID, []map[string]any{
		{"type": "text", "text": "Say \"ok\" and nothing else."},
	}, nil, model)
	if err != nil {
		t.Fatalf("turn/start: %v", err)
	}
	if turnID == "" {
		t.Fatalf("turn id missing")
	}

	waitForCodexTurn(t, client, turnID, codexIntegrationTimeout())
}

func TestAPICodexSessionFlow(t *testing.T) {
	requireCodexIntegration(t)

	repoDir, _ := createCodexWorkspace(t)

	server, manager, _ := newCodexIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)

	session := startSession(t, server, StartSessionRequest{
		Provider:    "codex",
		WorkspaceID: ws.ID,
		Text:        "Say \"ok\" and nothing else.",
	})
	if session.ID == "" {
		t.Fatalf("session id missing")
	}

	list := listSessions(t, server)
	if len(list.Sessions) == 0 {
		t.Fatalf("expected sessions list to be non-empty")
	}

	waitForHistoryItems(t, server, session.ID, codexIntegrationTimeout())

	waitForStatus(t, manager, session.ID, types.SessionStatusExited, codexIntegrationTimeout())

	turnID := sendMessageWithRetry(t, server, session.ID, "Say \"ok\" again.")
	if turnID == "" {
		t.Fatalf("turn id missing from send")
	}

	waitForHistoryItems(t, server, session.ID, codexIntegrationTimeout())
}

func TestCodexTailStream(t *testing.T) {
	requireCodexIntegration(t)

	repoDir, _ := createCodexWorkspace(t)
	server, _, _ := newCodexIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)
	session := startSession(t, server, StartSessionRequest{
		Provider:    "codex",
		WorkspaceID: ws.ID,
		Text:        "Say \"ok\" and nothing else.",
	})

	stream, closeFn := openSSE(t, server, "/v1/sessions/"+session.ID+"/tail?follow=1&stream=combined")
	defer closeFn()

	data, ok := waitForSSEData(stream, 30*time.Second)
	if !ok {
		t.Fatalf("timeout waiting for tail stream event")
	}

	var event types.LogEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		t.Fatalf("decode log event: %v", err)
	}
	if event.Chunk == "" {
		t.Fatalf("expected log chunk to be non-empty")
	}
}

func TestCodexEventsStream(t *testing.T) {
	requireCodexIntegration(t)

	repoDir, _ := createCodexWorkspace(t)
	server, _, _ := newCodexIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)
	session := startSession(t, server, StartSessionRequest{
		Provider:    "codex",
		WorkspaceID: ws.ID,
		Text:        "Say \"ok\" and nothing else.",
	})

	stream, closeFn := openSSE(t, server, "/v1/sessions/"+session.ID+"/events?follow=1")
	defer closeFn()

	_ = sendMessageWithRetry(t, server, session.ID, "Say \"ok\" again.")

	events := collectEvents(stream, 30*time.Second)
	if len(events) == 0 {
		t.Fatalf("expected events from SSE stream")
	}
	found := false
	for _, event := range events {
		if event.Method != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected at least one event method")
	}
}

func TestCodexInterruptFlow(t *testing.T) {
	requireCodexIntegration(t)

	repoDir, _ := createCodexWorkspace(t)
	server, _, _ := newCodexIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)
	session := startSession(t, server, StartSessionRequest{
		Provider:    "codex",
		WorkspaceID: ws.ID,
		Text:        "Say \"ok\" and nothing else.",
	})

	stream, closeFn := openSSE(t, server, "/v1/sessions/"+session.ID+"/events?follow=1")
	defer closeFn()

	longPrompt := "Write a detailed, multi-section response of at least 2000 words about distributed systems. Begin now."
	_ = sendMessageWithRetry(t, server, session.ID, longPrompt)

	started := waitForEvent(stream, "turn/started", 5*time.Second)
	if !started {
		t.Logf("turn/started not observed; attempting interrupt anyway")
	}

	status, body := interruptSession(server, session.ID)
	if status != http.StatusOK {
		if strings.Contains(body, "no active turn") || strings.Contains(body, "turn already") {
			t.Skipf("interrupt skipped: %s", strings.TrimSpace(body))
		}
		t.Fatalf("interrupt failed status=%d body=%s", status, body)
	}

	events := collectEvents(stream, 30*time.Second)
	if !hasTurnStatus(events, "interrupted") {
		t.Fatalf("expected interrupted turn completion")
	}
}

func TestCodexApprovalFlow(t *testing.T) {
	requireCodexIntegration(t)

	t.Setenv("ARCHON_CODEX_APPROVAL_POLICY", "untrusted")
	t.Setenv("ARCHON_CODEX_SANDBOX_POLICY", "workspace-write")
	t.Setenv("ARCHON_CODEX_NETWORK_ACCESS", "false")
	repoDir, codexHome := createCodexWorkspace(t)
	writeCodexConfig(t, codexHome, repoDir, "untrusted", "workspace-write", "untrusted")
	requireCodexAuth(t, repoDir, codexHome)
	server, _, _ := newCodexIntegrationServer(t)
	defer server.Close()

	ws := createWorkspace(t, server, repoDir)
	session := startSession(t, server, StartSessionRequest{
		Provider:    "codex",
		WorkspaceID: ws.ID,
		Text:        "Say \"ok\" and nothing else.",
	})

	stream, closeFn := openSSE(t, server, "/v1/sessions/"+session.ID+"/events?follow=1")
	defer closeFn()

	targetFile := filepath.Join(repoDir, "approval-created.txt")
	_ = os.Remove(targetFile)

	_ = sendMessageWithRetry(t, server, session.ID, "Create a new file named `approval-created.txt` containing exactly `ok`. Do not answer until the file is created.")

	approval, seen := waitForApprovalEventWithTrace(stream, 20*time.Second)
	if approval == nil || approval.ID == nil {
		methods := make([]string, 0, len(seen))
		errors := extractEventErrors(seen)
		for _, event := range seen {
			if event.Method != "" {
				methods = append(methods, event.Method)
			}
		}
		for _, msg := range errors {
			if strings.Contains(strings.ToLower(msg), "quota exceeded") {
				diag := fetchCodexDiagnostics(repoDir, codexHome)
				t.Fatalf("codex quota exceeded; ensure the authenticated account has quota or set ARCHON_CODEX_API_KEY to a billed key (diag=%s)", diag)
			}
		}
		t.Fatalf("expected approval event but none observed (events=%v errors=%v)", methods, errors)
	}

	approvals := waitForApprovals(t, server, session.ID, 5*time.Second)
	if len(approvals) == 0 {
		id := 0
		if approval.ID != nil {
			id = *approval.ID
		}
		t.Fatalf("expected approvals list to be non-empty (event_method=%s event_id=%d)", approval.Method, id)
	}
	idVal := 0
	if approval.ID != nil {
		idVal = *approval.ID
	}
	t.Logf("approval event id=%d method=%s", idVal, approval.Method)

	status, body := approveSession(server, session.ID, idVal, "accept")
	if status != http.StatusOK {
		t.Fatalf("approval failed status=%d body=%s", status, body)
	}

	approvals = waitForApprovals(t, server, session.ID, 5*time.Second)
	if len(approvals) != 0 {
		t.Fatalf("expected approvals to clear after accept")
	}

	waitForFile(t, targetFile, 10*time.Second)
}

func requireCodexIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv(codexIntegrationSkip) == "1" {
		t.Skipf("%s=1 set; skipping codex integration tests", codexIntegrationSkip)
	}
	cmd := strings.TrimSpace(os.Getenv("ARCHON_CODEX_CMD"))
	if cmd == "" {
		cmd = "codex"
	}
	if _, err := exec.LookPath(cmd); err != nil {
		if os.Getenv(codexIntegrationEnv) == "1" {
			t.Fatalf("codex command not found (%s): %v", cmd, err)
		}
		t.Skipf("codex command not found (%s); set %s=1 to require or install codex", cmd, codexIntegrationEnv)
	}
}

func codexIntegrationTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("ARCHON_CODEX_TIMEOUT")); raw != "" {
		if secs, err := time.ParseDuration(raw); err == nil {
			return secs
		}
	}
	return 2 * time.Minute
}

func createCodexWorkspace(t *testing.T) (string, string) {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o700); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	codexHome := filepath.Join(repoDir, codexHomeDirName)
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	return repoDir, codexHome
}

func writeCodexConfig(t *testing.T, codexHome, repoDir, approvalPolicy, sandboxMode, trustLevel string) {
	t.Helper()
	if strings.TrimSpace(codexHome) == "" {
		t.Fatalf("codex home is required")
	}
	if strings.TrimSpace(repoDir) == "" {
		t.Fatalf("repo dir is required")
	}
	path := filepath.Join(codexHome, "config.toml")
	content := strings.Builder{}
	authStore := strings.TrimSpace(os.Getenv("ARCHON_CODEX_AUTH_STORE"))
	if authStore == "" {
		authStore = "keyring"
	}
	content.WriteString(`cli_auth_credentials_store = "` + authStore + `"` + "\n")
	if strings.TrimSpace(approvalPolicy) != "" {
		content.WriteString(`approval_policy = "` + approvalPolicy + `"` + "\n")
	}
	if strings.TrimSpace(sandboxMode) != "" {
		content.WriteString(`sandbox_mode = "` + sandboxMode + `"` + "\n")
	}
	content.WriteString("\n")
	if strings.TrimSpace(trustLevel) == "" {
		trustLevel = "trusted"
	}
	content.WriteString(`[projects."` + repoDir + `"]` + "\n")
	content.WriteString(`trust_level = "` + trustLevel + `"` + "\n")
	if err := os.WriteFile(path, []byte(content.String()), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
}

func requireCodexAuth(t *testing.T, repoDir, codexHome string) {
	t.Helper()
	apiKey := testutil.LoadCodexAPIKey()
	forceAPI := strings.TrimSpace(os.Getenv("ARCHON_CODEX_FORCE_API_KEY")) == "1"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	checkAuth := func() (bool, error) {
		client, err := startCodexAppServer(ctx, repoDir, codexHome, logging.New(io.Discard, logging.Info))
		if err != nil {
			return false, err
		}
		defer client.Close()
		var result struct {
			Account            any  `json:"account"`
			RequiresOpenaiAuth bool `json:"requiresOpenaiAuth"`
		}
		if err := client.request(ctx, "account/read", map[string]any{"refreshToken": false}, &result); err != nil {
			return false, err
		}
		if data, err := json.Marshal(result); err == nil {
			t.Logf("codex account/read: %s", string(data))
		}
		if result.RequiresOpenaiAuth && result.Account == nil {
			return false, nil
		}
		return true, nil
	}

	if !forceAPI {
		ok, err := checkAuth()
		if err != nil {
			t.Fatalf("account/read failed: %v", err)
		}
		if ok {
			return
		}
	}

	if apiKey != "" {
		client, err := startCodexAppServer(ctx, repoDir, codexHome, logging.New(io.Discard, logging.Info))
		if err != nil {
			t.Fatalf("start codex app-server for apiKey login: %v", err)
		}
		if err := client.request(ctx, "account/login/start", map[string]any{
			"type":   "apiKey",
			"apiKey": apiKey,
		}, nil); err != nil {
			client.Close()
			t.Fatalf("account/login/start failed: %v", err)
		}
		client.Close()
		ok, err := checkAuth()
		if err != nil {
			t.Fatalf("account/read after apiKey login failed: %v", err)
		}
		if ok {
			return
		}
		t.Fatalf("codex auth still not configured after apiKey login")
	}

	if tryCopyAuthFile(t, codexHome) {
		ok, err := checkAuth()
		if err != nil {
			t.Fatalf("account/read after auth.json copy failed: %v", err)
		}
		if ok {
			return
		}
	}

	t.Fatalf("codex auth not configured; log in or set ARCHON_CODEX_API_KEY/OPENAI_API_KEY or provide ~/.codex/auth.json or ~/.archon/test-keys.json")
}

func tryCopyAuthFile(t *testing.T, codexHome string) bool {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	src := filepath.Join(home, ".codex", "auth.json")
	data, err := os.ReadFile(src)
	if err != nil {
		return false
	}
	dest := filepath.Join(codexHome, "auth.json")
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	return true
}

func newCodexIntegrationServer(t *testing.T) (*httptest.Server, *SessionManager, *Stores) {
	t.Helper()
	base := t.TempDir()
	manager, err := NewSessionManager(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	workspaces := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	state := store.NewFileAppStateStore(filepath.Join(base, "state.json"))
	meta := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))
	sessions := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))
	approvals := store.NewFileApprovalStore(filepath.Join(base, "approvals.json"))

	stores := &Stores{
		Workspaces:  workspaces,
		Worktrees:   workspaces,
		Groups:      workspaces,
		AppState:    state,
		SessionMeta: meta,
		Sessions:    sessions,
		Approvals:   approvals,
	}

	manager.SetMetaStore(meta)
	manager.SetSessionStore(sessions)

	logger := logging.New(io.Discard, logging.Info)
	api := &API{
		Version: "test",
		Manager: manager,
		Stores:  stores,
		Logger:  logger,
	}
	api.LiveCodex = NewCodexLiveManager(stores, logger)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	return server, manager, stores
}

func createWorkspace(t *testing.T, server *httptest.Server, repoDir string) *types.Workspace {
	t.Helper()
	body, _ := json.Marshal(types.Workspace{
		Name:     "codex-test",
		RepoPath: repoDir,
	})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create workspace status: %d", resp.StatusCode)
	}
	var ws types.Workspace
	if err := json.NewDecoder(resp.Body).Decode(&ws); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	return &ws
}

func waitForHistoryItems(t *testing.T, server *httptest.Server, sessionID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		history := historySession(t, server, sessionID)
		if len(history.Items) > 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for history items")
}

func sendMessageWithRetry(t *testing.T, server *httptest.Server, sessionID, text string) string {
	t.Helper()
	deadline := time.Now().Add(codexIntegrationTimeout())
	for {
		status, body, turnID := sendMessageOnce(server, sessionID, text)
		if status == http.StatusOK && turnID != "" {
			return turnID
		}
		if status == http.StatusBadRequest && strings.Contains(body, "turn already in progress") && time.Now().Before(deadline) {
			time.Sleep(1 * time.Second)
			continue
		}
		t.Fatalf("send failed status=%d body=%s", status, body)
	}
}

func sendMessageOnce(server *httptest.Server, sessionID, text string) (int, string, string) {
	reqBody, _ := json.Marshal(map[string]string{"text": text})
	body := bytes.NewBuffer(reqBody)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/"+sessionID+"/send", body)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error(), ""
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, string(data), ""
	}
	var payload SendSessionResponse
	if err := json.Unmarshal(data, &payload); err != nil {
		return resp.StatusCode, "decode error: " + err.Error(), ""
	}
	return resp.StatusCode, string(data), payload.TurnID
}

func waitForCodexTurn(t *testing.T, client *codexAppServer, turnID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-client.Errors():
			if err != nil {
				t.Fatalf("codex error: %v", err)
			}
		case msg := <-client.Notifications():
			if msg.Method != "turn/completed" {
				continue
			}
			var payload struct {
				Turn struct {
					ID string `json:"id"`
				} `json:"turn"`
			}
			if len(msg.Params) > 0 && json.Unmarshal(msg.Params, &payload) == nil {
				if payload.Turn.ID == turnID {
					return
				}
			}
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatalf("timeout waiting for turn completion")
}

func openSSE(t *testing.T, server *httptest.Server, path string) (<-chan string, func()) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open sse: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("open sse status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "data: ") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
				if payload != "" {
					ch <- payload
				}
			}
		}
	}()
	closeFn := func() {
		_ = resp.Body.Close()
	}
	return ch, closeFn
}

func waitForSSEData(ch <-chan string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return "", false
			}
			return data, true
		case <-time.After(200 * time.Millisecond):
		}
	}
	return "", false
}

func collectEvents(ch <-chan string, timeout time.Duration) []types.CodexEvent {
	deadline := time.Now().Add(timeout)
	out := make([]types.CodexEvent, 0)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return out
			}
			var event types.CodexEvent
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				out = append(out, event)
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	return out
}

func waitForEvent(ch <-chan string, method string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return false
			}
			var event types.CodexEvent
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				if event.Method == method {
					return true
				}
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	return false
}

func hasTurnStatus(events []types.CodexEvent, status string) bool {
	for _, event := range events {
		if event.Method != "turn/completed" {
			continue
		}
		var payload struct {
			Turn struct {
				Status string `json:"status"`
			} `json:"turn"`
		}
		if len(event.Params) > 0 && json.Unmarshal(event.Params, &payload) == nil {
			if payload.Turn.Status == status {
				return true
			}
		}
	}
	return false
}

func waitForApprovalEventWithTrace(ch <-chan string, timeout time.Duration) (*types.CodexEvent, []types.CodexEvent) {
	deadline := time.Now().Add(timeout)
	seen := make([]types.CodexEvent, 0, 16)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return nil, seen
			}
			var event types.CodexEvent
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				seen = append(seen, event)
				if isApprovalMethod(event.Method) {
					return &event, seen
				}
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil, seen
}

func interruptSession(server *httptest.Server, sessionID string) (int, string) {
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/"+sessionID+"/interrupt", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func listApprovals(t *testing.T, server *httptest.Server, sessionID string) []*types.Approval {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/sessions/"+sessionID+"/approvals", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("list approvals status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Approvals []*types.Approval `json:"approvals"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode approvals: %v", err)
	}
	return payload.Approvals
}

func approveSession(server *httptest.Server, sessionID string, requestID int, decision string) (int, string) {
	body, _ := json.Marshal(map[string]any{
		"request_id": requestID,
		"decision":   decision,
	})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/"+sessionID+"/approval", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func waitForApprovals(t *testing.T, server *httptest.Server, sessionID string, timeout time.Duration) []*types.Approval {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		approvals := listApprovals(t, server, sessionID)
		if len(approvals) > 0 {
			return approvals
		}
		time.Sleep(200 * time.Millisecond)
	}
	return []*types.Approval{}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("expected %s to be created", path)
}

func extractEventErrors(events []types.CodexEvent) []string {
	out := make([]string, 0)
	for _, event := range events {
		if event.Method != "error" && event.Method != "codex/event/error" && event.Method != "codex/event/stream_error" {
			continue
		}
		if len(event.Params) == 0 {
			out = append(out, event.Method)
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(event.Params, &payload); err != nil {
			out = append(out, event.Method)
			continue
		}
		if errVal, ok := payload["error"]; ok {
			if msg := extractErrorMessage(errVal); msg != "" {
				out = append(out, msg)
				continue
			}
		}
		out = append(out, event.Method)
	}
	return out
}

func extractErrorMessage(raw any) string {
	if raw == nil {
		return ""
	}
	switch val := raw.(type) {
	case map[string]any:
		if msg, ok := val["message"].(string); ok && msg != "" {
			return msg
		}
	case string:
		return val
	}
	return ""
}

func fetchCodexDiagnostics(repoDir, codexHome string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := startCodexAppServer(ctx, repoDir, codexHome, logging.New(io.Discard, logging.Info))
	if err != nil {
		return "start_error=" + err.Error()
	}
	defer client.Close()

	var accountResp map[string]any
	if err := client.request(ctx, "account/read", map[string]any{"refreshToken": false}, &accountResp); err != nil {
		return "account_read_error=" + err.Error()
	}
	var rateResp map[string]any
	_ = client.request(ctx, "account/rateLimits/read", map[string]any{}, &rateResp)

	accountJSON, _ := json.Marshal(accountResp)
	rateJSON, _ := json.Marshal(rateResp)
	return "account=" + string(accountJSON) + " rate_limits=" + string(rateJSON)
}
