package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/apicode"
	"control/internal/logging"
	"control/internal/types"
)

type stubCodexFileSearchEnvironmentResolver struct {
	env codexFileSearchEnvironment
	err error
}

func (r stubCodexFileSearchEnvironmentResolver) Environment(context.Context, types.FileSearchScope) (codexFileSearchEnvironment, error) {
	if r.err != nil {
		return codexFileSearchEnvironment{}, r.err
	}
	return r.env, nil
}

type stubCodexFileSearchClientFactory struct {
	client codexFileSearchClient
	err    error
}

func (f stubCodexFileSearchClientFactory) Client(context.Context, string, string, logging.Logger) (codexFileSearchClient, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.client, nil
}

type stubCodexFileSearchClient struct {
	searchFunc func(ctx context.Context, query string, roots []string) (*codexFuzzyFileSearchResponse, error)
	closeCount int
}

func (c *stubCodexFileSearchClient) FuzzyFileSearch(ctx context.Context, query string, roots []string) (*codexFuzzyFileSearchResponse, error) {
	if c.searchFunc == nil {
		return &codexFuzzyFileSearchResponse{}, nil
	}
	return c.searchFunc(ctx, query, roots)
}

func (c *stubCodexFileSearchClient) Close() {
	c.closeCount++
}

type stubCodexFileSearchClientManager struct {
	client     codexFileSearchClient
	clientErr  error
	closeErr   error
	closeCount int
	calls      []codexFileSearchEnvironment
}

func (m *stubCodexFileSearchClientManager) Client(_ context.Context, env codexFileSearchEnvironment) (codexFileSearchClient, error) {
	m.calls = append(m.calls, env)
	if m.clientErr != nil {
		return nil, m.clientErr
	}
	return m.client, nil
}

func (m *stubCodexFileSearchClientManager) Close() error {
	m.closeCount++
	return m.closeErr
}

type stubCodexFileSearchResultMapper struct {
	candidates []types.FileSearchCandidate
}

func (m stubCodexFileSearchResultMapper) Map([]FileSearchRoot, *codexFuzzyFileSearchResponse, int) []types.FileSearchCandidate {
	return append([]types.FileSearchCandidate(nil), m.candidates...)
}

type stubCodexFileSearchErrorMapper struct {
	mapFunc func(error) error
}

func (m stubCodexFileSearchErrorMapper) Map(err error) error {
	if m.mapFunc == nil {
		return err
	}
	return m.mapFunc(err)
}

type stubFileSearchRootContextLoader struct {
	context fileSearchRootContext
	err     error
}

func (l stubFileSearchRootContextLoader) Load(context.Context, types.FileSearchScope) (fileSearchRootContext, error) {
	if l.err != nil {
		return fileSearchRootContext{}, l.err
	}
	return l.context, nil
}

type stubCodexHomeResolver struct {
	resolveFunc func(cwd, workspacePath string) string
}

func (r stubCodexHomeResolver) Resolve(cwd, workspacePath string) string {
	if r.resolveFunc == nil {
		return ""
	}
	return r.resolveFunc(cwd, workspacePath)
}

func TestCodexInitializeParamsIncludesExperimentalCapability(t *testing.T) {
	params := codexInitializeParams(codexInitializeOptions{
		ClientName:      "archon_test",
		ClientTitle:     "Archon Test",
		ClientVersion:   "1.2.3",
		ExperimentalAPI: true,
	})
	capabilities, ok := params["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("expected initialize capabilities, got %#v", params)
	}
	if enabled, _ := capabilities["experimentalApi"].(bool); !enabled {
		t.Fatalf("expected experimental api capability, got %#v", capabilities)
	}
}

