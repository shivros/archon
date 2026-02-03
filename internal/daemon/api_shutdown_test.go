package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdownEndpoint(t *testing.T) {
	called := make(chan struct{}, 1)
	api := &API{
		Version: "test",
		Shutdown: func(ctx context.Context) error {
			called <- struct{}{}
			return nil
		},
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/shutdown", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("shutdown request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	select {
	case <-called:
	case <-time.After(1 * time.Second):
		t.Fatalf("shutdown not called")
	}
}
