package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func TestWorkflowTemplateStoreExpandsCompositionRefs(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow_templates.json")
	store := NewFileWorkflowTemplateStore(path)
	raw := `{
		"version": 1,
		"definitions": {
			"prompts": {"quality": "run tests"},
			"steps": {
				"quality_checks": {"id": "quality_checks", "name": "quality checks", "prompt_ref": "quality"}
			},
			"phase_templates": {
				"delivery": {
					"id": "phase_delivery",
					"name": "Delivery",
					"step_refs": ["quality_checks"]
				}
			}
		},
		"templates": [{
			"id": "composed",
			"name": "Composed",
			"phases": [{"phase_template_ref": "delivery"}]
		}]
	}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	templates, err := store.ListWorkflowTemplates(ctx)
	if err != nil {
		t.Fatalf("ListWorkflowTemplates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected one template, got %d", len(templates))
	}
	if got := templates[0].Phases[0].Steps[0].Prompt; got != "run tests" {
		t.Fatalf("expected expanded prompt, got %q", got)
	}
}

func TestWorkflowTemplateStoreInvalidCompositionFailsFast(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow_templates.json")
	store := NewFileWorkflowTemplateStore(path)
	raw := `{
		"version": 1,
		"templates": [{
			"id": "bad",
			"name": "Bad",
			"phases": [{"id":"p1","name":"P1","step_refs":["missing"]}]
		}]
	}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := store.ListWorkflowTemplates(ctx)
	if err == nil || !strings.Contains(err.Error(), "unknown step_ref") {
		t.Fatalf("expected unknown step_ref error, got %v", err)
	}
}

func TestWorkflowTemplateStoreRejectsDuplicatePhaseIDsOnUpsert(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow_templates.json")
	store := NewFileWorkflowTemplateStore(path)

	_, err := store.UpsertWorkflowTemplate(ctx, guidedworkflows.WorkflowTemplate{
		ID:   "dup_phase_template",
		Name: "Duplicate Phase Template",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "hello"},
				},
			},
			{
				ID:   "phase_1",
				Name: "Phase 1 Duplicate",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{ID: "step_2", Name: "Step 2", Prompt: "world"},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate phase id") {
		t.Fatalf("expected duplicate phase id error, got %v", err)
	}
}

func TestWorkflowTemplateStoreRejectsDuplicateStepIDsWithinPhaseOnUpsert(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workflow_templates.json")
	store := NewFileWorkflowTemplateStore(path)

	_, err := store.UpsertWorkflowTemplate(ctx, guidedworkflows.WorkflowTemplate{
		ID:   "dup_step_phase_template",
		Name: "Duplicate Step Template",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "hello"},
					{ID: "step_1", Name: "Step 1 Dup", Prompt: "world"},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate step id in phase") {
		t.Fatalf("expected duplicate step id in phase error, got %v", err)
	}
}
