package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control/internal/apicode"
	"control/internal/logging"
	"control/internal/providers"
	"control/internal/types"
)

type stubHandlerFileSearchService struct {
	startReq   *types.FileSearchStartRequest
	updateID   string
	updateReq  *types.FileSearchUpdateRequest
	closeID    string
	eventCh    chan types.FileSearchEvent
	startResp  *types.FileSearchSession
	updateResp *types.FileSearchSession
	startErr   error
	updateErr  error
	closeErr   error
	subErr     error
}

func (s *stubHandlerFileSearchService) Start(_ context.Context, req types.FileSearchStartRequest) (*types.FileSearchSession, error) {
	copyReq := req
	s.startReq = &copyReq
	if s.startErr != nil {
		return nil, s.startErr
	}
	return s.startResp, nil
}

func (s *stubHandlerFileSearchService) Update(_ context.Context, id string, req types.FileSearchUpdateRequest) (*types.FileSearchSession, error) {
	s.updateID = id
	copyReq := req
	s.updateReq = &copyReq
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return s.updateResp, nil
}

func (s *stubHandlerFileSearchService) Close(_ context.Context, id string) error {
	s.closeID = id
	return s.closeErr
}

func (s *stubHandlerFileSearchService) Subscribe(_ context.Context, id string) (<-chan types.FileSearchEvent, func(), error) {
	if s.subErr != nil {
		return nil, nil, s.subErr
	}
	if s.eventCh == nil {
		s.eventCh = make(chan types.FileSearchEvent, 1)
	}
	return s.eventCh, func() {}, nil
}

func TestAPIFileSearchEndpointReturnsUnsupportedProviderError(t *testing.T) {
	api := &API{
		Version:      "test",
		FileSearches: NewFileSearchService(nil, logging.Nop()),
	}
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	body := bytes.NewBufferString(`{"scope":{"provider":"claude"},"query":"main"}`)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/file-searches", body)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post file search: %v", err)
	}
	defer closeTestCloser(t, resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var payload struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("expected unsupported code, got %#v", payload)
	}
}

func TestAPIFileSearchEventsRequireFollow(t *testing.T) {
	api := &API{
		Version:      "test",
		FileSearches: NewFileSearchService(nil, logging.Nop()),
	}
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/file-searches/fs-1/events", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get file search events: %v", err)
	}
	defer closeTestCloser(t, resp.Body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestAPIFileSearchEndpointCreatePatchDeleteSuccess(t *testing.T) {
	query := "main"
	updateQuery := "main.go"
	startLimit := 5
	updateLimit := 9
	service := &stubHandlerFileSearchService{
		startResp:  &types.FileSearchSession{ID: "fs-1", Provider: "codex", Query: query, Limit: startLimit, Status: types.FileSearchStatusActive},
		updateResp: &types.FileSearchSession{ID: "fs-1", Provider: "codex", Query: updateQuery, Limit: updateLimit, Status: types.FileSearchStatusActive},
	}
	api := &API{Version: "test", FileSearches: service}
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/file-searches", bytes.NewBufferString(`{"scope":{"provider":"codex"},"query":"main","limit":5}`))
	createReq.Header.Set("Authorization", "Bearer token")
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create file search: %v", err)
	}
	defer closeTestCloser(t, createResp.Body)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	if service.startReq == nil || service.startReq.Query != "main" || service.startReq.Limit != startLimit || service.startReq.Scope.Provider != "codex" {
		t.Fatalf("unexpected start request: %#v", service.startReq)
	}

	patchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/file-searches/fs-1", bytes.NewBufferString(`{"query":"main.go","limit":9}`))
	patchReq.Header.Set("Authorization", "Bearer token")
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch file search: %v", err)
	}
	defer closeTestCloser(t, patchResp.Body)
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", patchResp.StatusCode)
	}
	if service.updateID != "fs-1" || service.updateReq == nil || service.updateReq.Query == nil || *service.updateReq.Query != "main.go" || service.updateReq.Limit == nil || *service.updateReq.Limit != updateLimit {
		t.Fatalf("unexpected update call: id=%q req=%#v", service.updateID, service.updateReq)
	}

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/file-searches/fs-1", nil)
	deleteReq.Header.Set("Authorization", "Bearer token")
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete file search: %v", err)
	}
	defer closeTestCloser(t, deleteResp.Body)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteResp.StatusCode)
	}
	if service.closeID != "fs-1" {
		t.Fatalf("unexpected close id: %q", service.closeID)
	}
}

