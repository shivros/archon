package store

import (
	"context"
	"path/filepath"
	"testing"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestWorkflowTemplateStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow_templates.json")
	store := NewFileWorkflowTemplateStore(path)

	templates, err := store.ListWorkflowTemplates(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowTemplates: %v", err)
	}
	if len(templates) != 0 {
		t.Fatalf("expected empty templates, got %d", len(templates))
	}

	template := guidedworkflows.WorkflowTemplate{
		ID:                 "custom_workflow",
		Name:               "Custom Workflow",
		Description:        "Test template",
		DefaultAccessLevel: types.AccessOnRequest,
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{
						ID:     "plan",
						Name:   "Plan",
						Prompt: "Write a plan",
						RuntimeOptions: &types.SessionRuntimeOptions{
							Model:     "gpt-5.3-codex",
							Reasoning: types.ReasoningHigh,
						},
					},
				},
			},
		},
	}
	saved, err := store.UpsertWorkflowTemplate(ctx, template)
	if err != nil {
		t.Fatalf("UpsertWorkflowTemplate: %v", err)
	}
	if saved == nil || saved.ID != template.ID {
		t.Fatalf("unexpected saved template: %#v", saved)
	}

	loaded, ok, err := store.GetWorkflowTemplate(ctx, template.ID)
	if err != nil {
		t.Fatalf("GetWorkflowTemplate: %v", err)
	}
	if !ok || loaded == nil {
		t.Fatalf("expected template to exist")
	}
	if loaded.Phases[0].Steps[0].Prompt != "Write a plan" {
		t.Fatalf("expected prompt to round-trip, got %q", loaded.Phases[0].Steps[0].Prompt)
	}
	if loaded.Phases[0].Steps[0].RuntimeOptions == nil {
		t.Fatalf("expected step runtime options to round-trip")
	}
	if loaded.Phases[0].Steps[0].RuntimeOptions.Model != "gpt-5.3-codex" {
		t.Fatalf("expected runtime model to round-trip, got %q", loaded.Phases[0].Steps[0].RuntimeOptions.Model)
	}
	if loaded.Phases[0].Steps[0].RuntimeOptions.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected runtime reasoning to round-trip, got %q", loaded.Phases[0].Steps[0].RuntimeOptions.Reasoning)
	}
	if loaded.DefaultAccessLevel != types.AccessOnRequest {
		t.Fatalf("expected default_access_level to round-trip, got %q", loaded.DefaultAccessLevel)
	}

	templates, err = store.ListWorkflowTemplates(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowTemplates reload: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected one template, got %d", len(templates))
	}

	if err := store.DeleteWorkflowTemplate(ctx, template.ID); err != nil {
		t.Fatalf("DeleteWorkflowTemplate: %v", err)
	}
	_, ok, err = store.GetWorkflowTemplate(ctx, template.ID)
	if err != nil {
		t.Fatalf("GetWorkflowTemplate post-delete: %v", err)
	}
	if ok {
		t.Fatalf("expected template deleted")
	}
}

func TestWorkflowTemplateStoreValidatesRequiredFields(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow_templates.json")
	store := NewFileWorkflowTemplateStore(path)

	_, err := store.UpsertWorkflowTemplate(ctx, guidedworkflows.WorkflowTemplate{
		ID:   "invalid",
		Name: "Invalid",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{
						ID:   "step_1",
						Name: "Step 1",
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected validation error for missing step prompt")
	}
}

func TestWorkflowTemplateStoreValidatesDefaultAccessLevel(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow_templates.json")
	store := NewFileWorkflowTemplateStore(path)

	_, err := store.UpsertWorkflowTemplate(ctx, guidedworkflows.WorkflowTemplate{
		ID:                 "invalid_access",
		Name:               "Invalid Access",
		DefaultAccessLevel: "invalid",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{
						ID:     "step_1",
						Name:   "Step 1",
						Prompt: "hello",
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected validation error for invalid default_access_level")
	}
}

func TestWorkflowTemplateStoreValidatesStepRuntimeAccess(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow_templates.json")
	store := NewFileWorkflowTemplateStore(path)

	_, err := store.UpsertWorkflowTemplate(ctx, guidedworkflows.WorkflowTemplate{
		ID:   "invalid_step_access",
		Name: "Invalid Step Access",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{
						ID:     "step_1",
						Name:   "Step 1",
						Prompt: "hello",
						RuntimeOptions: &types.SessionRuntimeOptions{
							Access: "invalid_access",
						},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected validation error for invalid step runtime_options.access")
	}
}

func TestWorkflowTemplateStoreValidatesStepRuntimeReasoning(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow_templates.json")
	store := NewFileWorkflowTemplateStore(path)

	_, err := store.UpsertWorkflowTemplate(ctx, guidedworkflows.WorkflowTemplate{
		ID:   "invalid_step_reasoning",
		Name: "Invalid Step Reasoning",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{
						ID:     "step_1",
						Name:   "Step 1",
						Prompt: "hello",
						RuntimeOptions: &types.SessionRuntimeOptions{
							Reasoning: "ultra",
						},
					},
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected validation error for invalid step runtime_options.reasoning")
	}
}
