package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"control/internal/types"
)

func TestOpenCodeHistoryReconcilerSyncValidationErrors(t *testing.T) {
	_, err := openCodeHistoryReconciler{}.Sync(context.Background(), 10)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)

	rec := newOpenCodeHistoryReconciler(
		&types.Session{ID: "s1", Provider: "opencode"},
		&types.SessionMeta{SessionID: "s1"},
		openCodeHistoryReconcilerStore{},
		nil,
	)
	_, err = rec.Sync(context.Background(), 10)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
}

func TestOpenCodeHistoryReconcilerSyncFetchFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	t.Setenv("OPENCODE_BASE_URL", server.URL)
	rec := newOpenCodeHistoryReconciler(
		&types.Session{ID: "s1", Provider: "opencode", Cwd: "/tmp/opencode"},
		&types.SessionMeta{SessionID: "s1", ProviderSessionID: "remote-s-1"},
		openCodeHistoryReconcilerStore{},
		nil,
	)

	_, err := rec.Sync(context.Background(), 25)
	if err == nil {
		t.Fatalf("expected fetch error")
	}
}

func TestOpenCodeHistoryReconcilerSyncAppendFailureStillReturnsItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/remote-s-2/message":
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"info": map[string]any{
						"id":        "msg-user",
						"role":      "user",
						"createdAt": "2026-02-13T01:00:00Z",
					},
					"parts": []map[string]any{{"type": "text", "text": "hello"}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("OPENCODE_BASE_URL", server.URL)
	rec := newOpenCodeHistoryReconciler(
		&types.Session{ID: "s2", Provider: "opencode", Cwd: "/tmp/opencode"},
		&types.SessionMeta{SessionID: "s2", ProviderSessionID: "remote-s-2"},
		openCodeHistoryReconcilerStore{
			readSessionItems: func(string, int) ([]map[string]any, error) { return nil, nil },
			appendSessionItems: func(string, []map[string]any) error {
				return errors.New("append failed")
			},
		},
		nil,
	)

	result, err := rec.Sync(context.Background(), 20)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(result.items) != 1 {
		t.Fatalf("expected one returned item despite append failure, got %#v", result.items)
	}
	if len(result.backfilled) != 0 {
		t.Fatalf("expected no backfilled record on append failure, got %#v", result.backfilled)
	}
}
