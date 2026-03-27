package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"control/internal/apicode"
	"control/internal/logging"
	"control/internal/providers"
	"control/internal/types"
)

type stubFileSearchCapabilityResolver struct {
	caps map[string]providers.Capabilities
}

func (s stubFileSearchCapabilityResolver) Capabilities(provider string) providers.Capabilities {
	return s.caps[providers.Normalize(provider)]
}

type stubFileSearchRuntime struct {
	session *types.FileSearchSession
	events  chan types.FileSearchEvent
}

func (r *stubFileSearchRuntime) Snapshot() *types.FileSearchSession {
	return cloneFileSearchSession(r.session)
}

func (r *stubFileSearchRuntime) Update(_ context.Context, req types.FileSearchUpdateRequest) (*types.FileSearchSession, error) {
	if req.Scope != nil {
		r.session.Scope = *req.Scope
		r.session.Provider = req.Scope.Provider
	}
	if req.Query != nil {
		r.session.Query = *req.Query
	}
	if req.Limit != nil {
		r.session.Limit = *req.Limit
	}
	return cloneFileSearchSession(r.session), nil
}

func (r *stubFileSearchRuntime) Close(context.Context) error {
	close(r.events)
	return nil
}

func (r *stubFileSearchRuntime) Events() <-chan types.FileSearchEvent {
	return r.events
}

type stubFileSearchIDGenerator struct {
	id string
}

func (g stubFileSearchIDGenerator) NewID() (string, error) {
	return g.id, nil
}

type failingFileSearchIDGenerator struct {
	err error
}

func (g failingFileSearchIDGenerator) NewID() (string, error) {
	return "", g.err
}

type stubFileSearchProvider struct {
	runtime *stubFileSearchRuntime
	lastReq FileSearchProviderStartRequest
	err     error
}

func (p *stubFileSearchProvider) Start(_ context.Context, req FileSearchProviderStartRequest) (FileSearchRuntime, error) {
	if p.err != nil {
		return nil, p.err
	}
	p.lastReq = req
	if p.runtime == nil {
		p.runtime = &stubFileSearchRuntime{events: make(chan types.FileSearchEvent, 8)}
	}
	if p.runtime.session == nil {
		p.runtime.session = &types.FileSearchSession{
			ID:        req.SearchID,
			Provider:  req.Provider,
			Scope:     req.Scope,
			Query:     req.Query,
			Limit:     req.Limit,
			Status:    types.FileSearchStatusActive,
			CreatedAt: req.CreatedAt,
		}
	}
	return p.runtime, nil
}

type snapshotOnlyFileSearchRuntime struct {
	session   *types.FileSearchSession
	updateErr error
	closeErr  error
}

func (r *snapshotOnlyFileSearchRuntime) Snapshot() *types.FileSearchSession {
	return cloneFileSearchSession(r.session)
}

func (r *snapshotOnlyFileSearchRuntime) Update(_ context.Context, req types.FileSearchUpdateRequest) (*types.FileSearchSession, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	if req.Query != nil {
		r.session.Query = *req.Query
	}
	if req.Limit != nil {
		r.session.Limit = *req.Limit
	}
	if req.Scope != nil {
		r.session.Scope = *req.Scope
		r.session.Provider = req.Scope.Provider
	}
	return cloneFileSearchSession(r.session), nil
}

func (r *snapshotOnlyFileSearchRuntime) Close(context.Context) error {
	return r.closeErr
}

type fakeFileSearchHub struct {
	registerErr  error
	lookupErr    error
	publishErr   error
	subscribeErr error
	session      *types.FileSearchSession
	runtime      FileSearchRuntime
	eventCh      chan types.FileSearchEvent
	cancelCount  int
}

func (h *fakeFileSearchHub) Register(_ string, session *types.FileSearchSession, runtime FileSearchRuntime) error {
	if h.registerErr != nil {
		return h.registerErr
	}
	h.session = cloneFileSearchSession(session)
	h.runtime = runtime
	if h.eventCh == nil {
		h.eventCh = make(chan types.FileSearchEvent, 8)
	}
	return nil
}

