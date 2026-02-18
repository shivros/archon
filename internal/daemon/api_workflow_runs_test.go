package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control/internal/guidedworkflows"
)

func TestWorkflowRunEndpointsLifecycle(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:  guidedworkflows.TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if created.ID == "" {
		t.Fatalf("expected run id")
	}
	if created.Status != guidedworkflows.WorkflowRunStatusCreated {
		t.Fatalf("expected created status, got %q", created.Status)
	}

	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", started.Status)
	}

	paused := postWorkflowRunAction(t, server, created.ID, "pause", http.StatusOK)
	if paused.Status != guidedworkflows.WorkflowRunStatusPaused {
		t.Fatalf("expected paused after pause, got %q", paused.Status)
	}

	resumed := postWorkflowRunAction(t, server, created.ID, "resume", http.StatusOK)
	if resumed.Status != guidedworkflows.WorkflowRunStatusRunning && resumed.Status != guidedworkflows.WorkflowRunStatusCompleted {
		t.Fatalf("expected running/completed after resume, got %q", resumed.Status)
	}

	fetched := getWorkflowRun(t, server, created.ID, http.StatusOK)
	if fetched.ID != created.ID {
		t.Fatalf("unexpected fetched run id: %q", fetched.ID)
	}

	timeline := getWorkflowRunTimeline(t, server, created.ID, http.StatusOK)
	if len(timeline) == 0 {
		t.Fatalf("expected non-empty timeline")
	}
	if timeline[0].Type != "run_created" {
		t.Fatalf("expected first timeline event run_created, got %q", timeline[0].Type)
	}

	runs := getWorkflowRuns(t, server, http.StatusOK)
	if len(runs) == 0 {
		t.Fatalf("expected workflow list to include created run")
	}
	if runs[0].ID != created.ID {
		t.Fatalf("expected most-recent run first in list, got %q", runs[0].ID)
	}
}

func TestWorkflowRunEndpointsDismissAndUndismiss(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	dismissed := postWorkflowRunAction(t, server, created.ID, "dismiss", http.StatusOK)
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set")
	}

	runs := getWorkflowRuns(t, server, http.StatusOK)
	if len(runs) != 0 {
		t.Fatalf("expected default workflow list to exclude dismissed runs, got %#v", runs)
	}

	included := getWorkflowRunsWithPath(t, server, "/v1/workflow-runs?include_dismissed=1", http.StatusOK)
	if len(included) != 1 || included[0].ID != created.ID {
		t.Fatalf("expected dismissed run in include_dismissed list, got %#v", included)
	}

	undismissed := postWorkflowRunAction(t, server, created.ID, "undismiss", http.StatusOK)
	if undismissed.DismissedAt != nil {
		t.Fatalf("expected dismissed_at to clear")
	}
}

func TestWorkflowRunEndpointsInvalidTransition(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	postWorkflowRunActionRaw(t, server, created.ID, "resume", http.StatusConflict)
}

func TestWorkflowRunEndpointsCreateWithPolicyOverrides(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	confidenceThreshold := 0.88
	pauseThreshold := 0.52
	preCommitHardGate := true
	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &guidedworkflows.CheckpointPolicyOverride{
			ConfidenceThreshold: &confidenceThreshold,
			PauseThreshold:      &pauseThreshold,
			HardGates: &guidedworkflows.CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if created.Policy.ConfidenceThreshold != 0.88 {
		t.Fatalf("unexpected confidence threshold override: %v", created.Policy.ConfidenceThreshold)
	}
	if created.Policy.PauseThreshold != 0.52 {
		t.Fatalf("unexpected pause threshold override: %v", created.Policy.PauseThreshold)
	}
	if !created.Policy.HardGates.PreCommitApproval {
		t.Fatalf("expected hard gate pre_commit_approval override")
	}
}

func TestWorkflowRunEndpointsDecisionActions(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", started.Status)
	}

	paused := postWorkflowRunDecision(t, server, created.ID, WorkflowRunDecisionRequest{
		Action: guidedworkflows.DecisionActionPauseRun,
		Note:   "pause for review",
	}, http.StatusOK)
	if paused.Status != guidedworkflows.WorkflowRunStatusPaused {
		t.Fatalf("expected paused after pause_run decision, got %q", paused.Status)
	}

	revised := postWorkflowRunDecision(t, server, created.ID, WorkflowRunDecisionRequest{
		Action: guidedworkflows.DecisionActionRequestRevision,
		Note:   "needs revision",
	}, http.StatusOK)
	if revised.Status != guidedworkflows.WorkflowRunStatusPaused {
		t.Fatalf("expected paused after request_revision decision, got %q", revised.Status)
	}

	continued := postWorkflowRunDecision(t, server, created.ID, WorkflowRunDecisionRequest{
		Action: guidedworkflows.DecisionActionApproveContinue,
		Note:   "continue",
	}, http.StatusOK)
	if continued.Status != guidedworkflows.WorkflowRunStatusRunning && continued.Status != guidedworkflows.WorkflowRunStatusCompleted {
		t.Fatalf("expected running/completed after approve_continue decision, got %q", continued.Status)
	}
}