func TestCodexAppServerInitializeWithExperimentalCapability(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	client := &codexAppServer{
		stdin:  stdinWriter,
		reader: bufio.NewReader(stdoutReader),
		nextID: 1,
		msgs:   make(chan rpcMessage, 8),
		notes:  make(chan rpcMessage, 8),
		reqs:   make(chan rpcMessage, 4),
		errs:   make(chan error, 1),
		reqMap: make(map[int]requestInfo),
	}
	go client.readLoop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdinReader)
		encoder := json.NewEncoder(stdoutWriter)
		for scanner.Scan() {
			var msg map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				continue
			}
			method, _ := msg["method"].(string)
			switch method {
			case "initialize":
				params, _ := msg["params"].(map[string]any)
				capabilities, _ := params["capabilities"].(map[string]any)
				if enabled, _ := capabilities["experimentalApi"].(bool); !enabled {
					t.Errorf("expected initialize.experimentalApi=true, got %#v", params)
				}
				id, _ := msg["id"].(float64)
				_ = encoder.Encode(map[string]any{"id": int(id), "result": map[string]any{"userAgent": "codex-test"}})
			case "initialized":
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.initializeWithOptions(ctx, codexInitializeOptions{ExperimentalAPI: true}); err != nil {
		t.Fatalf("initializeWithOptions: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for initialize exchange")
	}
}