func (h *fakeFileSearchHub) Lookup(_ string) (*types.FileSearchSession, FileSearchRuntime, error) {
	if h.lookupErr != nil {
		return nil, nil, h.lookupErr
	}
	return cloneFileSearchSession(h.session), h.runtime, nil
}

func (h *fakeFileSearchHub) Publish(_ string, session *types.FileSearchSession, event types.FileSearchEvent, terminal bool) error {
	if h.publishErr != nil {
		return h.publishErr
	}
	h.session = cloneFileSearchSession(session)
	if h.eventCh != nil {
		h.eventCh <- event
		if terminal {
			close(h.eventCh)
		}
	}
	return nil
}

func (h *fakeFileSearchHub) Subscribe(_ context.Context, _ string) (<-chan types.FileSearchEvent, func(), error) {
	if h.subscribeErr != nil {
		return nil, nil, h.subscribeErr
	}
	if h.eventCh == nil {
		h.eventCh = make(chan types.FileSearchEvent, 8)
	}
	return h.eventCh, func() { h.cancelCount++ }, nil
}

type notifyingFileSearchHub struct {
	FileSearchHub
	published chan types.FileSearchEvent
}

func newNotifyingFileSearchHub(base FileSearchHub) *notifyingFileSearchHub {
	if base == nil {
		base = NewMemoryFileSearchHub()
	}
	return &notifyingFileSearchHub{
		FileSearchHub: base,
		published:     make(chan types.FileSearchEvent, 32),
	}
}

func (h *notifyingFileSearchHub) Publish(searchID string, session *types.FileSearchSession, event types.FileSearchEvent, terminal bool) error {
	if err := h.FileSearchHub.Publish(searchID, session, event, terminal); err != nil {
		return err
	}
	select {
	case h.published <- event:
	default:
	}
	return nil
}

type errSessionIndexStore struct{ err error }

func (s errSessionIndexStore) ListRecords(context.Context) ([]*types.SessionRecord, error) {
	return nil, s.err
}
func (s errSessionIndexStore) GetRecord(context.Context, string) (*types.SessionRecord, bool, error) {
	return nil, false, s.err
}
func (s errSessionIndexStore) UpsertRecord(context.Context, *types.SessionRecord) (*types.SessionRecord, error) {
	return nil, s.err
}
func (s errSessionIndexStore) DeleteRecord(context.Context, string) error { return s.err }

type errSessionMetaStore struct{ err error }

func (s errSessionMetaStore) List(context.Context) ([]*types.SessionMeta, error) { return nil, s.err }
func (s errSessionMetaStore) Get(context.Context, string) (*types.SessionMeta, bool, error) {
	return nil, false, s.err
}
func (s errSessionMetaStore) Upsert(context.Context, *types.SessionMeta) (*types.SessionMeta, error) {
	return nil, s.err
}
func (s errSessionMetaStore) Delete(context.Context, string) error { return s.err }

type fileSearchStubSessionIndexStore struct {
	record *types.SessionRecord
}

func (s fileSearchStubSessionIndexStore) ListRecords(context.Context) ([]*types.SessionRecord, error) {
	if s.record == nil {
		return nil, nil
	}
	return []*types.SessionRecord{s.record}, nil
}

func (s fileSearchStubSessionIndexStore) GetRecord(_ context.Context, sessionID string) (*types.SessionRecord, bool, error) {
	if s.record == nil || s.record.Session == nil || s.record.Session.ID != sessionID {
		return nil, false, nil
	}
	return s.record, true, nil
}

func (s fileSearchStubSessionIndexStore) UpsertRecord(context.Context, *types.SessionRecord) (*types.SessionRecord, error) {
	return nil, nil
}

func (s fileSearchStubSessionIndexStore) DeleteRecord(context.Context, string) error {
	return nil
}

type fileSearchStubSessionMetaStore struct {
	meta *types.SessionMeta
}

