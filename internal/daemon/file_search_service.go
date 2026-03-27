package daemon

import (
	"context"
	"strings"
	"time"

	"control/internal/apicode"
	"control/internal/logging"
	"control/internal/providers"
	"control/internal/types"
)

const defaultFileSearchLimit = 20

type FileSearchProviderStartRequest struct {
	SearchID  string
	Provider  string
	Scope     types.FileSearchScope
	Query     string
	Limit     int
	CreatedAt time.Time
}

type FileSearchRuntime interface {
	Snapshot() *types.FileSearchSession
	Update(ctx context.Context, req types.FileSearchUpdateRequest) (*types.FileSearchSession, error)
	Close(ctx context.Context) error
}

type FileSearchEventSource interface {
	Events() <-chan types.FileSearchEvent
}

type FileSearchProvider interface {
	Start(ctx context.Context, req FileSearchProviderStartRequest) (FileSearchRuntime, error)
}

type FileSearchRuntimeRegistry interface {
	Lookup(provider string) (FileSearchProvider, bool)
}

type FileSearchCapabilityResolver interface {
	Capabilities(provider string) providers.Capabilities
}

type defaultFileSearchCapabilityResolver struct{}

func (defaultFileSearchCapabilityResolver) Capabilities(provider string) providers.Capabilities {
	return providers.CapabilitiesFor(provider)
}

type staticFileSearchRuntimeRegistry struct {
	providers map[string]FileSearchProvider
}

func NewFileSearchProviderRegistry(entries map[string]FileSearchProvider) FileSearchRuntimeRegistry {
	cloned := make(map[string]FileSearchProvider, len(entries))
	for provider, impl := range entries {
		name := providers.Normalize(provider)
		if name == "" || impl == nil {
			continue
		}
		cloned[name] = impl
	}
	return staticFileSearchRuntimeRegistry{providers: cloned}
}

func (r staticFileSearchRuntimeRegistry) Lookup(provider string) (FileSearchProvider, bool) {
	provider = providers.Normalize(provider)
	if provider == "" {
		return nil, false
	}
	impl, ok := r.providers[provider]
	if !ok || impl == nil {
		return nil, false
	}
	return impl, true
}

type FileSearchServiceOption func(*fileSearchService)

func WithFileSearchRuntimeRegistry(registry FileSearchRuntimeRegistry) FileSearchServiceOption {
	return func(s *fileSearchService) {
		if s == nil || registry == nil {
			return
		}
		s.runtimeRegistry = registry
	}
}

// WithFileSearchProviderRegistry is kept for compatibility with the initial Phase 1 wiring.
func WithFileSearchProviderRegistry(registry FileSearchRuntimeRegistry) FileSearchServiceOption {
	return WithFileSearchRuntimeRegistry(registry)
}

func WithFileSearchCapabilityResolver(resolver FileSearchCapabilityResolver) FileSearchServiceOption {
	return func(s *fileSearchService) {
		if s == nil || resolver == nil {
			return
		}
		s.capabilities = resolver
	}
}

func WithFileSearchIDGenerator(generator FileSearchIDGenerator) FileSearchServiceOption {
	return func(s *fileSearchService) {
		if s == nil || generator == nil {
			return
		}
		s.idGenerator = generator
	}
}

func WithFileSearchHub(hub FileSearchHub) FileSearchServiceOption {
	return func(s *fileSearchService) {
		if s == nil || hub == nil {
			return
		}
		s.hub = hub
	}
}

type fileSearchService struct {
	scopeResolver   FileSearchScopeResolver
	logger          logging.Logger
	capabilities    FileSearchCapabilityResolver
	runtimeRegistry FileSearchRuntimeRegistry
	idGenerator     FileSearchIDGenerator
	hub             FileSearchHub
}

