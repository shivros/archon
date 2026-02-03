package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"control/internal/types"
)

func TestAppState404IsIgnored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/state" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	ctx := context.Background()
	state, err := c.GetAppState(ctx)
	if err != nil {
		t.Fatalf("GetAppState error: %v", err)
	}
	if state == nil {
		t.Fatalf("expected non-nil state")
	}

	updated, err := c.UpdateAppState(ctx, &types.AppState{SidebarCollapsed: true})
	if err != nil {
		t.Fatalf("UpdateAppState error: %v", err)
	}
	if updated == nil {
		t.Fatalf("expected non-nil updated state")
	}
}