func (s fileSearchStubSessionMetaStore) List(context.Context) ([]*types.SessionMeta, error) {
	if s.meta == nil {
		return nil, nil
	}
	return []*types.SessionMeta{s.meta}, nil
}

func (s fileSearchStubSessionMetaStore) Get(_ context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	if s.meta == nil || s.meta.SessionID != sessionID {
		return nil, false, nil
	}
	return s.meta, true, nil
}

func (s fileSearchStubSessionMetaStore) Upsert(context.Context, *types.SessionMeta) (*types.SessionMeta, error) {
	return nil, nil
}

func (s fileSearchStubSessionMetaStore) Delete(context.Context, string) error {
	return nil
}

func TestFileSearchServiceStartReturnsUnsupportedForProviderWithoutCapability(t *testing.T) {
	service := NewFileSearchService(nil, logging.Nop())

	_, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{Provider: "claude"},
		Query: "main",
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected service error, got %T", err)
	}
	if serviceErr.Kind != ServiceErrorInvalid || serviceErr.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("unexpected service error: %#v", serviceErr)
	}
}

func TestFileSearchServiceStartResolvesSessionScopeBeforeCapabilityCheck(t *testing.T) {
	scopeResolver := NewDaemonFileSearchScopeResolver(nil, &Stores{
		Sessions: fileSearchStubSessionIndexStore{
			record: &types.SessionRecord{
				Session: &types.Session{ID: "sess-1", Provider: "claude", Cwd: "/repo"},
			},
		},
		SessionMeta: fileSearchStubSessionMetaStore{
			meta: &types.SessionMeta{SessionID: "sess-1", WorkspaceID: "ws-1", WorktreeID: "wt-1"},
		},
	})
	service := NewFileSearchService(scopeResolver, logging.Nop())

	_, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{SessionID: "sess-1"},
		Query: "main",
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected service error, got %T", err)
	}
	if serviceErr.Kind != ServiceErrorInvalid || serviceErr.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("unexpected service error: %#v", serviceErr)
	}
}