func TestWorkflowRunEndpointsStartPublishesDecisionNeededNotificationWhenPolicyPauses(t *testing.T) {
	notifier := &recordNotificationPublisher{}
	api := &API{
		Version: "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}, guidedworkflows.WithTemplate(guidedworkflows.WorkflowTemplate{
			ID:   "single_commit",
			Name: "Single Commit",
			Phases: []guidedworkflows.WorkflowTemplatePhase{
				{
					ID:   "phase",
					Name: "phase",
					Steps: []guidedworkflows.WorkflowTemplateStep{
						{ID: "commit", Name: "commit"},
					},
				},
			},
		})),
		Notifier: notifier,
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	preCommitHardGate := true
	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:  "single_commit",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &guidedworkflows.CheckpointPolicyOverride{
			HardGates: &guidedworkflows.CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusPaused {
		t.Fatalf("expected paused on start, got %q", started.Status)
	}
	if countDecisionNeededEvents(notifier.events) != 1 {
		t.Fatalf("expected one decision-needed notification, got %d", countDecisionNeededEvents(notifier.events))
	}
	event := lastDecisionNeededEvent(notifier.events)
	if event == nil {
		t.Fatalf("expected decision-needed event payload")
	}
	if asString(event.Payload["recommended_action"]) == "" {
		t.Fatalf("expected recommended_action in notification payload")
	}
}

func TestWorkflowRunEndpointsDisabled(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	body, _ := json.Marshal(CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 500, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

func TestWorkflowRunEndpointsMaxActiveRunsGuardrail(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}, guidedworkflows.WithMaxActiveRuns(1)),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	body, _ := json.Marshal(CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409 conflict when max active runs exceeded, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

func TestWorkflowRunMetricsEndpoint(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running start status, got %q", started.Status)
	}

	metrics := getWorkflowRunMetrics(t, server, http.StatusOK)
	if !metrics.Enabled {
		t.Fatalf("expected telemetry enabled")
	}
	if metrics.RunsStarted < 1 {
		t.Fatalf("expected runs_started >= 1, got %d", metrics.RunsStarted)
	}
	if metrics.PauseRate < 0 {
		t.Fatalf("expected non-negative pause rate, got %f", metrics.PauseRate)
	}
}

func TestWorkflowRunMetricsResetEndpoint(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	before := getWorkflowRunMetrics(t, server, http.StatusOK)
	if before.RunsStarted < 1 {
		t.Fatalf("expected pre-reset runs_started >= 1, got %d", before.RunsStarted)
	}
	reset := postWorkflowRunMetricsReset(t, server, http.StatusOK)
	if reset.RunsStarted != 0 || reset.RunsCompleted != 0 || reset.RunsFailed != 0 || reset.PauseCount != 0 || reset.ApprovalCount != 0 {
		t.Fatalf("expected reset metrics to be zeroed, got %#v", reset)
	}
	after := getWorkflowRunMetrics(t, server, http.StatusOK)
	if after.RunsStarted != 0 || after.PauseCount != 0 || after.ApprovalCount != 0 {
		t.Fatalf("expected metrics endpoint to return reset values, got %#v", after)
	}
}

func TestToGuidedWorkflowServiceErrorMappings(t *testing.T) {
	if err := toGuidedWorkflowServiceError(nil); err != nil {
		t.Fatalf("expected nil")
	}
	check := func(err error, want ServiceErrorKind) {
		t.Helper()
		mapped := toGuidedWorkflowServiceError(err)
		serviceErr, ok := mapped.(*ServiceError)
		if !ok {
			t.Fatalf("expected *ServiceError, got %T", mapped)
		}
		if serviceErr.Kind != want {
			t.Fatalf("unexpected error kind: got=%s want=%s", serviceErr.Kind, want)
		}
	}
	check(guidedworkflows.ErrRunNotFound, ServiceErrorNotFound)
	check(guidedworkflows.ErrTemplateNotFound, ServiceErrorInvalid)
	check(guidedworkflows.ErrMissingContext, ServiceErrorInvalid)
	check(guidedworkflows.ErrInvalidTransition, ServiceErrorConflict)
	check(guidedworkflows.ErrRunLimitExceeded, ServiceErrorConflict)
	check(guidedworkflows.ErrDisabled, ServiceErrorUnavailable)
	check(guidedworkflows.ErrStepDispatch, ServiceErrorUnavailable)

	mapped := toGuidedWorkflowServiceError(fmt.Errorf("%w: turn already in progress", guidedworkflows.ErrStepDispatch))
	serviceErr, ok := mapped.(*ServiceError)
	if !ok {
		t.Fatalf("expected *ServiceError for turn conflict, got %T", mapped)
	}
	if serviceErr.Kind != ServiceErrorConflict {
		t.Fatalf("expected conflict kind for turn-in-progress dispatch error, got %s", serviceErr.Kind)
	}
}

func newWorkflowRunTestServer(t *testing.T, api *API) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workflow-runs", api.WorkflowRunsEndpoint)
	mux.HandleFunc("/v1/workflow-runs/metrics", api.WorkflowRunMetricsEndpoint)
	mux.HandleFunc("/v1/workflow-runs/metrics/reset", api.WorkflowRunMetricsResetEndpoint)
	mux.HandleFunc("/v1/workflow-runs/", api.WorkflowRunByID)
	mux.HandleFunc("/health", api.Health)
	return httptest.NewServer(TokenAuthMiddleware("token", mux))
}

