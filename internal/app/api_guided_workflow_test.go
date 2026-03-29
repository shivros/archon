package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"control/internal/client"
	"control/internal/guidedworkflows"
)

func TestClientAPIStopWorkflowRunDelegatesToClient(t *testing.T) {
	t.Parallel()

	called := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/workflow-runs/gwf-1/stop" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"gwf-1","status":"stopped"}`))
	}))
	defer server.Close()

	raw := client.NewWithBaseURL(server.URL, "token")
	api := NewClientAPI(raw)

	run, err := api.StopWorkflowRun(context.Background(), "gwf-1")
	if err != nil {
		t.Fatalf("StopWorkflowRun error: %v", err)
	}
	if run == nil || run.ID != "gwf-1" || run.Status != guidedworkflows.WorkflowRunStatusStopped {
		t.Fatalf("unexpected stop workflow response: %#v", run)
	}
	if called != 1 {
		t.Fatalf("expected one stop request, got %d", called)
	}
}

func TestClientAPISnapshotFetchAvailabilityReflectsClientPresence(t *testing.T) {
	t.Parallel()

	var nilAPI *ClientAPI
	if nilAPI.SnapshotFetchAvailable() {
		t.Fatalf("expected nil ClientAPI to report snapshot fetch unavailable")
	}

	apiWithoutClient := NewClientAPI(nil)
	if apiWithoutClient.SnapshotFetchAvailable() {
		t.Fatalf("expected ClientAPI without client to report snapshot fetch unavailable")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	raw := client.NewWithBaseURL(server.URL, "token")
	apiWithClient := NewClientAPI(raw)
	if !apiWithClient.SnapshotFetchAvailable() {
		t.Fatalf("expected ClientAPI with client to report snapshot fetch available")
	}
}
