package daemon

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

type healthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
}

func TestHealth(t *testing.T) {
	recorder := httptest.NewRecorder()
	api := &API{Version: "test-version"}

	api.Health(recorder, httptest.NewRequest("GET", "/health", nil))

	var resp healthResponse
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true")
	}
	if resp.Version != "test-version" {
		t.Fatalf("expected version 'test-version', got %q", resp.Version)
	}
	if resp.PID <= 0 {
		t.Fatalf("expected pid to be positive, got %d", resp.PID)
	}
}
