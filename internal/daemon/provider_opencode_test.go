package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"control/internal/config"
	"control/internal/types"
)

type openCodePermissionRequesterFunc func(ctx context.Context, sessionID string, permission *openCodePermissionCreateRequest, directory string) error

func (f openCodePermissionRequesterFunc) RequestPermission(ctx context.Context, sessionID string, permission *openCodePermissionCreateRequest, directory string) error {
	return f(ctx, sessionID, permission, directory)
}

func TestResolveOpenCodeClientConfigEnvOverridesToken(t *testing.T) {
	t.Setenv("CUSTOM_OPENCODE_TOKEN", "env-token")
	t.Setenv("KILOCODE_TOKEN", "kilo-env-token")
	coreCfg := config.CoreConfig{
		Providers: config.CoreProvidersConfig{
			OpenCode: config.CoreOpenCodeProviderConfig{
				BaseURL:  "http://127.0.0.1:4096",
				Token:    "config-token",
				TokenEnv: "CUSTOM_OPENCODE_TOKEN",
				Username: "archon",
			},
			KiloCode: config.CoreOpenCodeProviderConfig{
				BaseURL:  "http://127.0.0.1:4097",
				Token:    "config-kilo-token",
				Username: "archon-kilo",
			},
		},
	}

	opencode := resolveOpenCodeClientConfig("opencode", coreCfg)
	if opencode.Token != "env-token" {
		t.Fatalf("expected opencode env token override, got %q", opencode.Token)
	}
	if opencode.Username != "archon" {
		t.Fatalf("unexpected opencode username: %q", opencode.Username)
	}
	if opencode.Timeout != 30*time.Second {
		t.Fatalf("expected opencode default timeout 30s, got %s", opencode.Timeout)
	}

	kilocode := resolveOpenCodeClientConfig("kilocode", coreCfg)
	if kilocode.Token != "kilo-env-token" {
		t.Fatalf("expected kilocode env token override, got %q", kilocode.Token)
	}
	if kilocode.Username != "archon-kilo" {
		t.Fatalf("unexpected kilocode username: %q", kilocode.Username)
	}
	if kilocode.Timeout != 30*time.Second {
		t.Fatalf("expected kilocode default timeout 30s, got %s", kilocode.Timeout)
	}
}

