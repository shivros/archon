package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"control/internal/apicode"
	"control/internal/types"
)

func TestClientStartFileSearch(t *testing.T) {
	var (
		seenMethod string
		seenPath   string
		seenBody   StartFileSearchRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.FileSearchSession{
			ID:       "fs-1",
			Provider: "codex",
			Scope:    seenBody.Scope,
			Query:    seenBody.Query,
			Limit:    seenBody.Limit,
			Status:   types.FileSearchStatusActive,
		})
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http:    &http.Client{Timeout: 2 * time.Second},
	}
	search, err := c.StartFileSearch(context.Background(), StartFileSearchRequest{
		Scope: types.FileSearchScope{Provider: "codex", WorkspaceID: "ws-1"},
		Query: "main",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("StartFileSearch error: %v", err)
	}
	if seenMethod != http.MethodPost || seenPath != "/v1/file-searches" {
		t.Fatalf("unexpected request: %s %s", seenMethod, seenPath)
	}
	if seenBody.Scope.Provider != "codex" || seenBody.Query != "main" || seenBody.Limit != 5 {
		t.Fatalf("unexpected request body: %#v", seenBody)
	}
	if search == nil || search.ID != "fs-1" || search.Status != types.FileSearchStatusActive {
		t.Fatalf("unexpected response payload: %#v", search)
	}
}

func TestClientUpdateAndCloseFileSearch(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPatch:
			_ = json.NewEncoder(w).Encode(types.FileSearchSession{
				ID:       "fs-1",
				Provider: "codex",
				Query:    "main.go",
				Limit:    9,
				Status:   types.FileSearchStatusActive,
			})
		case http.MethodDelete:
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http:    &http.Client{Timeout: 2 * time.Second},
	}
	query := "main.go"
	limit := 9
	search, err := c.UpdateFileSearch(context.Background(), "fs-1", UpdateFileSearchRequest{
		Query: &query,
		Limit: &limit,
	})
	if err != nil {
		t.Fatalf("UpdateFileSearch error: %v", err)
	}
	if search == nil || search.Query != "main.go" || search.Limit != 9 {
		t.Fatalf("unexpected updated search: %#v", search)
	}
	if err := c.CloseFileSearch(context.Background(), "fs-1"); err != nil {
		t.Fatalf("CloseFileSearch error: %v", err)
	}
	if len(calls) != 2 || calls[0] != "PATCH /v1/file-searches/fs-1" || calls[1] != "DELETE /v1/file-searches/fs-1" {
		t.Fatalf("unexpected calls: %#v", calls)
	}
}

func TestClientStartFileSearchReturnsAPIErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"file search is not supported for provider","code":"file_search_unsupported"}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http:    &http.Client{Timeout: 2 * time.Second},
	}
	_, err := c.StartFileSearch(context.Background(), StartFileSearchRequest{
		Scope: types.FileSearchScope{Provider: "codex"},
	})
	apiErr := asAPIError(err)
	if apiErr == nil {
		t.Fatalf("expected api error, got %v", err)
	}
	if apiErr.Code != apicode.ErrorCodeFileSearchUnsupported {
		t.Fatalf("expected file search unsupported code, got %#v", apiErr)
	}
}

func TestClientUpdateAndCloseFileSearchRejectBlankID(t *testing.T) {
	c := &Client{}
	query := "main"
	if _, err := c.UpdateFileSearch(context.Background(), "   ", UpdateFileSearchRequest{Query: &query}); err == nil {
		t.Fatalf("expected blank id update to fail")
	}
	if err := c.CloseFileSearch(context.Background(), "   "); err == nil {
		t.Fatalf("expected blank id close to fail")
	}
}