func NewFileSearchService(scopeResolver FileSearchScopeResolver, logger logging.Logger, opts ...FileSearchServiceOption) FileSearchService {
	if logger == nil {
		logger = logging.Nop()
	}
	service := &fileSearchService{
		scopeResolver:   scopeResolverOrDefault(scopeResolver),
		logger:          logger,
		capabilities:    defaultFileSearchCapabilityResolver{},
		runtimeRegistry: NewFileSearchProviderRegistry(nil),
		idGenerator:     NewRandomFileSearchIDGenerator(),
		hub:             NewMemoryFileSearchHub(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service
}

func (s *fileSearchService) Start(ctx context.Context, req types.FileSearchStartRequest) (*types.FileSearchSession, error) {
	scope, err := s.scopeResolver.ResolveScope(ctx, req.Scope)
	if err != nil {
		return nil, err
	}
	provider := providers.Normalize(scope.Provider)
	if provider == "" {
		return nil, invalidError("scope.provider is required", nil)
	}
	if !s.capabilities.Capabilities(provider).SupportsFileSearch {
		return nil, invalidErrorWithCode("file search is not supported for provider", apicode.ErrorCodeFileSearchUnsupported, nil)
	}
	providerRuntime, ok := s.runtimeRegistry.Lookup(provider)
	if !ok {
		return nil, unavailableErrorWithCode("file search provider is not configured", apicode.ErrorCodeFileSearchUnsupported, nil)
	}
	searchID, err := s.idGenerator.NewID()
	if err != nil {
		return nil, unavailableError("failed to create file search", err)
	}
	query := strings.TrimSpace(req.Query)
	limit := req.Limit
	if limit <= 0 {
		limit = defaultFileSearchLimit
	}
	createdAt := time.Now().UTC()
	runtime, err := providerRuntime.Start(ctx, FileSearchProviderStartRequest{
		SearchID:  searchID,
		Provider:  provider,
		Scope:     scope,
		Query:     query,
		Limit:     limit,
		CreatedAt: createdAt,
	})
	if err != nil {
		return nil, err
	}
	if runtime == nil {
		return nil, unavailableError("file search provider returned no runtime", nil)
	}
	session := newFileSearchSession(runtime.Snapshot(), searchID, provider, scope, query, limit, createdAt)
	if err := s.hub.Register(searchID, session, runtime); err != nil {
		return nil, unavailableError("failed to register file search", err)
	}
	if source, ok := runtime.(FileSearchEventSource); ok && source != nil {
		go s.consumeRuntimeEvents(searchID, source)
	}
	return cloneFileSearchSession(session), nil
}

func (s *fileSearchService) Update(ctx context.Context, id string, req types.FileSearchUpdateRequest) (*types.FileSearchSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, invalidError("file search id is required", nil)
	}
	current, runtime, err := s.lookup(id)
	if err != nil {
		return nil, err
	}
	normalizedReq, err := s.normalizeUpdateRequest(ctx, current, req)
	if err != nil {
		return nil, err
	}
	snapshot, err := runtime.Update(ctx, normalizedReq)
	if err != nil {
		return nil, err
	}
	next, event := applyFileSearchCommandUpdate(current, normalizedReq, snapshot, time.Now().UTC())
	if err := s.hub.Publish(id, next, event, false); err != nil {
		return nil, mapFileSearchHubError(err)
	}
	return cloneFileSearchSession(next), nil
}

func (s *fileSearchService) Close(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return invalidError("file search id is required", nil)
	}
	current, runtime, err := s.lookup(id)
	if err != nil {
		return err
	}
	closeErr := runtime.Close(ctx)
	next, event := applyFileSearchClose(current, time.Now().UTC())
	if err := s.hub.Publish(id, next, event, true); err != nil && !isFileSearchHubNotFound(err) {
		return mapFileSearchHubError(err)
	}
	if closeErr != nil {
		return closeErr
	}
	return nil
}

func (s *fileSearchService) Subscribe(ctx context.Context, id string) (<-chan types.FileSearchEvent, func(), error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil, invalidError("file search id is required", nil)
	}
	ch, cancel, err := s.hub.Subscribe(ctx, id)
	if err != nil {
		return nil, nil, mapFileSearchHubError(err)
	}
	return ch, cancel, nil
}

func (s *fileSearchService) consumeRuntimeEvents(searchID string, source FileSearchEventSource) {
	if source == nil {
		return
	}
	events := source.Events()
	if events == nil {
		return
	}
	for event := range events {
		s.applyRuntimeEvent(searchID, event)
	}
}

func (s *fileSearchService) applyRuntimeEvent(searchID string, event types.FileSearchEvent) {
	current, _, err := s.lookup(searchID)
	if err != nil {
		return
	}
	next, normalized, terminal := applyFileSearchRuntimeEvent(current, event, time.Now().UTC())
	if err := s.hub.Publish(searchID, next, normalized, terminal); err != nil && !isFileSearchHubNotFound(err) {
		return
	}
}

func (s *fileSearchService) normalizeUpdateRequest(ctx context.Context, current *types.FileSearchSession, req types.FileSearchUpdateRequest) (types.FileSearchUpdateRequest, error) {
	normalized := req
	if req.Query != nil {
		value := strings.TrimSpace(*req.Query)
		normalized.Query = &value
	}
	if req.Limit != nil {
		if *req.Limit <= 0 {
			return types.FileSearchUpdateRequest{}, invalidError("limit must be positive", nil)
		}
		value := *req.Limit
		normalized.Limit = &value
	}
	if req.Scope != nil {
		scope, err := s.scopeResolver.ResolveScope(ctx, *req.Scope)
		if err != nil {
			return types.FileSearchUpdateRequest{}, err
		}
		if providers.Normalize(scope.Provider) != providers.Normalize(current.Provider) {
			return types.FileSearchUpdateRequest{}, invalidError("file search provider cannot change after start", nil)
		}
		normalized.Scope = &scope
	}
	return normalized, nil
}

func (s *fileSearchService) lookup(searchID string) (*types.FileSearchSession, FileSearchRuntime, error) {
	session, runtime, err := s.hub.Lookup(searchID)
	if err != nil {
		return nil, nil, mapFileSearchHubError(err)
	}
	return session, runtime, nil
}

func mapFileSearchHubError(err error) error {
	if err == nil {
		return nil
	}
	if isFileSearchHubNotFound(err) {
		return notFoundError("file search not found", err)
	}
	return unavailableError("file search state is unavailable", err)
}
