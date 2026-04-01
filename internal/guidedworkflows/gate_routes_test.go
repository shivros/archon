package guidedworkflows

import (
	"context"
	"testing"
)

func TestCloneWorkflowGateSpecPreservesRoutes(t *testing.T) {
	original := WorkflowGateSpec{
		ID:   "gate_1",
		Kind: WorkflowGateKindManualReview,
		Boundary: WorkflowGateBoundaryRef{
			Boundary: WorkflowGateBoundaryPhaseEnd,
			PhaseID:  "phase_1",
		},
		Routes: []WorkflowGateRoute{
			{ID: "continue", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetNextStep}},
			{ID: "retry", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetStep, StepID: "step_1"}},
		},
		ManualReviewConfig: &ManualReviewConfig{Reason: "sign off"},
	}

	cloned := cloneWorkflowGateSpec(original)
	cloned.Routes[0].ID = "changed"
	cloned.ManualReviewConfig.Reason = "changed"

	if original.Routes[0].ID != "continue" {
		t.Fatalf("expected original routes to remain unchanged, got %#v", original.Routes)
	}
	if original.ManualReviewConfig.Reason != "sign off" {
		t.Fatalf("expected original config to remain unchanged, got %#v", original.ManualReviewConfig)
	}
}

func TestCloneWorkflowRunPreservesGateRoutes(t *testing.T) {
	run := &WorkflowRun{
		ID: "run-1",
		Phases: []PhaseRun{
			{
				ID: "phase_1",
				Gates: []WorkflowGateRun{
					{
						ID:     "gate_1",
						Kind:   WorkflowGateKindLLMJudge,
						Status: WorkflowGateStatusPending,
						Routes: []WorkflowGateRoute{
							{ID: "continue", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetNextStep}},
						},
						LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
		},
	}

	cloned := cloneWorkflowRun(run)
	cloned.Phases[0].Gates[0].Routes[0].ID = "changed"

	if run.Phases[0].Gates[0].Routes[0].ID != "continue" {
		t.Fatalf("expected original run gate routes to remain unchanged, got %#v", run.Phases[0].Gates[0].Routes)
	}
}

func TestCreateRunMaterializesGateRoutes(t *testing.T) {
	template := WorkflowTemplate{
		ID:   "gate_routes_run",
		Name: "Gate Routes Run",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "hello"},
					{ID: "step_2", Name: "Step 2", Prompt: "world"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindManualReview,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						Routes: []WorkflowGateRoute{
							{ID: "continue", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetNextStep}},
							{ID: "retry", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetStep, StepID: "step_1"}},
						},
						ManualReviewConfig: &ManualReviewConfig{Reason: "sign off"},
					},
				},
			},
		},
	}

	service := NewRunService(Config{Enabled: true}, WithTemplate(template))
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  template.ID,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if len(run.Phases) != 1 || len(run.Phases[0].Gates) != 1 {
		t.Fatalf("expected one phase gate, got %#v", run.Phases)
	}
	if len(run.Phases[0].Gates[0].Routes) != 2 {
		t.Fatalf("expected gate routes to materialize onto run, got %#v", run.Phases[0].Gates[0].Routes)
	}
	if run.Phases[0].Gates[0].Routes[1].Target.StepID != "step_1" {
		t.Fatalf("expected named-step route target to round-trip, got %#v", run.Phases[0].Gates[0].Routes[1])
	}
}
