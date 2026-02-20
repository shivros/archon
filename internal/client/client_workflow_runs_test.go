package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control/internal/guidedworkflows"
)

func TestWorkflowRunClientEndpoints(t *testing.T) {
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/workflow-templates":
			seen["templates"] = true
			_, _ = w.Write([]byte(`{"templates":[{"id":"solid_phase_delivery","name":"SOLID Phase Delivery","description":"default"}]}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/workflow-runs":
			seen["list"] = true
			_, _ = w.Write([]byte(`{"runs":[{"id":"gwf-1","status":"running","template_id":"solid_phase_delivery","template_name":"SOLID Phase Delivery"}]}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/workflow-runs":
			var req CreateWorkflowRunRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode create req: %v", err)
			}
			if req.WorkspaceID != "ws1" || req.WorktreeID != "wt1" || req.SessionID != "s1" {
				t.Fatalf("unexpected create request payload: %+v", req)
			}
			if req.UserPrompt != "Fix workflow startup path" {
				t.Fatalf("unexpected create request payload: %+v", req)
			}
			seen["create"] = true
			_, _ = w.Write([]byte(`{"id":"gwf-1","status":"created","template_id":"solid_phase_delivery","template_name":"SOLID Phase Delivery"}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/workflow-runs/gwf-1/start":
			seen["start"] = true
			_, _ = w.Write([]byte(`{"id":"gwf-1","status":"running","template_id":"solid_phase_delivery","template_name":"SOLID Phase Delivery"}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/workflow-runs/gwf-1/dismiss":
			seen["dismiss"] = true
			_, _ = w.Write([]byte(`{"id":"gwf-1","status":"running","dismissed_at":"2026-02-17T00:00:00Z","template_id":"solid_phase_delivery","template_name":"SOLID Phase Delivery"}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/workflow-runs/gwf-1/undismiss":
			seen["undismiss"] = true
			_, _ = w.Write([]byte(`{"id":"gwf-1","status":"running","template_id":"solid_phase_delivery","template_name":"SOLID Phase Delivery"}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/workflow-runs/gwf-1":
			seen["get"] = true
			_, _ = w.Write([]byte(`{"id":"gwf-1","status":"running","template_id":"solid_phase_delivery","template_name":"SOLID Phase Delivery"}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/workflow-runs/gwf-1/timeline":
			seen["timeline"] = true
			_, _ = w.Write([]byte(`{"timeline":[{"run_id":"gwf-1","type":"step_completed","message":"implementation complete"}]}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/workflow-runs/gwf-1/decision":
			var req WorkflowRunDecisionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode decision req: %v", err)
			}
			if req.Action != guidedworkflows.DecisionActionApproveContinue || strings.TrimSpace(req.DecisionID) != "cd-1" {
				t.Fatalf("unexpected decision payload: %+v", req)
			}
			seen["decision"] = true
			_, _ = w.Write([]byte(`{"id":"gwf-1","status":"running","template_id":"solid_phase_delivery","template_name":"SOLID Phase Delivery"}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/workflow-runs/metrics":
			seen["metrics_get"] = true
			_, _ = w.Write([]byte(`{"enabled":true,"runs_started":2,"runs_completed":1,"pause_count":1}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/v1/workflow-runs/metrics/reset":
			seen["metrics_reset"] = true
			_, _ = w.Write([]byte(`{"enabled":true,"runs_started":0,"runs_completed":0,"pause_count":0}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
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
	created, err := c.CreateWorkflowRun(ctx, CreateWorkflowRunRequest{
		TemplateID:  guidedworkflows.TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws1",
		WorktreeID:  "wt1",
		SessionID:   "s1",
		UserPrompt:  "Fix workflow startup path",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun error: %v", err)
	}
	if created == nil || created.ID != "gwf-1" {
		t.Fatalf("unexpected created run: %#v", created)
	}

	runs, err := c.ListWorkflowRuns(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowRuns error: %v", err)
	}
	if len(runs) != 1 || runs[0] == nil || runs[0].ID != "gwf-1" {
		t.Fatalf("unexpected runs list: %#v", runs)
	}

	templates, err := c.ListWorkflowTemplates(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowTemplates error: %v", err)
	}
	if len(templates) != 1 || strings.TrimSpace(templates[0].ID) != guidedworkflows.TemplateIDSolidPhaseDelivery {
		t.Fatalf("unexpected workflow template list: %#v", templates)
	}

	started, err := c.StartWorkflowRun(ctx, "gwf-1")
	if err != nil {
		t.Fatalf("StartWorkflowRun error: %v", err)
	}
	if started == nil || started.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("unexpected started run: %#v", started)
	}

	dismissed, err := c.DismissWorkflowRun(ctx, "gwf-1")
	if err != nil {
		t.Fatalf("DismissWorkflowRun error: %v", err)
	}
	if dismissed == nil || dismissed.DismissedAt == nil {
		t.Fatalf("unexpected dismissed run: %#v", dismissed)
	}

	undismissed, err := c.UndismissWorkflowRun(ctx, "gwf-1")
	if err != nil {
		t.Fatalf("UndismissWorkflowRun error: %v", err)
	}
	if undismissed == nil || undismissed.DismissedAt != nil {
		t.Fatalf("unexpected undismissed run: %#v", undismissed)
	}

	run, err := c.GetWorkflowRun(ctx, "gwf-1")
	if err != nil {
		t.Fatalf("GetWorkflowRun error: %v", err)
	}
	if run == nil || run.ID != "gwf-1" {
		t.Fatalf("unexpected run payload: %#v", run)
	}

	timeline, err := c.GetWorkflowRunTimeline(ctx, "gwf-1")
	if err != nil {
		t.Fatalf("GetWorkflowRunTimeline error: %v", err)
	}
	if len(timeline) != 1 || timeline[0].Type != "step_completed" {
		t.Fatalf("unexpected timeline payload: %#v", timeline)
	}

	decided, err := c.DecideWorkflowRun(ctx, "gwf-1", WorkflowRunDecisionRequest{
		Action:     guidedworkflows.DecisionActionApproveContinue,
		DecisionID: "cd-1",
	})
	if err != nil {
		t.Fatalf("DecideWorkflowRun error: %v", err)
	}
	if decided == nil || decided.ID != "gwf-1" {
		t.Fatalf("unexpected decision response: %#v", decided)
	}

	metrics, err := c.GetWorkflowRunMetrics(ctx)
	if err != nil {
		t.Fatalf("GetWorkflowRunMetrics error: %v", err)
	}
	if metrics == nil || metrics.RunsStarted != 2 || metrics.PauseCount != 1 {
		t.Fatalf("unexpected metrics response: %#v", metrics)
	}

	reset, err := c.ResetWorkflowRunMetrics(ctx)
	if err != nil {
		t.Fatalf("ResetWorkflowRunMetrics error: %v", err)
	}
	if reset == nil || reset.RunsStarted != 0 || reset.PauseCount != 0 {
		t.Fatalf("unexpected metrics reset response: %#v", reset)
	}

	for _, key := range []string{"templates", "list", "create", "start", "dismiss", "undismiss", "get", "timeline", "decision", "metrics_get", "metrics_reset"} {
		if !seen[key] {
			t.Fatalf("expected request %q to be executed", key)
		}
	}
}

func TestListWorkflowTemplatesReturnsErrorForNonOKResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/workflow-templates" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"guided workflow request failed"}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	templates, err := c.ListWorkflowTemplates(context.Background())
	if err == nil {
		t.Fatalf("expected error when template endpoint is unavailable")
	}
	if templates != nil {
		t.Fatalf("expected nil templates on error, got %#v", templates)
	}
}