func createWorkflowRunViaAPI(t *testing.T, server *httptest.Server, reqBody CreateWorkflowRunRequest) *guidedworkflows.WorkflowRun {
	t.Helper()
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var run guidedworkflows.WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode create run: %v", err)
	}
	return &run
}

func postWorkflowRunAction(t *testing.T, server *httptest.Server, runID, action string, wantStatus int) *guidedworkflows.WorkflowRun {
	t.Helper()
	resp := postWorkflowRunActionRaw(t, server, runID, action, wantStatus)
	defer resp.Body.Close()
	var run guidedworkflows.WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode action run response: %v", err)
	}
	return &run
}

func postWorkflowRunActionRaw(t *testing.T, server *httptest.Server, runID, action string, wantStatus int) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs/"+runID+"/"+action, nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("workflow action request: %v", err)
	}
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("unexpected status for %s: got=%d want=%d payload=%s", action, resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	return resp
}

func postWorkflowRunDecision(t *testing.T, server *httptest.Server, runID string, decision WorkflowRunDecisionRequest, wantStatus int) *guidedworkflows.WorkflowRun {
	t.Helper()
	body, _ := json.Marshal(decision)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs/"+runID+"/decision", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("workflow decision request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status for decision: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var run guidedworkflows.WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode decision response: %v", err)
	}
	return &run
}

func getWorkflowRun(t *testing.T, server *httptest.Server, runID string, wantStatus int) *guidedworkflows.WorkflowRun {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workflow-runs/"+runID, nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get workflow run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected get status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var run guidedworkflows.WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode workflow run: %v", err)
	}
	return &run
}

func getWorkflowRunTimeline(t *testing.T, server *httptest.Server, runID string, wantStatus int) []guidedworkflows.RunTimelineEvent {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workflow-runs/"+runID+"/timeline", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get timeline request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected timeline status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var payload struct {
		Timeline []guidedworkflows.RunTimelineEvent `json:"timeline"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode timeline payload: %v", err)
	}
	return payload.Timeline
}

func getWorkflowRuns(t *testing.T, server *httptest.Server, wantStatus int) []*guidedworkflows.WorkflowRun {
	return getWorkflowRunsWithPath(t, server, "/v1/workflow-runs", wantStatus)
}

func getWorkflowRunsWithPath(t *testing.T, server *httptest.Server, path string, wantStatus int) []*guidedworkflows.WorkflowRun {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get workflow runs request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected list status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var payload struct {
		Runs []*guidedworkflows.WorkflowRun `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode workflow runs: %v", err)
	}
	return payload.Runs
}

func getWorkflowRunMetrics(t *testing.T, server *httptest.Server, wantStatus int) guidedworkflows.RunMetricsSnapshot {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workflow-runs/metrics", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get metrics request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected metrics status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var metrics guidedworkflows.RunMetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode metrics payload: %v", err)
	}
	return metrics
}

func postWorkflowRunMetricsReset(t *testing.T, server *httptest.Server, wantStatus int) guidedworkflows.RunMetricsSnapshot {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs/metrics/reset", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post metrics reset request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected metrics reset status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var metrics guidedworkflows.RunMetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode metrics reset payload: %v", err)
	}
	return metrics
}

func TestWorkflowRunServiceInterfaceCompatibility(t *testing.T) {
	var _ GuidedWorkflowRunService = guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
}