func TestAPIFileSearchEndpointsRejectInvalidJSONAndMethods(t *testing.T) {
	service := &stubHandlerFileSearchService{}
	api := &API{Version: "test", FileSearches: service}
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	for _, tc := range []struct {
		method string
		path   string
		body   string
		status int
	}{
		{method: http.MethodPost, path: "/v1/file-searches", body: "{", status: http.StatusBadRequest},
		{method: http.MethodPatch, path: "/v1/file-searches/fs-1", body: "{", status: http.StatusBadRequest},
		{method: http.MethodGet, path: "/v1/file-searches", status: http.StatusMethodNotAllowed},
		{method: http.MethodPost, path: "/v1/file-searches/fs-1/events?follow=1", status: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/v1/file-searches/fs-1/unknown", status: http.StatusNotFound},
	} {
		req, _ := http.NewRequest(tc.method, server.URL+tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		defer closeTestCloser(t, resp.Body)
		if resp.StatusCode != tc.status {
			t.Fatalf("%s %s: expected %d, got %d", tc.method, tc.path, tc.status, resp.StatusCode)
		}
	}
}

func TestAPIFileSearchEventsStreamSuccess(t *testing.T) {
	service := &stubHandlerFileSearchService{
		eventCh: make(chan types.FileSearchEvent, 1),
	}
	service.eventCh <- types.FileSearchEvent{
		Kind:     types.FileSearchEventResults,
		SearchID: "fs-1",
		Provider: "codex",
		Status:   types.FileSearchStatusActive,
	}
	close(service.eventCh)

	api := &API{Version: "test", FileSearches: service}
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/file-searches/fs-1/events?follow=1", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream file search events: %v", err)
	}
	defer closeTestCloser(t, resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	var body bytes.Buffer
	_, _ = body.ReadFrom(resp.Body)
	if !bytes.Contains(body.Bytes(), []byte(`"kind":"file_search.results"`)) {
		t.Fatalf("expected streamed event payload, got %q", body.String())
	}
}

func TestAPIFileSearchEventsLateSubscriberReceivesInitialResults(t *testing.T) {
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

	api := &API{Version: "test", FileSearches: service}
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/file-searches", bytes.NewBufferString(`{"scope":{"provider":"stub","workspace_id":"ws-1"},"query":"main","limit":5}`))
	createReq.Header.Set("Authorization", "Bearer token")
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create file search: %v", err)
	}
	defer closeTestCloser(t, createResp.Body)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	waitForFileSearchEventKind(t, hub.published, types.FileSearchEventResults)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	streamReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/file-searches/fs-1/events?follow=1", nil)
	streamReq.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("stream file search events: %v", err)
	}
	defer closeTestCloser(t, resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	event, err := readFirstFileSearchSSEEvent(resp.Body)
	if err != nil {
		t.Fatalf("read first sse event: %v", err)
	}
	if event.Kind != types.FileSearchEventResults || event.Query != "main" || len(event.Candidates) != 1 || event.Candidates[0].Path != "main.go" {
		t.Fatalf("unexpected replayed sse event: %#v", event)
	}
}

func TestAPIFileSearchEventsReturnsNotFoundAfterClose(t *testing.T) {
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
			"stub": &stubFileSearchProvider{
				runtime: &stubFileSearchRuntime{
					events: make(chan types.FileSearchEvent, 8),
				},
			},
		})),
	)

	api := &API{Version: "test", FileSearches: service}
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/file-searches", bytes.NewBufferString(`{"scope":{"provider":"stub"},"query":"main"}`))
	createReq.Header.Set("Authorization", "Bearer token")
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create file search: %v", err)
	}
	defer closeTestCloser(t, createResp.Body)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/file-searches/fs-1", nil)
	deleteReq.Header.Set("Authorization", "Bearer token")
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete file search: %v", err)
	}
	defer closeTestCloser(t, deleteResp.Body)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteResp.StatusCode)
	}

	eventsReq, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/file-searches/fs-1/events?follow=1", nil)
	eventsReq.Header.Set("Authorization", "Bearer token")
	eventsResp, err := http.DefaultClient.Do(eventsReq)
	if err != nil {
		t.Fatalf("get events after close: %v", err)
	}
	defer closeTestCloser(t, eventsResp.Body)
	if eventsResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", eventsResp.StatusCode)
	}
}

func readFirstFileSearchSSEEvent(body io.Reader) (types.FileSearchEvent, error) {
	scanner := bufio.NewScanner(body)
	var payload bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if payload.Len() == 0 {
				continue
			}
			var event types.FileSearchEvent
			if err := json.Unmarshal(payload.Bytes(), &event); err != nil {
				return types.FileSearchEvent{}, err
			}
			return event, nil
		}
		if strings.HasPrefix(line, "data:") {
			payload.WriteString(strings.TrimSpace(line[len("data:"):]))
		}
	}
	if err := scanner.Err(); err != nil {
		return types.FileSearchEvent{}, err
	}
	return types.FileSearchEvent{}, context.DeadlineExceeded
}
