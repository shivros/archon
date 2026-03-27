package daemon

import (
	"context"
	"errors"
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
	searchFunc func(ctx context.Context, query, directory string) ([]string, error)
}

func (s stubOpenCodeFileSearcher) SearchFiles(ctx context.Context, query, directory string) ([]string, error) {
	if s.searchFunc == nil {
		return nil, nil
	}
	return s.searchFunc(ctx, query, directory)
}

func TestOpenCodeFileSearchExecutorSearchesRootsAndNormalizes(t *testing.T) {
	executor := newOpenCodeFileSearchExecutor(stubOpenCodeFileSearcher{
		searchFunc: func(_ context.Context, query, directory string) ([]string, error) {
			if query != "foo" {
				t.Fatalf("unexpected query: %q", query)
			}
			switch directory {
			case "/repo/app":
				return []string{"foo.go"}, nil
			case "/repo/backend":
				return []string{"api/foo_service.go"}, nil
			default:
				t.Fatalf("unexpected directory: %q", directory)
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

func TestOpenCodeFileSearchRuntimeUpdateEmitsResults(t *testing.T) {
	runtime := &openCodeFileSearchRuntime{
		providerName: "opencode",
		rootResolver: stubFileSearchRootResolver{roots: []FileSearchRoot{
			{Path: "/repo/app", DisplayBase: "/repo/app", Primary: true},
		}},
		searcherFactory: stubOpenCodeFileSearchClientFactory{
			searcher: stubOpenCodeFileSearcher{
				searchFunc: func(_ context.Context, query, directory string) ([]string, error) {
					if query != "main" || directory != "/repo/app" {
						t.Fatalf("unexpected search request query=%q directory=%q", query, directory)
					}
					return []string{"src/main.go"}, nil
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

	updated, err := runtime.Update(context.Background(), types.FileSearchUpdateRequest{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Query != "main" {
		t.Fatalf("expected query to be preserved, got %#v", updated)
	}
	select {
	case event := <-runtime.events:
		if event.Kind != types.FileSearchEventResults || len(event.Candidates) != 1 {
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
			searchFunc: func(_ context.Context, query, directory string) ([]string, error) {
				if query != "main" || directory != "/repo/app" {
					t.Fatalf("unexpected search request query=%q directory=%q", query, directory)
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
				searchFunc: func(context.Context, string, string) ([]string, error) {
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
				searchFunc: func(context.Context, string, string) ([]string, error) {
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
				searchFunc: func(context.Context, string, string) ([]string, error) {
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

func TestRecoveringOpenCodeFileSearcherNilClient(t *testing.T) {
	searcher := &recoveringOpenCodeFileSearcher{}
	_, err := searcher.SearchFiles(context.Background(), "foo", "/repo")
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
	_, err := searcher.SearchFiles(context.Background(), "foo", "/repo")
	var reqErr *openCodeRequestError
	if !errors.As(err, &reqErr) || reqErr.StatusCode != 400 {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestMapOpenCodeFileSearchErrorPassesThroughServiceErrors(t *testing.T) {
	original := unavailableError("upstream unavailable", nil)
	if got := mapOpenCodeFileSearchError(original); got != original {
		t.Fatalf("expected service error passthrough, got %#v", got)
	}
}
