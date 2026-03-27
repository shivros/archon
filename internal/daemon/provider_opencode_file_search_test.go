package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/apicode"
	"control/internal/types"
)

type stubOpenCodeJSONRequester struct {
	doJSONFunc func(ctx context.Context, method, path string, body any, out any) error
}

func (r stubOpenCodeJSONRequester) doJSON(ctx context.Context, method, path string, body any, out any) error {
	if r.doJSONFunc == nil {
		return nil
	}
	return r.doJSONFunc(ctx, method, path, body, out)
}

type stubFileSearchRootResolver struct {
	roots []FileSearchRoot
	err   error
}

func (r stubFileSearchRootResolver) ResolveRoots(context.Context, types.FileSearchScope) ([]FileSearchRoot, error) {
	if r.err != nil {
		return nil, r.err
	}
	return append([]FileSearchRoot(nil), r.roots...), nil
}

type stubOpenCodeFileSearchClientFactory struct {
	searcher openCodeFileSearcher
	err      error
}

func (f stubOpenCodeFileSearchClientFactory) Searcher(string) (openCodeFileSearcher, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.searcher, nil
}

func TestFileSearchCandidateNormalizerMergesDedupesAndNormalizesDisplayPaths(t *testing.T) {
	normalizer := defaultFileSearchCandidateNormalizer{}
	candidates := normalizer.Normalize("foo", []FileSearchRoot{
		{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
		{Path: "/repo/backend", DisplayBase: "/repo/app"},
	}, map[string][]string{
		"/repo/app":     {"foo.go", "internal/foo_helper.go"},
		"/repo/backend": {"/repo/app/foo.go", "api/foo_service.go"},
	}, 10)
	if len(candidates) != 3 {
		t.Fatalf("expected 3 merged candidates, got %#v", candidates)
	}
	if candidates[0].Path != "/repo/app/foo.go" || candidates[0].DisplayPath != "foo.go" {
		t.Fatalf("unexpected primary candidate: %#v", candidates[0])
	}
	if candidates[1].DisplayPath != "internal/foo_helper.go" {
		t.Fatalf("expected app-relative helper path, got %#v", candidates[1])
	}
	if candidates[2].DisplayPath != "../backend/api/foo_service.go" {
		t.Fatalf("expected backend path relative to primary root, got %#v", candidates[2])
	}
}

func TestFileSearchCandidateNormalizerPrefersPrimaryOnScoreTie(t *testing.T) {
	normalizer := defaultFileSearchCandidateNormalizer{}
	candidates := normalizer.Normalize("foo", []FileSearchRoot{
		{Path: "/repo/secondary", DisplayBase: "/repo/primary"},
		{Path: "/repo/primary", DisplayBase: "/repo/primary", Primary: true},
	}, map[string][]string{
		"/repo/secondary": {"foo.go"},
		"/repo/primary":   {"foo.go"},
	}, 10)
	if len(candidates) != 2 {
		t.Fatalf("expected two candidates, got %#v", candidates)
	}
	if candidates[0].Path != "/repo/primary/foo.go" {
		t.Fatalf("expected primary root candidate first, got %#v", candidates)
	}
}

func TestFileSearchCandidateNormalizerIgnoresUnknownRootsAndFallsBackWithoutDisplayBase(t *testing.T) {
	normalizer := defaultFileSearchCandidateNormalizer{}
	candidates := normalizer.Normalize("foo", []FileSearchRoot{
		{Path: "/repo/app", Primary: true},
	}, map[string][]string{
		"/repo/app":     {"nested/foo.txt"},
		"/repo/unknown": {"foo.go"},
	}, 10)
	if len(candidates) != 1 {
		t.Fatalf("expected only known-root candidate, got %#v", candidates)
	}
	if candidates[0].DisplayPath != "/repo/app/nested/foo.txt" {
		t.Fatalf("expected absolute display path fallback without display base, got %#v", candidates[0])
	}
}

func TestScoreFileSearchCandidateUsesAbsolutePathFallback(t *testing.T) {
	score := scoreFileSearchCandidate("backend", "src/main.go", "/repo/backend/src/main.go")
	noMatch := scoreFileSearchCandidate("backend", "src/main.go", "/repo/other/src/main.go")
	if score <= noMatch {
		t.Fatalf("expected absolute path fallback score above no-match baseline, got score=%f baseline=%f", score, noMatch)
	}
}

func TestPreferFileSearchCandidateUsesDisplayPathTieBreak(t *testing.T) {
	left := scoredFileSearchCandidate{
		candidate: types.FileSearchCandidate{Path: "/repo/b.go", DisplayPath: "b.go", Score: 50},
	}
	right := scoredFileSearchCandidate{
		candidate: types.FileSearchCandidate{Path: "/repo/a.go", DisplayPath: "a.go", Score: 50},
	}
	if preferFileSearchCandidate(left, right) {
		t.Fatalf("expected lexical display path tie-break to prefer right candidate")
	}
}

type stubOpenCodeFileSearcher struct {
	searchFunc func(ctx context.Context, req openCodeFileSearchRequest) ([]string, error)
}

func (s stubOpenCodeFileSearcher) SearchFiles(ctx context.Context, req openCodeFileSearchRequest) ([]string, error) {
	if s.searchFunc == nil {
		return nil, nil
	}
	return s.searchFunc(ctx, req)
}

func TestOpenCodeFileSearchExecutorSearchesRootsAndNormalizes(t *testing.T) {
	executor := newOpenCodeFileSearchExecutor(stubOpenCodeFileSearcher{
		searchFunc: func(_ context.Context, req openCodeFileSearchRequest) ([]string, error) {
			if req.Query != "foo" {
				t.Fatalf("unexpected query: %q", req.Query)
			}
			if req.Limit != 10 {
				t.Fatalf("unexpected limit: %d", req.Limit)
			}
			switch req.Directory {
			case "/repo/app":
				return []string{"foo.go"}, nil
			case "/repo/backend":
				return []string{"api/foo_service.go"}, nil
			default:
				t.Fatalf("unexpected directory: %q", req.Directory)
				return nil, nil
			}
		},
	}, defaultFileSearchCandidateNormalizer{})
	candidates, err := executor.Search(context.Background(), "foo", []FileSearchRoot{
		{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
		{Path: "/repo/backend", DisplayBase: "/repo/app"},
	}, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(candidates) != 2 || candidates[0].DisplayPath != "foo.go" || candidates[1].DisplayPath != "../backend/api/foo_service.go" {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
}

func TestOpenCodeFileSearchExecutorFinalResultsStillRespectLimit(t *testing.T) {
	executor := newOpenCodeFileSearchExecutor(stubOpenCodeFileSearcher{
		searchFunc: func(_ context.Context, req openCodeFileSearchRequest) ([]string, error) {
			if req.Limit != 2 {
				t.Fatalf("unexpected limit: %d", req.Limit)
			}
			return []string{"main.go", "main_test.go", "cmd/main.go"}, nil
		},
	}, defaultFileSearchCandidateNormalizer{})
	candidates, err := executor.Search(context.Background(), "main", []FileSearchRoot{
		{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
	}, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates after normalization limit, got %#v", candidates)
	}
}

func TestOpenCodeFileSearchRuntimeUpdateEmitsResults(t *testing.T) {
	nextLimit := 3
	runtime := &openCodeFileSearchRuntime{
		providerName: "opencode",
		rootResolver: stubFileSearchRootResolver{roots: []FileSearchRoot{
			{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
		}},
		searcherFactory: stubOpenCodeFileSearchClientFactory{
			searcher: stubOpenCodeFileSearcher{
				searchFunc: func(_ context.Context, req openCodeFileSearchRequest) ([]string, error) {
					if req.Query != "main" || req.Directory != "/repo/app" || req.Limit != nextLimit {
						t.Fatalf("unexpected search request %#v", req)
					}
					return []string{"src/main.go", "src/main_test.go", "cmd/main.go"}, nil
				},
			},
		},
		normalizer: defaultFileSearchCandidateNormalizer{},
		searchID:   "fs-1",
		createdAt:  time.Unix(1, 0).UTC(),
		events:     make(chan types.FileSearchEvent, 4),
		session: &types.FileSearchSession{
			ID:        "fs-1",
			Provider:  "opencode",
			Scope:     types.FileSearchScope{Provider: "opencode", Cwd: "/repo/app"},
			Query:     "main",
			Limit:     10,
			Status:    types.FileSearchStatusActive,
			CreatedAt: time.Unix(1, 0).UTC(),
		},
	}

	updated, err := runtime.Update(context.Background(), types.FileSearchUpdateRequest{Limit: &nextLimit})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Query != "main" || updated.Limit != nextLimit {
		t.Fatalf("expected query to be preserved, got %#v", updated)
	}
	select {
	case event := <-runtime.events:
		if event.Kind != types.FileSearchEventResults || len(event.Candidates) != nextLimit {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for results event")
	}
}

func TestOpenCodeFileSearchProviderStartAndRuntimeLifecycle(t *testing.T) {
	provider := NewOpenCodeFileSearchProvider("opencode", stubFileSearchRootResolver{roots: []FileSearchRoot{
		{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
	}}).(*openCodeFileSearchProvider)
	provider.searcherFactory = stubOpenCodeFileSearchClientFactory{
		searcher: stubOpenCodeFileSearcher{
			searchFunc: func(_ context.Context, req openCodeFileSearchRequest) ([]string, error) {
				if req.Query != "main" || req.Directory != "/repo/app" || req.Limit != 10 {
					t.Fatalf("unexpected search request %#v", req)
				}
				return []string{"main.go"}, nil
			},
		},
	}
	provider.normalizer = defaultFileSearchCandidateNormalizer{}

	runtimeRaw, err := provider.Start(context.Background(), FileSearchProviderStartRequest{
		SearchID:  "fs-1",
		Provider:  "opencode",
		Scope:     types.FileSearchScope{Provider: "opencode", Cwd: "/repo/app"},
		Query:     "main",
		Limit:     10,
		CreatedAt: time.Unix(5, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	runtime := runtimeRaw.(*openCodeFileSearchRuntime)
	snapshot := runtime.Snapshot()
	if snapshot == nil || snapshot.ID != "fs-1" || snapshot.Query != "main" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if runtime.Events() == nil {
		t.Fatal("expected events channel")
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("second Close should be a no-op: %v", err)
	}
}

func TestOpenCodeFileSearchProviderStartReturnsUnavailableOnSearchFailure(t *testing.T) {
	provider := &openCodeFileSearchProvider{
		providerName: "opencode",
		rootResolver: stubFileSearchRootResolver{roots: []FileSearchRoot{
			{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
		}},
		searcherFactory: stubOpenCodeFileSearchClientFactory{
			searcher: stubOpenCodeFileSearcher{
				searchFunc: func(context.Context, openCodeFileSearchRequest) ([]string, error) {
					return nil, &openCodeRequestError{Method: "GET", Path: "/find/file", StatusCode: 500, Message: "boom"}
				},
			},
		},
		normalizer: defaultFileSearchCandidateNormalizer{},
	}

	_, err := provider.Start(context.Background(), FileSearchProviderStartRequest{
		SearchID:  "fs-1",
		Provider:  "opencode",
		Scope:     types.FileSearchScope{Provider: "opencode", Cwd: "/repo/app"},
		Query:     "foo",
		Limit:     5,
		CreatedAt: time.Now().UTC(),
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestOpenCodeFileSearchProviderStartMapsUnsupportedEndpoint(t *testing.T) {
	provider := &openCodeFileSearchProvider{
		providerName: "opencode",
		rootResolver: stubFileSearchRootResolver{roots: []FileSearchRoot{
			{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
		}},
		searcherFactory: stubOpenCodeFileSearchClientFactory{
			searcher: stubOpenCodeFileSearcher{
				searchFunc: func(context.Context, openCodeFileSearchRequest) ([]string, error) {
					return nil, &openCodeRequestError{Method: "GET", Path: "/find/file", StatusCode: 404, Message: "missing"}
				},
			},
		},
		normalizer: defaultFileSearchCandidateNormalizer{},
	}
	_, err := provider.Start(context.Background(), FileSearchProviderStartRequest{
		SearchID: "fs-1",
		Provider: "opencode",
		Scope:    types.FileSearchScope{Provider: "opencode", Cwd: "/repo/app"},
		Query:    "foo",
		Limit:    5,
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable || serviceErr.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestOpenCodeFileSearchRuntimeAllowsEmptyQueryWithoutCallingProvider(t *testing.T) {
	runtime := &openCodeFileSearchRuntime{
		providerName: "opencode",
		rootResolver: stubFileSearchRootResolver{roots: []FileSearchRoot{
			{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
		}},
		searcherFactory: stubOpenCodeFileSearchClientFactory{
			searcher: stubOpenCodeFileSearcher{
				searchFunc: func(context.Context, openCodeFileSearchRequest) ([]string, error) {
					return nil, errors.New("provider should not be called for empty query")
				},
			},
		},
		normalizer: defaultFileSearchCandidateNormalizer{},
		searchID:   "fs-1",
		createdAt:  time.Now().UTC(),
		events:     make(chan types.FileSearchEvent, 4),
	}

	session, results, err := runtime.search(context.Background(), types.FileSearchScope{Provider: "opencode", Cwd: "/repo/app"}, "", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if session == nil || len(results) != 0 {
		t.Fatalf("expected empty results for empty query, got session=%#v results=%#v", session, results)
	}
}

func TestOpenCodeFileSearchRuntimeDefaultsLimitBeforeCallingSearcher(t *testing.T) {
	runtime := &openCodeFileSearchRuntime{
		providerName: "opencode",
		rootResolver: stubFileSearchRootResolver{roots: []FileSearchRoot{
			{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
		}},
		searcherFactory: stubOpenCodeFileSearchClientFactory{
			searcher: stubOpenCodeFileSearcher{
				searchFunc: func(_ context.Context, req openCodeFileSearchRequest) ([]string, error) {
					if req.Limit != defaultFileSearchLimit {
						t.Fatalf("expected default limit %d, got %d", defaultFileSearchLimit, req.Limit)
					}
					return []string{"src/main.go"}, nil
				},
			},
		},
		normalizer: defaultFileSearchCandidateNormalizer{},
		searchID:   "fs-1",
		createdAt:  time.Now().UTC(),
		events:     make(chan types.FileSearchEvent, 4),
	}

	session, results, err := runtime.search(context.Background(), types.FileSearchScope{Provider: "opencode", Cwd: "/repo/app"}, "main", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if session == nil || session.Limit != defaultFileSearchLimit {
		t.Fatalf("expected session limit %d, got %#v", defaultFileSearchLimit, session)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %#v", results)
	}
}

func TestRecoveringOpenCodeFileSearcherNilClient(t *testing.T) {
	searcher := &recoveringOpenCodeFileSearcher{}
	_, err := searcher.SearchFiles(context.Background(), openCodeFileSearchRequest{
		Query:     "foo",
		Directory: "/repo",
		Limit:     5,
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestRecoveringOpenCodeFileSearcherReturnsNonRetryableError(t *testing.T) {
	searcher := &recoveringOpenCodeFileSearcher{
		provider: "opencode",
		client: &openCodeClient{
			fileSearchSvc: &openCodeFileSearchService{requester: stubOpenCodeJSONRequester{
				doJSONFunc: func(context.Context, string, string, any, any) error {
					return &openCodeRequestError{Method: "GET", Path: "/find/file", StatusCode: 400, Message: "bad"}
				},
			}},
		},
	}
	_, err := searcher.SearchFiles(context.Background(), openCodeFileSearchRequest{
		Query:     "foo",
		Directory: "/repo",
		Limit:     5,
	})
	var reqErr *openCodeRequestError
	if !errors.As(err, &reqErr) || reqErr.StatusCode != 400 {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestRecoveringOpenCodeFileSearcherRetryPreservesRequest(t *testing.T) {
	t.Cleanup(resetOpenCodeAutoStartStateForTest)
	origStart := startOpenCodeServeProcess
	origProbe := probeOpenCodeServer
	origWait := waitForOpenCodeServerReady
	origFallback := pickOpenCodeFallbackPortFn
	t.Cleanup(func() {
		startOpenCodeServeProcess = origStart
		probeOpenCodeServer = origProbe
		waitForOpenCodeServerReady = origWait
		pickOpenCodeFallbackPortFn = origFallback
	})

	var seenRawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/find/file" {
			http.NotFound(w, r)
			return
		}
		seenRawQuery = r.URL.RawQuery
		writeJSON(w, http.StatusOK, []string{"src/main.go"})
	}))
	defer server.Close()

	portIdx := strings.LastIndex(server.URL, ":")
	if portIdx < 0 || portIdx+1 >= len(server.URL) {
		t.Fatalf("unexpected server url: %q", server.URL)
	}
	fallbackPort := server.URL[portIdx+1:]

	tmpDir := t.TempDir()
	cmdPath := filepath.Join(tmpDir, "opencode")
	if err := os.WriteFile(cmdPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake opencode: %v", err)
	}
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	startOpenCodeServeProcess = func(_ string, args []string, _ []string, _ ProviderBaseSink) error {
		if len(args) >= 5 && args[4] == fallbackPort {
			return nil
		}
		return errors.New("primary unavailable")
	}
	probeOpenCodeServer = func(string) error { return errors.New("unreachable") }
	waitForOpenCodeServerReady = func(string, time.Duration) error { return nil }
	pickOpenCodeFallbackPortFn = func(string) (string, error) { return fallbackPort, nil }
	resetOpenCodeAutoStartStateForTest()

	searcher := &recoveringOpenCodeFileSearcher{
		provider: "opencode",
		client: &openCodeClient{
			baseURL: "http://127.0.0.1:49123",
			token:   "token-123",
			timeout: time.Second,
			fileSearchSvc: &openCodeFileSearchService{requester: stubOpenCodeJSONRequester{
				doJSONFunc: func(context.Context, string, string, any, any) error {
					return &openCodeRequestError{Method: "GET", Path: "/find/file", StatusCode: 503, Message: "unreachable"}
				},
			}},
		},
	}

	results, err := searcher.SearchFiles(context.Background(), openCodeFileSearchRequest{
		Query:     "  main  ",
		Directory: " /tmp/opencode-worktree ",
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("SearchFiles retry: %v", err)
	}
	if len(results) != 1 || results[0] != "src/main.go" {
		t.Fatalf("unexpected results: %#v", results)
	}
	if got, want := strings.TrimSpace(seenRawQuery), "query=main&limit=5&directory=%2Ftmp%2Fopencode-worktree"; got != want {
		t.Fatalf("unexpected retried raw query: got %q want %q", got, want)
	}
}

func TestMapOpenCodeFileSearchErrorPassesThroughServiceErrors(t *testing.T) {
	original := unavailableError("upstream unavailable", nil)
	if got := mapOpenCodeFileSearchError(original); got != original {
		t.Fatalf("expected service error passthrough, got %#v", got)
	}
}
