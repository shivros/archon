package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestAPIWorkflowDispatchDefaultsAccessor(t *testing.T) {
	var nilAPI *API
	if got := nilAPI.workflowDispatchDefaults(); got.Provider != "" || got.Model != "" || got.Access != "" || got.Reasoning != "" {
		t.Fatalf("expected zero-value defaults for nil API, got %+v", got)
	}

	api := &API{}
	if got := api.workflowDispatchDefaults(); got.Provider != "" || got.Model != "" || got.Access != "" || got.Reasoning != "" {
		t.Fatalf("expected zero-value defaults for empty API, got %+v", got)
	}

	api.WorkflowDispatchDefaults = guidedWorkflowDispatchDefaults{
		Provider:  "opencode",
		Model:     "gpt-5.3-codex",
		Access:    types.AccessReadOnly,
		Reasoning: types.ReasoningHigh,
	}
	got := api.workflowDispatchDefaults()
	if got.Provider != "opencode" {
		t.Fatalf("expected configured provider, got %q", got.Provider)
	}
	if got.Model != "gpt-5.3-codex" {
		t.Fatalf("expected configured model, got %q", got.Model)
	}
	if got.Access != types.AccessReadOnly {
		t.Fatalf("expected configured access, got %q", got.Access)
	}
	if got.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected configured reasoning, got %q", got.Reasoning)
	}
}

type stubWorkflowRunMetricsService struct{}

func (stubWorkflowRunMetricsService) GetRunMetrics(context.Context) (guidedworkflows.RunMetricsSnapshot, error) {
	return guidedworkflows.RunMetricsSnapshot{Enabled: true}, nil
}

type stubWorkflowRunMetricsResetService struct{}

func (stubWorkflowRunMetricsResetService) ResetRunMetrics(context.Context) (guidedworkflows.RunMetricsSnapshot, error) {
	return guidedworkflows.RunMetricsSnapshot{Enabled: true}, nil
}

type stubWorkflowSessionVisibilityService struct{}

func (stubWorkflowSessionVisibilityService) SyncWorkflowRunSessionVisibility(*guidedworkflows.WorkflowRun, bool) {
}

type stubWorkflowSessionInterruptService struct{}

func (stubWorkflowSessionInterruptService) InterruptWorkflowRunSessions(context.Context, *guidedworkflows.WorkflowRun) error {
	return nil
}

type stubWorkflowRunStopCoordinator struct{}

func (stubWorkflowRunStopCoordinator) StopWorkflowRun(context.Context, string) (*guidedworkflows.WorkflowRun, error) {
	return &guidedworkflows.WorkflowRun{ID: "gwf-1"}, nil
}

type stubWorkflowPolicyResolver struct {
	calls int
}

func (r *stubWorkflowPolicyResolver) ResolvePolicyOverrides(explicit *guidedworkflows.CheckpointPolicyOverride) *guidedworkflows.CheckpointPolicyOverride {
	r.calls++
	return explicit
}

func TestAPIServiceAccessors(t *testing.T) {
	var nilAPI *API
	if nilAPI.workflowRunService() != nil {
		t.Fatalf("expected nil run service for nil API")
	}
	if nilAPI.workflowRunMetricsService() != nil {
		t.Fatalf("expected nil run metrics service for nil API")
	}
	if nilAPI.workflowRunMetricsResetService() != nil {
		t.Fatalf("expected nil run metrics reset service for nil API")
	}
	if nilAPI.workflowSessionVisibilityService() != nil {
		t.Fatalf("expected nil session visibility service for nil API")
	}
	if nilAPI.workflowSessionInterruptService() != nil {
		t.Fatalf("expected nil session interrupt service for nil API")
	}
	if nilAPI.workflowRunStopCoordinator() != nil {
		t.Fatalf("expected nil run stop coordinator for nil API")
	}

	metricsService := stubWorkflowRunMetricsService{}
	resetService := stubWorkflowRunMetricsResetService{}
	visibilityService := stubWorkflowSessionVisibilityService{}
	interruptService := stubWorkflowSessionInterruptService{}
	stopCoordinator := stubWorkflowRunStopCoordinator{}
	api := &API{
		WorkflowRuns:              guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		WorkflowRunMetrics:        metricsService,
		WorkflowRunMetricsReset:   resetService,
		WorkflowSessionVisibility: visibilityService,
		WorkflowSessionInterrupt:  interruptService,
		WorkflowRunStop:           stopCoordinator,
	}
	if api.workflowRunService() == nil {
		t.Fatalf("expected configured run service")
	}
	if api.workflowRunMetricsService() != metricsService {
		t.Fatalf("expected configured run metrics service")
	}
	if api.workflowRunMetricsResetService() != resetService {
		t.Fatalf("expected configured run metrics reset service")
	}
	if api.workflowSessionVisibilityService() != visibilityService {
		t.Fatalf("expected configured session visibility service")
	}
	if api.workflowSessionInterruptService() != interruptService {
		t.Fatalf("expected configured session interrupt service")
	}
	if api.workflowRunStopCoordinator() != stopCoordinator {
		t.Fatalf("expected configured run stop coordinator")
	}

	api.WorkflowRunStop = nil
	if api.workflowRunStopCoordinator() == nil {
		t.Fatalf("expected fallback run stop coordinator when run service is configured")
	}
}

func TestAPIWorkflowTemplateAndPolicyResolverAccessors(t *testing.T) {
	var nilAPI *API
	if nilAPI.workflowTemplateService() != nil {
		t.Fatalf("expected nil workflow template service for nil API")
	}
	if got := nilAPI.workflowPolicyResolver(); got == nil {
		t.Fatalf("expected default policy resolver for nil API")
	}

	api := &API{}
	if api.workflowTemplateService() != nil {
		t.Fatalf("expected nil workflow template service when none configured")
	}
	api.WorkflowRuns = guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	if api.workflowTemplateService() == nil {
		t.Fatalf("expected workflow template service fallback from run service")
	}

	custom := &stubWorkflowPolicyResolver{}
	api.WorkflowPolicy = custom
	if got := api.workflowPolicyResolver(); got == nil {
		t.Fatalf("expected configured policy resolver")
	} else {
		_ = got.ResolvePolicyOverrides(nil)
	}
	if custom.calls != 1 {
		t.Fatalf("expected configured policy resolver to be used, got %d calls", custom.calls)
	}
}

func TestAPIBoolAndLineParsers(t *testing.T) {
	if parseLines("") != 200 {
		t.Fatalf("expected parseLines empty default")
	}
	if parseLines("invalid") != 200 {
		t.Fatalf("expected parseLines invalid default")
	}
	if parseLines("0") != 200 {
		t.Fatalf("expected parseLines non-positive default")
	}
	if parseLines("42") != 42 {
		t.Fatalf("expected parseLines to parse valid values")
	}

	if !parseBoolQueryValue("1") || !parseBoolQueryValue(" true ") || !parseBoolQueryValue("YES") {
		t.Fatalf("expected true-like query values to parse as true")
	}
	if parseBoolQueryValue("0") || parseBoolQueryValue("no") {
		t.Fatalf("expected false-like query values to parse as false")
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions?follow=1&refresh=yes", nil)
	if !isFollowRequest(req) {
		t.Fatalf("expected follow query helper to parse true")
	}
	if !isRefreshRequest(req) {
		t.Fatalf("expected refresh query helper to parse true")
	}
}
