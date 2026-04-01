package guidedworkflows

import (
	"strings"
	"testing"
)

func TestNormalizeWorkflowTemplateRejectsConflictingTypedGateConfigs(t *testing.T) {
	_, err := NormalizeWorkflowTemplate(WorkflowTemplate{
		ID:   "bad_manual",
		Name: "Bad Manual",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "hello"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindManualReview,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						ManualReviewConfig: &ManualReviewConfig{Reason: "sign off"},
						LLMJudgeConfig:     &LLMJudgeConfig{Prompt: "should not be here"},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "manual_review cannot define llm_judge_config") {
		t.Fatalf("expected conflicting manual_review config error, got %v", err)
	}

	_, err = NormalizeWorkflowTemplate(WorkflowTemplate{
		ID:   "bad_judge",
		Name: "Bad Judge",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "hello"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindLLMJudge,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase_1",
						},
						ManualReviewConfig: &ManualReviewConfig{Reason: "should not be here"},
						LLMJudgeConfig:     &LLMJudgeConfig{Prompt: "judge"},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "llm_judge cannot define manual_review_config") {
		t.Fatalf("expected conflicting llm_judge config error, got %v", err)
	}
}

func TestNormalizeWorkflowTemplateRejectsTypedBoundaryPhaseMismatch(t *testing.T) {
	_, err := NormalizeWorkflowTemplate(WorkflowTemplate{
		ID:   "bad_boundary_phase",
		Name: "Bad Boundary Phase",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "hello"},
				},
				Gates: []WorkflowGateSpec{
					{
						ID:   "gate_1",
						Kind: WorkflowGateKindManualReview,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "other_phase",
						},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "boundary.phase_id must match containing phase") {
		t.Fatalf("expected phase mismatch error, got %v", err)
	}
}

func TestNormalizeWorkflowTemplateRejectsUnsupportedTypedRouteTargetKind(t *testing.T) {
	_, err := NormalizeWorkflowTemplate(WorkflowTemplate{
		ID:   "bad_route_kind",
		Name: "Bad Route Kind",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "hello"},
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
							{
								ID: "branch",
								Target: WorkflowGateRouteTargetRef{
									Kind: "teleport",
								},
							},
						},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "target kind") {
		t.Fatalf("expected unsupported route target kind error, got %v", err)
	}
}

func TestNormalizeWorkflowTemplateRejectsTypedRouteTargetStepMismatch(t *testing.T) {
	_, err := NormalizeWorkflowTemplate(WorkflowTemplate{
		ID:   "bad_route_step",
		Name: "Bad Route Step",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "hello"},
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
							{
								ID: "branch",
								Target: WorkflowGateRouteTargetRef{
									Kind:   WorkflowGateRouteTargetStep,
									StepID: "missing_step",
								},
							},
						},
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match any template step") {
		t.Fatalf("expected route target step mismatch error, got %v", err)
	}
}