func TestCodexAppServerFuzzyFileSearchUsesExpectedRequest(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	client := &codexAppServer{
		stdin:  stdinWriter,
		reader: bufio.NewReader(stdoutReader),
		nextID: 1,
		msgs:   make(chan rpcMessage, 8),
		notes:  make(chan rpcMessage, 8),
		reqs:   make(chan rpcMessage, 4),
		errs:   make(chan error, 1),
		reqMap: make(map[int]requestInfo),
	}
	go client.readLoop()

	go func() {
		scanner := bufio.NewScanner(stdinReader)
		encoder := json.NewEncoder(stdoutWriter)
		for scanner.Scan() {
			var msg map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				continue
			}
			if method, _ := msg["method"].(string); method == "fuzzyFileSearch" {
				params, _ := msg["params"].(map[string]any)
				query, _ := params["query"].(string)
				if query != "main" {
					t.Errorf("unexpected query: %q", query)
				}
				id, _ := msg["id"].(float64)
				_ = encoder.Encode(map[string]any{
					"id": int(id),
					"result": map[string]any{
						"files": []map[string]any{{
							"file_name": "main.go",
							"path":      "cmd/main.go",
							"root":      "/repo",
							"score":     99,
						}},
					},
				})
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := client.FuzzyFileSearch(ctx, "main", []string{"/repo", "/repo"})
	if err != nil {
		t.Fatalf("FuzzyFileSearch: %v", err)
	}
	if result == nil || len(result.Files) != 1 || result.Files[0].Path != "cmd/main.go" {
		t.Fatalf("unexpected fuzzy search result: %#v", result)
	}
}

func TestCodexAppServerRequestReturnsTypedRPCError(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	client := &codexAppServer{
		stdin:  stdinWriter,
		reader: bufio.NewReader(stdoutReader),
		nextID: 1,
		msgs:   make(chan rpcMessage, 8),
		notes:  make(chan rpcMessage, 8),
		reqs:   make(chan rpcMessage, 4),
		errs:   make(chan error, 1),
		reqMap: make(map[int]requestInfo),
	}
	go client.readLoop()

	go func() {
		scanner := bufio.NewScanner(stdinReader)
		encoder := json.NewEncoder(stdoutWriter)
		for scanner.Scan() {
			var msg map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				continue
			}
			if method, _ := msg["method"].(string); method == "fuzzyFileSearch" {
				id, _ := msg["id"].(float64)
				_ = encoder.Encode(map[string]any{
					"id": int(id),
					"error": map[string]any{
						"code":    -32601,
						"message": "method not found",
					},
				})
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.FuzzyFileSearch(ctx, "main", []string{"/repo"})
	var rpcErr *codexRPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected typed rpc error, got %v", err)
	}
	if rpcErr.Code != -32601 || rpcErr.Message != "method not found" {
		t.Fatalf("unexpected rpc error: %#v", rpcErr)
	}
}

func TestCodexAppServerRequestContextCancellation(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, _ := io.Pipe()
	client := &codexAppServer{
		stdin:  stdinWriter,
		reader: bufio.NewReader(stdoutReader),
		nextID: 1,
		msgs:   make(chan rpcMessage, 8),
		notes:  make(chan rpcMessage, 8),
		reqs:   make(chan rpcMessage, 4),
		errs:   make(chan error, 1),
		reqMap: make(map[int]requestInfo),
	}
	go client.readLoop()
	go func() {
		scanner := bufio.NewScanner(stdinReader)
		for scanner.Scan() {
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.FuzzyFileSearch(ctx, "main", []string{"/repo"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestCodexAppServerReadLoopSkipsMalformedJSONAndContinues(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	client := &codexAppServer{
		stdin:  stdinWriter,
		reader: bufio.NewReader(stdoutReader),
		nextID: 1,
		msgs:   make(chan rpcMessage, 8),
		notes:  make(chan rpcMessage, 8),
		reqs:   make(chan rpcMessage, 4),
		errs:   make(chan error, 1),
		reqMap: make(map[int]requestInfo),
	}
	go client.readLoop()

	go func() {
		scanner := bufio.NewScanner(stdinReader)
		encoder := json.NewEncoder(stdoutWriter)
		for scanner.Scan() {
			var msg map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				continue
			}
			if method, _ := msg["method"].(string); method == "fuzzyFileSearch" {
				id, _ := msg["id"].(float64)
				_, _ = stdoutWriter.Write([]byte("{not json}\n"))
				_ = encoder.Encode(map[string]any{
					"id": int(id),
					"result": map[string]any{
						"files": []map[string]any{{
							"file_name": "main.go",
							"path":      "cmd/main.go",
							"root":      "/repo",
							"score":     99,
						}},
					},
				})
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := client.FuzzyFileSearch(ctx, "main", []string{"/repo"})
	if err != nil {
		t.Fatalf("FuzzyFileSearch after malformed line: %v", err)
	}
	if result == nil || len(result.Files) != 1 {
		t.Fatalf("unexpected result after malformed line: %#v", result)
	}
}

func TestStartCodexAppServerWithOptionsEnablesExperimentalAPI(t *testing.T) {
	wrapperDir := t.TempDir()
	logFile := filepath.Join(wrapperDir, "codex-init.json")
	script := filepath.Join(wrapperDir, "codex")
	testBin := os.Args[0]
	shell := "#!/bin/sh\nexec \"" + testBin + "\" -test.run=TestCodexAppServerHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(script, []byte(shell), 0o755); err != nil {
		t.Fatalf("write helper wrapper: %v", err)
	}
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GO_WANT_CODEX_APP_SERVER_HELPER_PROCESS", "1")
	t.Setenv("ARCHON_CODEX_APP_SERVER_HELPER_LOG", logFile)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := startCodexAppServerWithOptions(ctx, "", "", logging.Nop(), codexInitializeOptions{
		ClientName:      "archon_test",
		ClientTitle:     "Archon Test",
		ClientVersion:   "1.0.0",
		ExperimentalAPI: true,
	})
	if err != nil {
		t.Fatalf("startCodexAppServerWithOptions: %v", err)
	}
	client.Close()

	data, err := os.ReadFile(filepath.Clean(logFile))
	if err != nil {
		t.Fatalf("read helper log: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode helper log: %v", err)
	}
	params, _ := payload["params"].(map[string]any)
	capabilities, _ := params["capabilities"].(map[string]any)
	if enabled, _ := capabilities["experimentalApi"].(bool); !enabled {
		t.Fatalf("expected helper to observe experimental api initialize params, got %#v", payload)
	}
}

func TestCodexAppServerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CODEX_APP_SERVER_HELPER_PROCESS") != "1" {
		return
	}
	if len(os.Args) < 3 || os.Args[len(os.Args)-1] != "app-server" {
		os.Exit(2)
	}
	logFile := strings.TrimSpace(os.Getenv("ARCHON_CODEX_APP_SERVER_HELPER_LOG"))
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		method, _ := msg["method"].(string)
		switch method {
		case "initialize":
			if logFile != "" {
				data, _ := json.Marshal(msg)
				_ = os.WriteFile(logFile, data, 0o600)
			}
			id, _ := msg["id"].(float64)
			_ = encoder.Encode(map[string]any{"id": int(id), "result": map[string]any{"userAgent": "codex-helper"}})
		case "initialized":
			select {}
		}
	}
	os.Exit(0)
}

func TestDaemonCodexFileSearchEnvironmentResolverUsesWorkspaceRepoForCodexHome(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	subdir := filepath.Join(repo, "subdir")
	codexHome := filepath.Join(repo, ".archon")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}

	resolver := daemonCodexFileSearchEnvironmentProvider{
		rootResolver: stubFileSearchRootResolver{roots: []FileSearchRoot{{
			Path:        subdir,
			DisplayBase: subdir,
			Primary:     true,
		}}},
		contextLoader: stubFileSearchRootContextLoader{
			context: fileSearchRootContext{
				workspace: &types.Workspace{RepoPath: repo},
			},
		},
		homeResolver: defaultCodexHomeResolver{},
	}

	env, err := resolver.Environment(context.Background(), types.FileSearchScope{Provider: "codex", WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if env.Cwd != filepath.Clean(subdir) {
		t.Fatalf("expected cwd %q, got %#v", filepath.Clean(subdir), env)
	}
	if env.CodexHome != filepath.Clean(codexHome) {
		t.Fatalf("expected codex home %q, got %#v", filepath.Clean(codexHome), env)
	}
}

func TestDaemonCodexFileSearchEnvironmentResolverUsesHomeResolverPort(t *testing.T) {
	resolver := daemonCodexFileSearchEnvironmentProvider{
		rootResolver: stubFileSearchRootResolver{roots: []FileSearchRoot{{
			Path:        "/repo/app",
			DisplayBase: "/repo/app",
			Primary:     true,
		}}},
		homeResolver: stubCodexHomeResolver{
			resolveFunc: func(cwd, workspacePath string) string {
				if cwd != "/repo/app" || workspacePath != "" {
					t.Fatalf("unexpected home resolver args cwd=%q workspacePath=%q", cwd, workspacePath)
				}
				return "/custom/.archon"
			},
		},
	}
	env, err := resolver.Environment(context.Background(), types.FileSearchScope{Provider: "codex", Cwd: "/repo/app"})
	if err != nil {
		t.Fatalf("Environment: %v", err)
	}
	if env.CodexHome != "/custom/.archon" {
		t.Fatalf("expected injected home resolver result, got %#v", env)
	}
}

func TestCodexFileSearchEnvironmentProviderDefaultAndFailureBranches(t *testing.T) {
	provider := codexFileSearchEnvironmentProviderOrDefault(nil)
	env, err := provider.Environment(context.Background(), types.FileSearchScope{Provider: "codex", Cwd: t.TempDir()})
	if err != nil {
		t.Fatalf("default environment provider: %v", err)
	}
	if env.Cwd == "" || len(env.Roots) != 1 {
		t.Fatalf("unexpected default environment: %#v", env)
	}

	provider = daemonCodexFileSearchEnvironmentProvider{
		rootResolver: stubFileSearchRootResolver{err: errors.New("boom")},
	}
	if _, err := provider.Environment(context.Background(), types.FileSearchScope{Provider: "codex", Cwd: "/repo"}); err == nil {
		t.Fatal("expected root resolver failure")
	}

	provider = daemonCodexFileSearchEnvironmentProvider{
		rootResolver: stubFileSearchRootResolver{},
	}
	if _, err := provider.Environment(context.Background(), types.FileSearchScope{Provider: "codex", Cwd: "/repo"}); err == nil {
		t.Fatal("expected empty roots error")
	}

	provider = daemonCodexFileSearchEnvironmentProvider{
		rootResolver: stubFileSearchRootResolver{roots: []FileSearchRoot{{Path: "/repo/app", Primary: true}}},
	}
	env, err = provider.Environment(context.Background(), types.FileSearchScope{Provider: "codex", Cwd: "/repo/app"})
	if err != nil {
		t.Fatalf("nil home resolver fallback: %v", err)
	}
	if env.Cwd != "/repo/app" {
		t.Fatalf("unexpected fallback environment: %#v", env)
	}
}

func TestDefaultCodexFileSearchResultMapperUsesScoresAndDisplayPaths(t *testing.T) {
	candidates := defaultCodexFileSearchResultMapper{}.Map([]FileSearchRoot{
		{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
		{Path: "/repo/shared", DisplayBase: "/repo/app"},
	}, &codexFuzzyFileSearchResponse{
		Files: []codexFuzzyFileSearchResult{
			{FileName: "main.go", Path: "cmd/main.go", Root: "/repo/app", Score: 90},
			{FileName: "util.go", Path: "pkg/util.go", Root: "/repo/shared", Score: 80},
			{FileName: "main.go", Path: "/repo/app/cmd/main.go", Root: "/repo/app", Score: 95},
		},
	}, 10)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 merged candidates, got %#v", candidates)
	}
	if candidates[0].Path != "/repo/app/cmd/main.go" || candidates[0].DisplayPath != "cmd/main.go" || candidates[0].Score != 95 {
		t.Fatalf("unexpected primary candidate: %#v", candidates[0])
	}
	if candidates[1].DisplayPath != "../shared/pkg/util.go" {
		t.Fatalf("expected shared root display path, got %#v", candidates[1])
	}
}

func TestDefaultCodexFileSearchResultMapperSkipsUnknownsAndTruncates(t *testing.T) {
	candidates := defaultCodexFileSearchResultMapper{}.Map([]FileSearchRoot{
		{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
	}, &codexFuzzyFileSearchResponse{
		Files: []codexFuzzyFileSearchResult{
			{FileName: "a.go", Path: "a.go", Root: "/repo/app", Score: 10},
			{FileName: "bad.go", Path: "", Root: "/repo/app", Score: 90},
			{FileName: "ghost.go", Path: "ghost.go", Root: "/repo/ghost", Score: 100},
			{FileName: "b.go", Path: "b.go", Root: "/repo/app", Score: 20},
		},
	}, 1)
	if len(candidates) != 1 || candidates[0].Path != "/repo/app/b.go" {
		t.Fatalf("expected filtered + truncated candidates, got %#v", candidates)
	}
}

func TestReusableCodexFileSearchClientManagerReusesAndRotatesClients(t *testing.T) {
	first := &stubCodexFileSearchClient{}
	second := &stubCodexFileSearchClient{}
	manager := newReusableCodexFileSearchClientManager(stubCodexFileSearchClientFactory{
		client: first,
	}, nil).(*reusableCodexFileSearchClientManager)

	client, err := manager.Client(context.Background(), codexFileSearchEnvironment{Cwd: "/repo/a", CodexHome: "/repo/.archon"})
	if err != nil {
		t.Fatalf("Client: %v", err)
	}
	if client != first {
		t.Fatalf("expected first client, got %#v", client)
	}
	client, err = manager.Client(context.Background(), codexFileSearchEnvironment{Cwd: "/repo/a", CodexHome: "/repo/.archon"})
	if err != nil {
		t.Fatalf("Client reuse: %v", err)
	}
	if client != first {
		t.Fatalf("expected reused client, got %#v", client)
	}
	manager.factory = stubCodexFileSearchClientFactory{client: second}
	client, err = manager.Client(context.Background(), codexFileSearchEnvironment{Cwd: "/repo/b", CodexHome: "/repo/.archon"})
	if err != nil {
		t.Fatalf("Client rotate: %v", err)
	}
	if client != second {
		t.Fatalf("expected rotated client, got %#v", client)
	}
	if first.closeCount == 0 {
		t.Fatal("expected prior client to be closed on rotation")
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if second.closeCount == 0 {
		t.Fatal("expected active client to be closed on manager close")
	}
}

func TestReusableCodexFileSearchClientManagerNilAndFactoryErrorBranches(t *testing.T) {
	var nilManager *reusableCodexFileSearchClientManager
	if _, err := nilManager.Client(context.Background(), codexFileSearchEnvironment{}); err == nil {
		t.Fatal("expected nil manager client error")
	}
	if err := nilManager.Close(); err != nil {
		t.Fatalf("nil manager close should be safe: %v", err)
	}

	manager := newReusableCodexFileSearchClientManager(stubCodexFileSearchClientFactory{err: errors.New("boom")}, nil)
	if _, err := manager.Client(context.Background(), codexFileSearchEnvironment{Cwd: "/repo"}); err == nil {
		t.Fatal("expected factory error")
	}
}

func TestDefaultCodexFileSearchErrorMapperMapsUnsupportedRPC(t *testing.T) {
	err := (defaultCodexFileSearchErrorMapper{}).Map(&codexRPCError{Code: -32601, Message: "method not found"})
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("expected service error, got %v", err)
	}
	if serviceErr.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("expected unsupported code, got %#v", serviceErr)
	}
}

func TestDefaultCodexFileSearchErrorMapperPassesThroughServiceErrors(t *testing.T) {
	original := unavailableError("upstream unavailable", nil)
	if mapped := (defaultCodexFileSearchErrorMapper{}).Map(original); mapped != original {
		t.Fatalf("expected service error passthrough, got %#v", mapped)
	}
}

func TestDefaultCodexFileSearchErrorMapperCoversRemainingBranches(t *testing.T) {
	if err := (defaultCodexFileSearchErrorMapper{}).Map(nil); err != nil {
		t.Fatalf("expected nil passthrough, got %v", err)
	}
	err := (defaultCodexFileSearchErrorMapper{}).Map(&codexRPCError{Code: -32602, Message: "invalid params"})
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("expected invalid params to map to unsupported, got %#v", err)
	}
	err = (defaultCodexFileSearchErrorMapper{}).Map(&codexRPCError{Code: -32000, Message: "experimental feature not enabled"})
	if !errors.As(err, &serviceErr) || serviceErr.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("expected experimental message to map to unsupported, got %#v", err)
	}
	err = (defaultCodexFileSearchErrorMapper{}).Map(errors.New("network down"))
	if !errors.As(err, &serviceErr) || serviceErr.Code != "" || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("expected generic unavailable fallback, got %#v", err)
	}
}

func TestCodexFileSearchProviderStartAndUpdateEmitResults(t *testing.T) {
	client := &stubCodexFileSearchClient{
		searchFunc: func(_ context.Context, query string, roots []string) (*codexFuzzyFileSearchResponse, error) {
			if query != "main" {
				t.Fatalf("unexpected query: %q", query)
			}
			if len(roots) != 1 || roots[0] != "/repo/app" {
				t.Fatalf("unexpected roots: %#v", roots)
			}
			return &codexFuzzyFileSearchResponse{
				Files: []codexFuzzyFileSearchResult{{
					FileName: "main.go",
					Path:     "cmd/main.go",
					Root:     "/repo/app",
					Score:    88,
				}},
			}, nil
		},
	}
	provider := NewCodexFileSearchProvider("codex", stubCodexFileSearchEnvironmentResolver{
		env: codexFileSearchEnvironment{
			Cwd:       "/repo/app",
			CodexHome: "/repo/.archon",
			Roots: []FileSearchRoot{{
				Path:        "/repo/app",
				DisplayBase: "/repo/app",
				Primary:     true,
			}},
		},
	}, nil).(*codexFileSearchProvider)
	provider.clientFactory = stubCodexFileSearchClientFactory{client: client}

	runtimeRaw, err := provider.Start(context.Background(), FileSearchProviderStartRequest{
		SearchID:  "fs-codex",
		Provider:  "codex",
		Scope:     types.FileSearchScope{Provider: "codex", Cwd: "/repo/app"},
		Query:     "main",
		Limit:     10,
		CreatedAt: time.Unix(10, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	runtime := runtimeRaw.(*codexFileSearchRuntime)
	if snapshot := runtime.Snapshot(); snapshot == nil || snapshot.Provider != "codex" || snapshot.Query != "main" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	event := <-runtime.Events()
	if event.Kind != types.FileSearchEventStarted {
		t.Fatalf("expected started event, got %#v", event)
	}
	event = <-runtime.Events()
	if event.Kind != types.FileSearchEventResults || len(event.Candidates) != 1 || event.Candidates[0].DisplayPath != "cmd/main.go" {
		t.Fatalf("unexpected results event: %#v", event)
	}
	updated, err := runtime.Update(context.Background(), types.FileSearchUpdateRequest{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil || updated.Query != "main" {
		t.Fatalf("unexpected updated snapshot: %#v", updated)
	}
	<-runtime.Events()
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if client.closeCount == 0 {
		t.Fatal("expected client to be closed")
	}
}

func TestCodexFileSearchRuntimeCoversEmptyQueryAndFailureBranches(t *testing.T) {
	manager := &stubCodexFileSearchClientManager{
		client: &stubCodexFileSearchClient{
			searchFunc: func(_ context.Context, query string, roots []string) (*codexFuzzyFileSearchResponse, error) {
				return &codexFuzzyFileSearchResponse{}, nil
			},
		},
	}
	runtime := &codexFileSearchRuntime{
		providerName: "codex",
		environment: stubCodexFileSearchEnvironmentResolver{
			env: codexFileSearchEnvironment{
				Cwd: "/repo/app",
				Roots: []FileSearchRoot{{
					Path:        "/repo/app",
					DisplayBase: "/repo/app",
					Primary:     true,
				}},
			},
		},
		clients:      manager,
		resultMapper: stubCodexFileSearchResultMapper{},
		errorMapper:  stubCodexFileSearchErrorMapper{},
		searchID:     "fs-codex",
		events:       make(chan types.FileSearchEvent, 1),
	}
	session, candidates, err := runtime.search(context.Background(), types.FileSearchScope{Provider: "codex", Cwd: "/repo/app"}, "", 0)
	if err != nil {
		t.Fatalf("search empty query: %v", err)
	}
	if len(candidates) != 0 || session.CreatedAt.IsZero() || session.Limit != defaultFileSearchLimit {
		t.Fatalf("unexpected empty-query search result: session=%#v candidates=%#v", session, candidates)
	}
	if len(manager.calls) != 0 {
		t.Fatalf("expected no client acquisition for empty query, got %#v", manager.calls)
	}

	runtime.events <- types.FileSearchEvent{}
	runtime.enqueue(types.FileSearchEvent{Kind: types.FileSearchEventResults})

	runtime.environment = stubCodexFileSearchEnvironmentResolver{err: errors.New("env failed")}
	if _, _, err := runtime.search(context.Background(), types.FileSearchScope{Provider: "codex"}, "main", 5); err == nil {
		t.Fatal("expected environment error")
	}

	runtime.environment = stubCodexFileSearchEnvironmentResolver{
		env: codexFileSearchEnvironment{
			Cwd:   "/repo/app",
			Roots: []FileSearchRoot{{Path: "/repo/app", Primary: true}},
		},
	}
	runtime.clients = &stubCodexFileSearchClientManager{clientErr: errors.New("client failed")}
	runtime.errorMapper = stubCodexFileSearchErrorMapper{
		mapFunc: func(err error) error { return unavailableError("mapped client failure", err) },
	}
	if _, _, err := runtime.search(context.Background(), types.FileSearchScope{Provider: "codex"}, "main", 5); err == nil || !strings.Contains(err.Error(), "mapped client failure") {
		t.Fatalf("expected mapped client failure, got %v", err)
	}
}

func TestCodexFileSearchProviderStartMapsUnsupportedRPC(t *testing.T) {
	provider := NewCodexFileSearchProvider("codex", stubCodexFileSearchEnvironmentResolver{
		env: codexFileSearchEnvironment{
			Cwd: "/repo/app",
			Roots: []FileSearchRoot{{
				Path:        "/repo/app",
				DisplayBase: "/repo/app",
				Primary:     true,
			}},
		},
	}, nil).(*codexFileSearchProvider)
	provider.clientFactory = stubCodexFileSearchClientFactory{
		client: &stubCodexFileSearchClient{
			searchFunc: func(context.Context, string, []string) (*codexFuzzyFileSearchResponse, error) {
				return nil, &codexRPCError{Code: -32601, Message: "method not found"}
			},
		},
	}
	_, err := provider.Start(context.Background(), FileSearchProviderStartRequest{
		SearchID: "fs-codex",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex", Cwd: "/repo/app"},
		Query:    "main",
		Limit:    10,
	})
	if err == nil {
		t.Fatal("expected unsupported error")
	}
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("expected service error, got %v", err)
	}
	if serviceErr.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("expected unsupported code, got %#v", serviceErr)
	}
}
