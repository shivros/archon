package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"control/internal/apicode"
	"control/internal/types"
)

type openCodeFileSearchProvider struct {
	providerName    string
	rootResolver    FileSearchRootResolver
	searcherFactory openCodeFileSearcherFactory
	normalizer      FileSearchCandidateNormalizer
}

type openCodeFileSearchRuntime struct {
	providerName    string
	rootResolver    FileSearchRootResolver
	searcherFactory openCodeFileSearcherFactory
	normalizer      FileSearchCandidateNormalizer
	searchID        string
	createdAt       time.Time

	mu      sync.Mutex
	session *types.FileSearchSession
	events  chan types.FileSearchEvent
	closed  bool
}

func NewOpenCodeFileSearchProvider(provider string, rootResolver FileSearchRootResolver) FileSearchProvider {
	return &openCodeFileSearchProvider{
		providerName:    strings.TrimSpace(provider),
		rootResolver:    fileSearchRootResolverOrDefault(rootResolver),
		searcherFactory: defaultOpenCodeFileSearcherFactory{},
		normalizer:      defaultFileSearchCandidateNormalizer{},
	}
}

func (p *openCodeFileSearchProvider) Start(ctx context.Context, req FileSearchProviderStartRequest) (FileSearchRuntime, error) {
	if p == nil {
		return nil, unavailableError("file search provider is not initialized", nil)
	}
	runtime := &openCodeFileSearchRuntime{
		providerName:    strings.TrimSpace(req.Provider),
		rootResolver:    fileSearchRootResolverOrDefault(p.rootResolver),
		searcherFactory: openCodeFileSearcherFactoryOrDefault(p.searcherFactory),
		normalizer:      fileSearchCandidateNormalizerOrDefault(p.normalizer),
		searchID:        strings.TrimSpace(req.SearchID),
		createdAt:       req.CreatedAt,
		events:          make(chan types.FileSearchEvent, 8),
	}
	session, results, err := runtime.search(ctx, req.Scope, req.Query, req.Limit)
	if err != nil {
		close(runtime.events)
		return nil, err
	}
	runtime.session = session
	occurredAt := time.Now().UTC()
	runtime.enqueue(buildFileSearchEvent(types.FileSearchEventStarted, session, nil, "", occurredAt))
	runtime.enqueue(buildFileSearchEvent(types.FileSearchEventResults, session, results, "", occurredAt))
	return runtime, nil
}

func (r *openCodeFileSearchRuntime) Snapshot() *types.FileSearchSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneFileSearchSession(r.session)
}

func (r *openCodeFileSearchRuntime) Update(ctx context.Context, req types.FileSearchUpdateRequest) (*types.FileSearchSession, error) {
	r.mu.Lock()
	current := cloneFileSearchSession(r.session)
	r.mu.Unlock()
	if current == nil {
		return nil, unavailableError("file search state is unavailable", nil)
	}

	scope := current.Scope
	if req.Scope != nil {
		scope = copyFileSearchScope(*req.Scope)
	}
	query := current.Query
	if req.Query != nil {
		query = strings.TrimSpace(*req.Query)
	}
	limit := current.Limit
	if req.Limit != nil {
		limit = *req.Limit
	}

	session, results, err := r.search(ctx, scope, query, limit)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.session = session
	r.mu.Unlock()
	r.enqueue(buildFileSearchEvent(types.FileSearchEventResults, session, results, "", time.Now().UTC()))
	return cloneFileSearchSession(session), nil
}

func (r *openCodeFileSearchRuntime) Close(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	close(r.events)
	return nil
}

func (r *openCodeFileSearchRuntime) Events() <-chan types.FileSearchEvent {
	return r.events
}

func (r *openCodeFileSearchRuntime) search(ctx context.Context, scope types.FileSearchScope, query string, limit int) (*types.FileSearchSession, []types.FileSearchCandidate, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = defaultFileSearchLimit
	}
	roots, err := r.rootResolver.ResolveRoots(ctx, scope)
	if err != nil {
		return nil, nil, err
	}
	searcher, err := r.searcherFactory.Searcher(r.providerName)
	if err != nil {
		return nil, nil, unavailableError("file search client is not available", err)
	}
	executor := newOpenCodeFileSearchExecutor(searcher, r.normalizer)
	candidates, err := executor.Search(ctx, query, roots, limit)
	if err != nil {
		return nil, nil, mapOpenCodeFileSearchError(err)
	}
	now := time.Now().UTC()
	session := &types.FileSearchSession{
		ID:        strings.TrimSpace(r.searchID),
		Provider:  strings.TrimSpace(r.providerName),
		Scope:     copyFileSearchScope(scope),
		Query:     query,
		Limit:     limit,
		Status:    types.FileSearchStatusActive,
		CreatedAt: r.createdAt,
		UpdatedAt: &now,
	}
	if r.createdAt.IsZero() {
		session.CreatedAt = now
	}
	return session, candidates, nil
}

func (r *openCodeFileSearchRuntime) enqueue(event types.FileSearchEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.events == nil {
		return
	}
	select {
	case r.events <- event:
	default:
	}
}

func mapOpenCodeFileSearchError(err error) error {
	if err == nil {
		return nil
	}
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		return err
	}
	var requestErr *openCodeRequestError
	if errors.As(err, &requestErr) && requestErr != nil {
		switch requestErr.StatusCode {
		case 404, 405:
			return unavailableErrorWithCode("file search endpoint is not available for this provider runtime", apicode.ErrorCodeFileSearchUnsupported, err)
		}
	}
	return unavailableError("file search failed", err)
}
