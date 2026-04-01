package guidedworkflows

import "testing"

func TestCloneWorkflowTemplateDeepCopiesNestedGateRoutes(t *testing.T) {
	original := WorkflowTemplate{
		ID:   "clone_template",
		Name: "Clone Template",
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
							{ID: "continue", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetNextStep}},
						},
						ManualReviewConfig: &ManualReviewConfig{Reason: "review"},
					},
				},
			},
		},
	}

	cloned := CloneWorkflowTemplate(original)
	cloned.Phases[0].Steps[0].Prompt = "changed"
	cloned.Phases[0].Gates[0].Routes[0].ID = "changed"
	cloned.Phases[0].Gates[0].ManualReviewConfig.Reason = "changed"

	if original.Phases[0].Steps[0].Prompt != "hello" {
		t.Fatalf("expected original step prompt unchanged, got %q", original.Phases[0].Steps[0].Prompt)
	}
	if original.Phases[0].Gates[0].Routes[0].ID != "continue" {
		t.Fatalf("expected original route unchanged, got %#v", original.Phases[0].Gates[0].Routes)
	}
	if original.Phases[0].Gates[0].ManualReviewConfig.Reason != "review" {
		t.Fatalf("expected original gate config unchanged, got %#v", original.Phases[0].Gates[0].ManualReviewConfig)
	}
}

func TestCloneWorkflowTemplatesDeepCopiesSliceMembers(t *testing.T) {
	original := []WorkflowTemplate{
		{
			ID:   "t1",
			Name: "Template 1",
			Phases: []WorkflowTemplatePhase{
				{
					ID:   "phase_1",
					Name: "Phase 1",
					Steps: []WorkflowTemplateStep{
						{ID: "step_1", Name: "Step 1", Prompt: "hello"},
					},
				},
			},
		},
	}

	cloned := CloneWorkflowTemplates(original)
	cloned[0].Name = "Changed"
	cloned[0].Phases[0].Steps[0].Prompt = "changed"

	if original[0].Name != "Template 1" {
		t.Fatalf("expected original template name unchanged, got %q", original[0].Name)
	}
	if original[0].Phases[0].Steps[0].Prompt != "hello" {
		t.Fatalf("expected original nested step unchanged, got %q", original[0].Phases[0].Steps[0].Prompt)
	}
}