func TestFileSearchServiceOrchestratesStubProviderRuntime(t *testing.T) {
	provider := &stubFileSearchProvider{
		runtime: &stubFileSearchRuntime{
			events: make(chan types.FileSearchEvent, 8),
		},
	}
	service, ok := NewFileSearchService(
		nil,
		logging.Nop(),
		WithFileSearchCapabilityResolver(stubFileSearchCapabilityResolver{
			caps: map[string]providers.Capabilities{
				"stub": {SupportsFileSearch: true},
			},
		}),
		WithFileSearchIDGenerator(stubFileSearchIDGenerator{id: "fs-1"}),
		WithFileSearchProviderRegistry(NewFileSearchProviderRegistry(map[string]FileSearchProvider{
			"stub": provider,
		})),
	).(*fileSearchService)
	if !ok {
		t.Fatalf("expected concrete file search service")
	}

	search, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{Provider: "stub", WorkspaceID: "ws-1"},
		Query: "main",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if search == nil || search.ID == "" || search.Provider != "stub" || search.Status != types.FileSearchStatusActive {
		t.Fatalf("unexpected search: %#v", search)
	}
	if provider.lastReq.Provider != "stub" || provider.lastReq.Scope.WorkspaceID != "ws-1" {
		t.Fatalf("unexpected provider start request: %#v", provider.lastReq)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, stop, err := service.Subscribe(ctx, search.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	query := "main.go"
	limit := 9
	updated, err := service.Update(context.Background(), search.ID, types.FileSearchUpdateRequest{
		Query: &query,
		Limit: &limit,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Query != "main.go" || updated.Limit != 9 {
		t.Fatalf("unexpected updated search: %#v", updated)
	}

	select {
	case event := <-events:
		if event.Kind != types.FileSearchEventUpdated || event.Query != "main.go" || event.Limit != 9 {
			t.Fatalf("unexpected update event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for updated event")
	}

	provider.runtime.events <- types.FileSearchEvent{
		Kind:     types.FileSearchEventResults,
		SearchID: search.ID,
		Candidates: []types.FileSearchCandidate{
			{Path: "main.go", DisplayPath: "./main.go"},
		},
	}

	select {
	case event := <-events:
		if event.Kind != types.FileSearchEventResults || len(event.Candidates) != 1 || event.Candidates[0].Path != "main.go" {
			t.Fatalf("unexpected runtime event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for runtime event")
	}

	if err := service.Close(context.Background(), search.ID); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatalf("expected closed event before stream closes")
		}
		if event.Kind != types.FileSearchEventClosed || event.Status != types.FileSearchStatusClosed {
			t.Fatalf("unexpected close event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for closed event")
	}
}

func TestFileSearchServiceStartWithQueryLateSubscribeReceivesInitialResults(t *testing.T) {
	hub := newNotifyingFileSearchHub(NewMemoryFileSearchHub())
	service := NewFileSearchService(
		nil,
		logging.Nop(),
		WithFileSearchCapabilityResolver(stubFileSearchCapabilityResolver{
			caps: map[string]providers.Capabilities{
				"stub": {SupportsFileSearch: true},
			},
		}),
		WithFileSearchIDGenerator(stubFileSearchIDGenerator{id: "fs-1"}),
		WithFileSearchProviderRegistry(NewFileSearchProviderRegistry(map[string]FileSearchProvider{
			"stub": fileSearchProviderFunc(func(_ context.Context, req FileSearchProviderStartRequest) (FileSearchRuntime, error) {
				session := &types.FileSearchSession{
					ID:        req.SearchID,
					Provider:  req.Provider,
					Scope:     req.Scope,
					Query:     req.Query,
					Limit:     req.Limit,
					Status:    types.FileSearchStatusActive,
					CreatedAt: req.CreatedAt,
				}
				runtime := &stubFileSearchRuntime{
					session: session,
					events:  make(chan types.FileSearchEvent, 8),
				}
				occurredAt := time.Now().UTC()
				runtime.events <- buildFileSearchEvent(types.FileSearchEventStarted, session, nil, "", occurredAt)
				runtime.events <- buildFileSearchEvent(types.FileSearchEventResults, session, []types.FileSearchCandidate{
					{Path: "main.go", DisplayPath: "./main.go"},
				}, "", occurredAt)
				return runtime, nil
			}),
		})),
		WithFileSearchHub(hub),
	)

	search, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{Provider: "stub", WorkspaceID: "ws-1"},
		Query: "main",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitForFileSearchEventKind(t, hub.published, types.FileSearchEventResults)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, stop, err := service.Subscribe(ctx, search.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	select {
	case event := <-events:
		if event.Kind != types.FileSearchEventResults || event.Query != "main" || len(event.Candidates) != 1 || event.Candidates[0].Path != "main.go" {
			t.Fatalf("unexpected replayed event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for replayed results event")
	}
}

func TestFileSearchServiceUpdateLateSubscribeReplaysResultsOverUpdated(t *testing.T) {
	provider := &stubFileSearchProvider{
		runtime: &stubFileSearchRuntime{
			events: make(chan types.FileSearchEvent, 8),
		},
	}
	hub := newNotifyingFileSearchHub(NewMemoryFileSearchHub())
	service := NewFileSearchService(
		nil,
		logging.Nop(),
		WithFileSearchCapabilityResolver(stubFileSearchCapabilityResolver{
			caps: map[string]providers.Capabilities{
				"stub": {SupportsFileSearch: true},
			},
		}),
		WithFileSearchIDGenerator(stubFileSearchIDGenerator{id: "fs-1"}),
		WithFileSearchProviderRegistry(NewFileSearchProviderRegistry(map[string]FileSearchProvider{
			"stub": provider,
		})),
		WithFileSearchHub(hub),
	)

	search, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{Provider: "stub", WorkspaceID: "ws-1"},
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	query := "main.go"
	limit := 9
	provider.runtime.session = &types.FileSearchSession{
		ID:        search.ID,
		Provider:  "stub",
		Scope:     types.FileSearchScope{Provider: "stub", WorkspaceID: "ws-1"},
		Query:     "",
		Limit:     5,
		Status:    types.FileSearchStatusActive,
		CreatedAt: search.CreatedAt,
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = service.Update(context.Background(), search.ID, types.FileSearchUpdateRequest{
			Query: &query,
			Limit: &limit,
		})
	}()

	waitForFileSearchEventKind(t, hub.published, types.FileSearchEventUpdated)
	provider.runtime.events <- types.FileSearchEvent{
		Kind:     types.FileSearchEventResults,
		SearchID: search.ID,
		Provider: "stub",
		Scope:    types.FileSearchScope{Provider: "stub", WorkspaceID: "ws-1"},
		Query:    query,
		Limit:    limit,
		Status:   types.FileSearchStatusActive,
		Candidates: []types.FileSearchCandidate{
			{Path: "main.go", DisplayPath: "./main.go"},
		},
	}
	waitForFileSearchEventKind(t, hub.published, types.FileSearchEventResults)
	<-done

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, stop, err := service.Subscribe(ctx, search.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	select {
	case event := <-events:
		if event.Kind != types.FileSearchEventResults || event.Query != query || len(event.Candidates) != 1 || event.Candidates[0].Path != "main.go" {
			t.Fatalf("unexpected replayed event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for replayed results event")
	}
}

func TestFileSearchServiceStartReturnsUnavailableWhenRegistryMissingProvider(t *testing.T) {
	service := NewFileSearchService(
		nil,
		logging.Nop(),
		WithFileSearchCapabilityResolver(stubFileSearchCapabilityResolver{
			caps: map[string]providers.Capabilities{"stub": {SupportsFileSearch: true}},
		}),
	)
	_, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{Provider: "stub"},
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable || serviceErr.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestFileSearchServiceStartReturnsUnavailableWhenIDGenerationFails(t *testing.T) {
	service := NewFileSearchService(
		nil,
		logging.Nop(),
		WithFileSearchCapabilityResolver(stubFileSearchCapabilityResolver{
			caps: map[string]providers.Capabilities{"stub": {SupportsFileSearch: true}},
		}),
		WithFileSearchProviderRegistry(NewFileSearchProviderRegistry(map[string]FileSearchProvider{
			"stub": &stubFileSearchProvider{},
		})),
		WithFileSearchIDGenerator(failingFileSearchIDGenerator{err: errors.New("boom")}),
	)
	_, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{Provider: "stub"},
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestFileSearchServiceStartReturnsProviderStartError(t *testing.T) {
	service := NewFileSearchService(
		nil,
		logging.Nop(),
		WithFileSearchCapabilityResolver(stubFileSearchCapabilityResolver{
			caps: map[string]providers.Capabilities{"stub": {SupportsFileSearch: true}},
		}),
		WithFileSearchProviderRegistry(NewFileSearchProviderRegistry(map[string]FileSearchProvider{
			"stub": &stubFileSearchProvider{err: errors.New("start failed")},
		})),
		WithFileSearchIDGenerator(stubFileSearchIDGenerator{id: "fs-1"}),
	)
	if _, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{Provider: "stub"},
	}); err == nil || err.Error() != "start failed" {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestFileSearchServiceStartRejectsNilRuntime(t *testing.T) {
	service := NewFileSearchService(
		nil,
		logging.Nop(),
		WithFileSearchCapabilityResolver(stubFileSearchCapabilityResolver{
			caps: map[string]providers.Capabilities{"stub": {SupportsFileSearch: true}},
		}),
		WithFileSearchProviderRegistry(NewFileSearchProviderRegistry(map[string]FileSearchProvider{
			"stub": &stubFileSearchProvider{runtime: nil},
		})),
		WithFileSearchIDGenerator(stubFileSearchIDGenerator{id: "fs-1"}),
		WithFileSearchHub(&fakeFileSearchHub{registerErr: nil}),
	)
	// Force nil runtime by overriding provider Start behavior with nil runtime and no error.
	serviceImpl := service.(*fileSearchService)
	serviceImpl.runtimeRegistry = NewFileSearchProviderRegistry(map[string]FileSearchProvider{
		"stub": fileSearchProviderFunc(func(context.Context, FileSearchProviderStartRequest) (FileSearchRuntime, error) {
			return nil, nil
		}),
	})
	_, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{Provider: "stub"},
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestFileSearchServiceStartReturnsUnavailableWhenHubRegisterFails(t *testing.T) {
	service := NewFileSearchService(
		nil,
		logging.Nop(),
		WithFileSearchCapabilityResolver(stubFileSearchCapabilityResolver{
			caps: map[string]providers.Capabilities{"stub": {SupportsFileSearch: true}},
		}),
		WithFileSearchProviderRegistry(NewFileSearchProviderRegistry(map[string]FileSearchProvider{
			"stub": &stubFileSearchProvider{},
		})),
		WithFileSearchIDGenerator(stubFileSearchIDGenerator{id: "fs-1"}),
		WithFileSearchHub(&fakeFileSearchHub{registerErr: errors.New("register failed")}),
	)
	_, err := service.Start(context.Background(), types.FileSearchStartRequest{
		Scope: types.FileSearchScope{Provider: "stub"},
	})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestFileSearchServiceUpdateMapsHubPublishError(t *testing.T) {
	runtime := &snapshotOnlyFileSearchRuntime{
		session: &types.FileSearchSession{ID: "fs-1", Provider: "stub", Scope: types.FileSearchScope{Provider: "stub"}, Status: types.FileSearchStatusActive},
	}
	hub := &fakeFileSearchHub{
		session:    cloneFileSearchSession(runtime.session),
		runtime:    runtime,
		publishErr: errors.New("publish failed"),
	}
	service := NewFileSearchService(nil, logging.Nop(), WithFileSearchHub(hub))
	query := "next"
	_, err := service.Update(context.Background(), "fs-1", types.FileSearchUpdateRequest{Query: &query})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestFileSearchServiceCloseMapsHubLookupNotFound(t *testing.T) {
	hub := &fakeFileSearchHub{lookupErr: errFileSearchHubNotFound}
	service := NewFileSearchService(nil, logging.Nop(), WithFileSearchHub(hub))
	err := service.Close(context.Background(), "fs-1")
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorNotFound {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestFileSearchServiceCloseReturnsRuntimeCloseError(t *testing.T) {
	runtime := &snapshotOnlyFileSearchRuntime{
		session:  &types.FileSearchSession{ID: "fs-1", Provider: "stub", Scope: types.FileSearchScope{Provider: "stub"}, Status: types.FileSearchStatusActive},
		closeErr: errors.New("close failed"),
	}
	hub := &fakeFileSearchHub{
		session: cloneFileSearchSession(runtime.session),
		runtime: runtime,
	}
	service := NewFileSearchService(nil, logging.Nop(), WithFileSearchHub(hub))
	err := service.Close(context.Background(), "fs-1")
	if err == nil || err.Error() != "close failed" {
		t.Fatalf("expected runtime close error, got %v", err)
	}
}

func TestFileSearchServiceSubscribeMapsHubNotFound(t *testing.T) {
	service := NewFileSearchService(nil, logging.Nop(), WithFileSearchHub(&fakeFileSearchHub{subscribeErr: errFileSearchHubNotFound}))
	if _, _, err := service.Subscribe(context.Background(), "fs-1"); err == nil {
		t.Fatalf("expected not found error")
	}
}

func waitForFileSearchEventKind(t *testing.T, ch <-chan types.FileSearchEvent, kind types.FileSearchEventKind) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case event := <-ch:
			if event.Kind == kind {
				return
			}
		case <-deadline:
			t.Fatalf("timeout waiting for event kind %q", kind)
		}
	}
}

type fileSearchProviderFunc func(context.Context, FileSearchProviderStartRequest) (FileSearchRuntime, error)

func (f fileSearchProviderFunc) Start(ctx context.Context, req FileSearchProviderStartRequest) (FileSearchRuntime, error) {
	return f(ctx, req)
}
