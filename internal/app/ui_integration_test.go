package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"control/internal/client"
	"control/internal/daemon"
	"control/internal/logging"
	"control/internal/store"
	"control/internal/testutil"
	"control/internal/types"
)

const (
	uiIntegrationEnv      = "ARCHON_UI_INTEGRATION"
	codexIntegrationEnv   = "ARCHON_CODEX_INTEGRATION"
	codexIntegrationSkip  = "ARCHON_CODEX_SKIP"
	claudeIntegrationEnv  = "ARCHON_CLAUDE_INTEGRATION"
	claudeIntegrationSkip = "ARCHON_CLAUDE_SKIP"
)

func TestUICodexStreamingExistingSession(t *testing.T) {
	requireUIIntegration(t)
	requireCodexIntegration(t)

	start := time.Now()
	phase := "init"
	logPhase := func(next string) {
		phase = next
		t.Logf("phase=%s elapsed=%s", phase, time.Since(start).Truncate(time.Millisecond))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	logPhase("create_workspace")
	repoDir, codexHome := createCodexWorkspace(t)
	writeCodexConfig(t, codexHome, repoDir, "", "", "")
	requireCodexAuth(t, repoDir, codexHome)

	server, _, _ := newUITestServer(t)
	defer server.Close()

	api := client.NewWithBaseURL(server.URL, "token")

	ws, err := api.CreateWorkspace(ctx, &types.Workspace{
		Name:     "codex-ui",
		RepoPath: repoDir,
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	logPhase("start_session")
	session, err := api.StartWorkspaceSession(ctx, ws.ID, "", client.StartSessionRequest{
		Provider:    "codex",
		WorkspaceID: ws.ID,
		Text:        "Say \"ok\" and nothing else.",
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	logPhase("wait_history_initial")
	waitForHistoryItems(t, api, session.ID, codexIntegrationTimeout())

	model := NewModel(api)
	model.tickFn = func() tea.Cmd { return nil }

	h := newUIHarness(t, &model)
	defer h.Close()
	logPhase("ui_init")
	h.Init()
	h.Resize(120, 40)
	h.SelectSession(session.ID)

	logPhase("ui_send")
	h.SendKey(tea.KeyMsg{Type: tea.KeyEnter})
	h.SetChatInput("Say \"ok\" again.")
	h.SendKey(tea.KeyMsg{Type: tea.KeyEnter})

	logPhase("wait_agent_reply")
	h.WaitForAgentReply(45 * time.Second)
}

func TestUIClaudeStreamingExistingSession(t *testing.T) {
	requireUIIntegration(t)
	requireClaudeIntegration(t)

	start := time.Now()
	phase := "init"
	logPhase := func(next string) {
		phase = next
		t.Logf("phase=%s elapsed=%s", phase, time.Since(start).Truncate(time.Millisecond))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	logPhase("create_workspace")
	repoDir := createClaudeWorkspace(t)

	server, _, _ := newUITestServer(t)
	defer server.Close()

	api := client.NewWithBaseURL(server.URL, "token")

	ws, err := api.CreateWorkspace(ctx, &types.Workspace{
		Name:     "claude-ui",
		RepoPath: repoDir,
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	logPhase("start_session")
	session, err := api.StartWorkspaceSession(ctx, ws.ID, "", client.StartSessionRequest{
		Provider:    "claude",
		WorkspaceID: ws.ID,
		Text:        "Say \"ok\" and nothing else.",
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	logPhase("wait_history_initial")
	waitForHistoryItems(t, api, session.ID, claudeIntegrationTimeout())
	waitForHistoryAgent(t, api, session.ID, "ok", claudeIntegrationTimeout())

	model := NewModel(api)
	model.tickFn = func() tea.Cmd { return nil }

	h := newUIHarness(t, &model)
	defer h.Close()
	logPhase("ui_init")
	h.Init()
	h.Resize(120, 40)
	h.SelectSession(session.ID)

	logPhase("ui_send")
	h.SendKey(tea.KeyMsg{Type: tea.KeyEnter})
	h.SetChatInput("Say \"ok\" again.")
	h.SendKey(tea.KeyMsg{Type: tea.KeyEnter})

	logPhase("wait_agent_reply")
	h.WaitForAgentReply(45 * time.Second)
}

func containsAgentReply(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "### agent") {
			return true
		}
	}
	return false
}

func TestUICodexStreamingNewSession(t *testing.T) {
	requireUIIntegration(t)
	requireCodexIntegration(t)

	start := time.Now()
	phase := "init"
	logPhase := func(next string) {
		phase = next
		t.Logf("phase=%s elapsed=%s", phase, time.Since(start).Truncate(time.Millisecond))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	logPhase("create_workspace")
	repoDir, codexHome := createCodexWorkspace(t)
	writeCodexConfig(t, codexHome, repoDir, "", "", "")
	requireCodexAuth(t, repoDir, codexHome)

	server, _, _ := newUITestServer(t)
	defer server.Close()

	api := client.NewWithBaseURL(server.URL, "token")

	ws, err := api.CreateWorkspace(ctx, &types.Workspace{
		Name:     "codex-ui-new",
		RepoPath: repoDir,
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	model := NewModel(api)
	model.tickFn = func() tea.Cmd { return nil }

	h := newUIHarness(t, &model)
	defer h.Close()
	logPhase("ui_init")
	h.Init()
	h.Resize(120, 40)
	h.SelectWorkspace(ws.ID)

	logPhase("ui_new_session")
	h.SendKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	h.SendKey(tea.KeyMsg{Type: tea.KeyEnter})
	h.SetChatInput("Say \"ok\" and nothing else.")
	h.SendKey(tea.KeyMsg{Type: tea.KeyEnter})

	logPhase("wait_agent_reply")
	h.WaitForAgentReply(45 * time.Second)
}

type uiHarness struct {
	t     *testing.T
	model *Model
}

func newUIHarness(t *testing.T, model *Model) *uiHarness {
	t.Helper()
	return &uiHarness{t: t, model: model}
}

func (h *uiHarness) Close() {
	h.t.Helper()
	if h.model != nil {
		h.model.resetStream()
	}
}

func (h *uiHarness) Init() {
	h.t.Helper()
	h.runCmd(h.model.Init())
}

func (h *uiHarness) Resize(width, height int) {
	h.t.Helper()
	h.apply(tea.WindowSizeMsg{Width: width, Height: height})
}

func (h *uiHarness) SendKey(msg tea.KeyMsg) {
	h.t.Helper()
	h.apply(msg)
}

func (h *uiHarness) SetChatInput(text string) {
	h.t.Helper()
	if h.model.chatInput != nil {
		h.model.chatInput.SetValue(text)
	}
}

func (h *uiHarness) SelectSession(sessionID string) {
	h.t.Helper()
	if h.model.sidebar == nil {
		h.t.Fatalf("sidebar not initialized")
	}
	if !h.model.sidebar.SelectBySessionID(sessionID) {
		h.t.Fatalf("session %s not found in sidebar", sessionID)
	}
	item := h.model.selectedItem()
	if item == nil {
		h.t.Fatalf("selected item missing after select")
	}
	h.runCmd(h.model.loadSelectedSession(item))
	if h.model.pendingSessionKey == "" {
		h.t.Fatalf("pendingSessionKey not set after loadSelectedSession")
	}
	if h.model.loading {
		h.t.Logf("loadSelectedSession: loadingKey=%s", h.model.loadingKey)
	}
	if h.model.pendingApproval != nil {
		h.t.Logf("pendingApproval detected: %s", h.model.pendingApproval.Method)
	}
}

func (h *uiHarness) SelectWorkspace(workspaceID string) {
	h.t.Helper()
	if h.model.sidebar == nil {
		h.t.Fatalf("sidebar not initialized")
	}
	items := h.model.sidebar.Items()
	for i, item := range items {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.kind != sidebarWorkspace || entry.workspace == nil {
			continue
		}
		if entry.workspace.ID == workspaceID {
			h.model.sidebar.Select(i)
			h.runCmd(h.model.onSelectionChanged())
			return
		}
	}
	h.t.Fatalf("workspace %s not found in sidebar", workspaceID)
}

func (h *uiHarness) WaitForAgentReply(timeout time.Duration) {
	h.t.Helper()
	h.WaitFor(func() bool {
		if h.model.mode != uiModeCompose {
			return false
		}
		return containsAgentReply(h.model.currentLines())
	}, timeout)
}

func (h *uiHarness) WaitFor(check func() bool, timeout time.Duration) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	lastLines := 0
	ticks := 0
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		if strings.HasPrefix(h.model.status, "codex error:") {
			h.t.Fatalf("codex stream error: %s", h.model.status)
		}
		if status := strings.ToLower(h.model.status); strings.Contains(status, "context canceled") || strings.Contains(status, "stream error") || strings.Contains(status, "events error") || strings.Contains(status, "items stream error") {
			h.t.Fatalf("stream error surfaced: %s", h.model.status)
		}
		h.apply(tickMsg(time.Now()))
		ticks++
		if ticks%10 == 0 {
			lines := len(h.model.renderedPlain)
			if lines != lastLines {
				h.t.Logf("tick_progress lines=%d status=%s loading=%v", lines, h.model.status, h.model.loading)
				lastLines = lines
			} else {
				h.t.Logf("tick_progress status=%s loading=%v", h.model.status, h.model.loading)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	lines := h.model.currentLines()
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	if len(lines) > 0 {
		h.t.Logf("content_raw_tail:\n%s", strings.Join(lines, "\n"))
	}
	h.t.Fatalf("timeout waiting for UI condition (status=%s loading=%v)", h.model.status, h.model.loading)
}

func (h *uiHarness) apply(msg tea.Msg) {
	h.t.Helper()
	if msg == nil {
		return
	}
	switch m := msg.(type) {
	case historyMsg:
		if m.err != nil {
			h.t.Logf("historyMsg id=%s err=%v key=%s", m.id, m.err, m.key)
		} else {
			h.t.Logf("historyMsg id=%s items=%d key=%s", m.id, len(m.items), m.key)
		}
	case tailMsg:
		if m.err != nil {
			h.t.Logf("tailMsg id=%s err=%v key=%s", m.id, m.err, m.key)
		} else {
			h.t.Logf("tailMsg id=%s items=%d key=%s", m.id, len(m.items), m.key)
		}
	case streamMsg:
		h.t.Logf("streamMsg id=%s err=%v", m.id, m.err)
	case eventsMsg:
		h.t.Logf("eventsMsg id=%s err=%v", m.id, m.err)
	case itemsStreamMsg:
		h.t.Logf("itemsStreamMsg id=%s err=%v", m.id, m.err)
	}
	model, cmd := h.model.Update(msg)
	if updated, ok := model.(*Model); ok && updated != nil {
		h.model = updated
	}
	h.runCmd(cmd)
}

func (h *uiHarness) runCmd(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if msg == nil {
		return
	}
	if cmds, ok := asCmdSlice(msg); ok {
		for _, next := range cmds {
			h.runCmd(next)
		}
		return
	}
	h.apply(msg)
}

func asCmdSlice(msg tea.Msg) ([]tea.Cmd, bool) {
	val := reflect.ValueOf(msg)
	if val.Kind() != reflect.Slice {
		return nil, false
	}
	cmdType := reflect.TypeOf((tea.Cmd)(nil))
	if val.Type().Elem() != cmdType {
		return nil, false
	}
	cmds := make([]tea.Cmd, 0, val.Len())
	for i := 0; i < val.Len(); i++ {
		if cmd, ok := val.Index(i).Interface().(tea.Cmd); ok {
			cmds = append(cmds, cmd)
		}
	}
	return cmds, true
}

func requireUIIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv(uiIntegrationEnv) != "1" {
		t.Skipf("%s=1 not set; skipping UI integration tests", uiIntegrationEnv)
	}
}

func requireClaudeIntegration(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv(claudeIntegrationSkip)) != "" {
		t.Skipf("%s set", claudeIntegrationSkip)
	}
	if os.Getenv(claudeIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run Claude integration tests", claudeIntegrationEnv)
	}
	cmd := strings.TrimSpace(os.Getenv("ARCHON_CLAUDE_CMD"))
	if cmd == "" {
		cmd = "claude"
	}
	if _, err := exec.LookPath(cmd); err != nil {
		t.Fatalf("claude command not found (%s): %v", cmd, err)
	}
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

func claudeIntegrationTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("ARCHON_CLAUDE_TIMEOUT")); raw != "" {
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
	codexHome := filepath.Join(repoDir, ".archon")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	return repoDir, codexHome
}

func createClaudeWorkspace(t *testing.T) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o700); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	return repoDir
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
		client, err := startTestCodexAppServer(ctx, repoDir, codexHome)
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
		client, err := startTestCodexAppServer(ctx, repoDir, codexHome)
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

type testCodexAppServer struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	reader  *bufio.Scanner
	mu      sync.Mutex
	nextID  int
	pending map[int]chan rpcMessage
}

type rpcMessage struct {
	ID     *int            `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func startTestCodexAppServer(ctx context.Context, cwd, codexHome string) (*testCodexAppServer, error) {
	cmdName := strings.TrimSpace(os.Getenv("ARCHON_CODEX_CMD"))
	if cmdName == "" {
		cmdName = "codex"
	}
	cmd := exec.Command(cmdName, "app-server")
	if cwd != "" {
		cmd.Dir = cwd
	}
	if strings.TrimSpace(codexHome) != "" {
		cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go io.Copy(io.Discard, stderr)

	server := &testCodexAppServer{
		cmd:     cmd,
		stdin:   stdin,
		reader:  bufio.NewScanner(stdout),
		nextID:  1,
		pending: make(map[int]chan rpcMessage),
	}
	server.reader.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	go server.readLoop()

	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := server.initialize(initCtx); err != nil {
		server.Close()
		return nil, err
	}
	return server, nil
}

func (c *testCodexAppServer) Close() {
	if c == nil {
		return
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
}

func (c *testCodexAppServer) initialize(ctx context.Context) error {
	params := map[string]any{
		"clientInfo": map[string]any{
			"name":    "archon_test",
			"title":   "Archon Test",
			"version": "dev",
		},
	}
	if err := c.request(ctx, "initialize", params, nil); err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func (c *testCodexAppServer) request(ctx context.Context, method string, params any, out any) error {
	id := c.nextRequestID()
	ch := make(chan rpcMessage, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.send(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case msg := <-ch:
		if msg.Error != nil {
			return errors.New(msg.Error.Message)
		}
		if out != nil && len(msg.Result) > 0 {
			return json.Unmarshal(msg.Result, out)
		}
		return nil
	}
}

func (c *testCodexAppServer) notify(method string, params any) error {
	return c.send(map[string]any{"method": method, "params": params})
}

func (c *testCodexAppServer) send(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

func (c *testCodexAppServer) nextRequestID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	return id
}

func (c *testCodexAppServer) readLoop() {
	for c.reader.Scan() {
		line := strings.TrimSpace(c.reader.Text())
		if line == "" {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.ID == nil {
			continue
		}
		c.mu.Lock()
		ch := c.pending[*msg.ID]
		delete(c.pending, *msg.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- msg
			close(ch)
		}
	}
}

func newUITestServer(t *testing.T) (*httptest.Server, *daemon.SessionManager, *daemon.Stores) {
	t.Helper()
	base := t.TempDir()
	manager, err := daemon.NewSessionManager(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	workspaces := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	state := store.NewFileAppStateStore(filepath.Join(base, "state.json"))
	keymap := store.NewFileKeymapStore(filepath.Join(base, "keymap.json"))
	meta := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))
	sessions := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))
	approvals := store.NewFileApprovalStore(filepath.Join(base, "approvals.json"))

	stores := &daemon.Stores{
		Workspaces:  workspaces,
		Worktrees:   workspaces,
		AppState:    state,
		Keymap:      keymap,
		SessionMeta: meta,
		Sessions:    sessions,
		Approvals:   approvals,
	}

	manager.SetMetaStore(meta)
	manager.SetSessionStore(sessions)

	logger := logging.New(os.Stdout, logging.Debug)
	api := &daemon.API{
		Version: "test",
		Manager: manager,
		Stores:  stores,
		Logger:  logger,
	}
	api.LiveCodex = daemon.NewCodexLiveManager(stores, logger)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(daemon.TokenAuthMiddleware("token", mux))
	return server, manager, stores
}

func waitForHistoryItems(t *testing.T, api *client.Client, sessionID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		history, err := api.History(ctx, sessionID, 200)
		cancel()
		if err == nil && len(history.Items) > 0 {
			return
		}
		if err != nil {
			lastErr = err
			t.Logf("history poll error: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("timeout waiting for history items (last_err=%v)", lastErr)
	}
	t.Fatalf("timeout waiting for history items")
}

func waitForHistoryAgent(t *testing.T, api *client.Client, sessionID, needle string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		history, err := api.History(ctx, sessionID, 200)
		cancel()
		if err == nil && historyHasAgentText(history.Items, needle) {
			return
		}
		if err != nil {
			lastErr = err
			t.Logf("history poll error: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("timeout waiting for history agent text (last_err=%v)", lastErr)
	}
	t.Fatalf("timeout waiting for history agent text")
}

func historyHasAgentText(items []map[string]any, needle string) bool {
	if len(items) == 0 || strings.TrimSpace(needle) == "" {
		return false
	}
	needle = strings.ToLower(needle)
	for _, item := range items {
		if item == nil {
			continue
		}
		typ, _ := item["type"].(string)
		if typ != "agentMessage" && typ != "agentMessageDelta" && typ != "assistant" {
			continue
		}
		if text := extractHistoryText(item); text != "" {
			if strings.Contains(strings.ToLower(text), needle) {
				return true
			}
		}
	}
	return false
}

func extractHistoryText(item map[string]any) string {
	if item == nil {
		return ""
	}
	if text, _ := item["text"].(string); strings.TrimSpace(text) != "" {
		return text
	}
	content, ok := item["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, entry := range content {
		block, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if text, _ := block["text"].(string); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}