func TestOpenCodeProviderStartSendAndInterrupt(t *testing.T) {
	var (
		createCalls     atomic.Int32
		permissionCalls atomic.Int32
		promptCalls     atomic.Int32
		abortCalls      atomic.Int32
		lastAuthSeen    atomic.Value
		requestMu       sync.Mutex
		requestPaths    []string
		permissionBody  map[string]any
	)
	const directory = "/tmp/opencode-provider-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuthSeen.Store(r.Header.Get("Authorization"))
		requestMu.Lock()
		requestPaths = append(requestPaths, r.URL.Path)
		requestMu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			createCalls.Add(1)
			writeJSON(w, http.StatusCreated, map[string]any{"id": "sess_123"})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_123/permissions":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			permissionCalls.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			requestMu.Lock()
			permissionBody = body
			requestMu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_123/prompt":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			promptCalls.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			writeJSON(w, http.StatusOK, map[string]any{
				"parts": []map[string]any{
					{"type": "text", "text": "assistant reply"},
				},
			})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_123/abort":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			abortCalls.Add(1)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	home := filepath.Join(t.TempDir(), "home")
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`
[providers.opencode]
base_url = "` + server.URL + `"
token = "config-token"
token_env = "OPENCODE_TOKEN"
username = "archon"
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("OPENCODE_TOKEN", "env-token")

	provider, err := ResolveProvider("opencode", "")
	if err != nil {
		t.Fatalf("ResolveProvider(opencode): %v", err)
	}
	if provider.Name() != "opencode" {
		t.Fatalf("unexpected provider name: %q", provider.Name())
	}
	if provider.Command() != server.URL {
		t.Fatalf("expected provider command to be base url, got %q", provider.Command())
	}

	itemSink := &testItemSink{}
	proc, err := provider.Start(StartSessionConfig{
		Provider:    "opencode",
		Cwd:         directory,
		InitialText: "hello there",
		AdditionalDirectories: []string{
			"/tmp/backend",
			"/tmp/shared",
		},
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model: "anthropic/claude-sonnet-4-20250514",
		},
	}, &testProviderLogSink{}, itemSink)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if createCalls.Load() != 1 {
		t.Fatalf("expected one session create call, got %d", createCalls.Load())
	}
	if permissionCalls.Load() != 1 {
		t.Fatalf("expected one permission call, got %d", permissionCalls.Load())
	}
	deadline := time.Now().Add(2 * time.Second)
	for promptCalls.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if promptCalls.Load() < 1 {
		t.Fatalf("expected async prompt call for initial text, got %d", promptCalls.Load())
	}
	requestMu.Lock()
	pathsSnapshot := append([]string(nil), requestPaths...)
	permissionSnapshot := map[string]any{}
	for key, value := range permissionBody {
		permissionSnapshot[key] = value
	}
	requestMu.Unlock()
	createIdx := -1
	permissionIdx := -1
	promptIdx := -1
	for idx, path := range pathsSnapshot {
		switch path {
		case "/session":
			if createIdx == -1 {
				createIdx = idx
			}
		case "/session/sess_123/permissions":
			if permissionIdx == -1 {
				permissionIdx = idx
			}
		case "/session/sess_123/prompt":
			if promptIdx == -1 {
				promptIdx = idx
			}
		}
	}
	if createIdx < 0 || permissionIdx < 0 || promptIdx < 0 {
		t.Fatalf("expected create, permission, and prompt requests, got %v", pathsSnapshot)
	}
	if !(createIdx < permissionIdx && permissionIdx < promptIdx) {
		t.Fatalf("expected request order create -> permission -> prompt, got %v", pathsSnapshot)
	}
	if got := strings.TrimSpace(asString(permissionSnapshot["permission"])); got != "external_directory" {
		t.Fatalf("unexpected permission payload: %#v", permissionSnapshot)
	}
	patterns, _ := permissionSnapshot["patterns"].([]any)
	if len(patterns) != 2 || strings.TrimSpace(asString(patterns[0])) != "/tmp/backend/*" || strings.TrimSpace(asString(patterns[1])) != "/tmp/shared/*" {
		t.Fatalf("unexpected permission patterns: %#v", permissionSnapshot)
	}
	if proc.Send == nil {
		t.Fatalf("expected send function")
	}
	if err := proc.Send(buildOpenCodeUserPayloadWithRuntime("again", nil)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if promptCalls.Load() != 2 {
		t.Fatalf("expected second prompt call after send, got %d", promptCalls.Load())
	}
	if itemSink.Len() < 4 {
		t.Fatalf("expected user/assistant items to be appended, got %d", itemSink.Len())
	}

	if proc.Interrupt == nil {
		t.Fatalf("expected interrupt function")
	}
	if err := proc.Interrupt(); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if abortCalls.Load() != 1 {
		t.Fatalf("expected one abort call, got %d", abortCalls.Load())
	}

	auth, _ := lastAuthSeen.Load().(string)
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("archon:env-token"))
	if strings.TrimSpace(auth) != wantAuth {
		t.Fatalf("unexpected auth header: %q", auth)
	}
}

func TestOpenCodeProviderResumeRequiresProviderSessionID(t *testing.T) {
	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: "http://127.0.0.1:4096"})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	provider := &openCodeProvider{
		providerName: "opencode",
		client:       client,
	}
	_, err = provider.Start(StartSessionConfig{
		Provider: "opencode",
		Resume:   true,
	}, &testProviderLogSink{}, &testItemSink{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "provider session id") {
		t.Fatalf("expected provider session id validation error, got %v", err)
	}
}

func TestOpenCodeProviderStartPermissionRequestFailure(t *testing.T) {
	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: "http://127.0.0.1:4096"})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	provider := &openCodeProvider{
		providerName: "opencode",
		client:       client,
		permissionRequester: openCodePermissionRequesterFunc(func(_ context.Context, _ string, _ *openCodePermissionCreateRequest, _ string) error {
			return errors.New("permission request failed")
		}),
	}
	_, err = provider.Start(StartSessionConfig{
		Provider:          "opencode",
		Resume:            true,
		ProviderSessionID: "sess_123",
		Cwd:               "/tmp/opencode-provider-worktree",
		AdditionalDirectories: []string{
			"/tmp/backend",
		},
	}, &testProviderLogSink{}, &testItemSink{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "permission request failed") {
		t.Fatalf("expected permission requester error, got %v", err)
	}
}

func TestOpenCodeProviderStartResumeUsesClientPermissionRequesterFallback(t *testing.T) {
	var (
		createCalls     atomic.Int32
		permissionCalls atomic.Int32
	)
	const directory = "/tmp/opencode-provider-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			createCalls.Add(1)
			writeJSON(w, http.StatusCreated, map[string]any{"id": "sess_123"})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_123/permissions":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			permissionCalls.Add(1)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	provider := &openCodeProvider{
		providerName:        "opencode",
		client:              client,
		permissionRequester: nil, // exercise fallback-to-client branch in Start
	}
	proc, err := provider.Start(StartSessionConfig{
		Provider:          "opencode",
		Resume:            true,
		ProviderSessionID: "sess_123",
		Cwd:               directory,
		AdditionalDirectories: []string{
			"/tmp/backend",
		},
	}, &testProviderLogSink{}, &testItemSink{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if proc == nil {
		t.Fatalf("expected provider process")
	}
	if createCalls.Load() != 0 {
		t.Fatalf("expected no create call on resume, got %d", createCalls.Load())
	}
	if permissionCalls.Load() != 1 {
		t.Fatalf("expected one permission call, got %d", permissionCalls.Load())
	}
}

func TestKiloCodeProviderStartResumeRequestsAdditionalDirectoryPermission(t *testing.T) {
	var permissionCalls atomic.Int32
	const directory = "/tmp/kilocode-provider-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session/kilo_123/permissions":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			permissionCalls.Add(1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if got := strings.TrimSpace(asString(body["permission"])); got != "external_directory" {
				http.Error(w, "unexpected permission", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	provider := &openCodeProvider{
		providerName: "kilocode",
		client:       client,
	}
	proc, err := provider.Start(StartSessionConfig{
		Provider:          "kilocode",
		Resume:            true,
		ProviderSessionID: "kilo_123",
		Cwd:               directory,
		AdditionalDirectories: []string{
			"/tmp/backend",
		},
	}, &testProviderLogSink{}, &testItemSink{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if proc == nil {
		t.Fatalf("expected provider process")
	}
	if permissionCalls.Load() != 1 {
		t.Fatalf("expected one permission call, got %d", permissionCalls.Load())
	}
}

func TestOpenCodePayloadValidation(t *testing.T) {
	if _, _, err := extractOpenCodeSendRequest([]byte("{broken")); err == nil {
		t.Fatalf("expected invalid json error")
	}
	if _, _, err := extractOpenCodeSendRequest([]byte(`{"type":"assistant"}`)); err == nil {
		t.Fatalf("expected unsupported payload type error")
	}
	text, runtimeOptions, err := extractOpenCodeSendRequest(buildOpenCodeUserPayloadWithRuntime("hello", &types.SessionRuntimeOptions{Model: "x"}))
	if err != nil {
		t.Fatalf("extractOpenCodeSendRequest: %v", err)
	}
	if text != "hello" {
		t.Fatalf("unexpected text: %q", text)
	}
	if runtimeOptions == nil || runtimeOptions.Model != "x" {
		t.Fatalf("unexpected runtime options: %#v", runtimeOptions)
	}
}

func TestOpenCodeClientAbortValidation(t *testing.T) {
	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: "http://127.0.0.1:4096"})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	if err := client.AbortSession(context.Background(), "   ", ""); err == nil {
		t.Fatalf("expected session id validation error")
	}
}
