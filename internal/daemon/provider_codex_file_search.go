package daemon

import (
	"context"
	"strings"
	"sync"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

type codexFileSearchProvider struct {
	providerName  string
	environment   codexFileSearchEnvironmentProvider
	clientFactory codexFileSearchClientFactory
	resultMapper  codexFileSearchResultMapper
	errorMapper   codexFileSearchErrorMapper
	logger        logging.Logger
}

type codexFileSearchRuntime struct {
	providerName string
	environment  codexFileSearchEnvironmentProvider
	clients      codexFileSearchClientManager
	resultMapper codexFileSearchResultMapper
	errorMapper  codexFileSearchErrorMapper
	searchID     string
	createdAt    time.Time

	mu      sync.Mutex
	session *types.FileSearchSession
	events  chan types.FileSearchEvent
	closed  bool
}

func NewCodexFileSearchProvider(provider string, environment codexFileSearchEnvironmentProvider, logger logging.Logger) FileSearchProvider {
	if logger == nil {
		logger = logging.Nop()
	}
	return &codexFileSearchProvider{
		providerName:  strings.TrimSpace(provider),
		environment:   codexFileSearchEnvironmentProviderOrDefault(environment),
		clientFactory: defaultCodexFileSearchClientFactory{},
		resultMapper:  defaultCodexFileSearchResultMapper{},
		errorMapper:   defaultCodexFileSearchErrorMapper{},
		logger:        logger,
	}
}

func (p *codexFileSearchProvider) Start(ctx context.Context, req FileSearchProviderStartRequest) (FileSearchRuntime, error) {
	if p == nil {
		return nil, unavailableError("file search provider is not initialized", nil)
	}
	runtime := &codexFileSearchRuntime{
		providerName: strings.TrimSpace(req.Provider),
		environment:  codexFileSearchEnvironmentProviderOrDefault(p.environment),
		clients:      newReusableCodexFileSearchClientManager(p.clientFactory, p.logger),
		resultMapper: codexFileSearchResultMapperOrDefault(p.resultMapper),
		errorMapper:  codexFileSearchErrorMapperOrDefault(p.errorMapper),
		searchID:     strings.TrimSpace(req.SearchID),
		createdAt:    req.CreatedAt,
		events:       make(chan types.FileSearchEvent, 8),
	}
	session, candidates, err := runtime.search(ctx, req.Scope, req.Query, req.Limit)
	if err != nil {
		close(runtime.events)
		_ = runtime.clients.Close()
		return nil, err
	}
	runtime.session = session
	occurredAt := time.Now().UTC()
	runtime.enqueue(buildFileSearchEvent(types.FileSearchEventStarted, session, nil, "", occurredAt))
	runtime.enqueue(buildFileSearchEvent(types.FileSearchEventResults, session, candidates, "", occurredAt))
	return runtime, nil
}

func (r *codexFileSearchRuntime) Snapshot() *types.FileSearchSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneFileSearchSession(r.session)
}

func (r *codexFileSearchRuntime) Update(ctx context.Context, req types.FileSearchUpdateRequest) (*types.FileSearchSession, error) {
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

	session, candidates, err := r.search(ctx, scope, query, limit)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.session = session
	r.mu.Unlock()
	r.enqueue(buildFileSearchEvent(types.FileSearchEventResults, session, candidates, "", time.Now().UTC()))
	return cloneFileSearchSession(session), nil
}

func (r *codexFileSearchRuntime) Close(context.Context) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	events := r.events
	r.events = nil
	r.mu.Unlock()
	if events != nil {
		close(events)
	}
	return r.clients.Close()
}

func (r *codexFileSearchRuntime) Events() <-chan types.FileSearchEvent {
	return r.events
}

func (r *codexFileSearchRuntime) search(ctx context.Context, scope types.FileSearchScope, query string, limit int) (*types.FileSearchSession, []types.FileSearchCandidate, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = defaultFileSearchLimit
	}
	env, err := r.environment.Environment(ctx, scope)
	if err != nil {
		return nil, nil, err
	}

	var candidates []types.FileSearchCandidate
	if query != "" {
		client, err := r.clients.Client(ctx, env)
		if err != nil {
			return nil, nil, r.errorMapper.Map(err)
		}
		response, err := client.FuzzyFileSearch(ctx, query, fileSearchRootPaths(env.Roots))
		if err != nil {
			return nil, nil, r.errorMapper.Map(err)
		}
		candidates = r.resultMapper.Map(env.Roots, response, limit)
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

func (r *codexFileSearchRuntime) enqueue(event types.FileSearchEvent) {
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

func fileSearchRootPaths(roots []FileSearchRoot) []string {
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		root.Path = strings.TrimSpace(root.Path)
		if root.Path == "" {
			continue
		}
		out = append(out, root.Path)
	}
	return out
}
