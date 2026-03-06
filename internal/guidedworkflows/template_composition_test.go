package guidedworkflows

import (
	"strings"
	"testing"

	"control/internal/types"
)

func TestParseWorkflowTemplateCatalogJSONInlineOnlyStillWorks(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "inline",
			"name": "Inline",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{
					"id": "s1",
					"name": "Step",
					"prompt": "hello"
				}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	if len(parsed.Templates) != 1 {
		t.Fatalf("expected one template, got %d", len(parsed.Templates))
	}
	if got := parsed.Templates[0].Phases[0].Steps[0].Prompt; got != "hello" {
		t.Fatalf("expected prompt hello, got %q", got)
	}
}

func TestParseWorkflowTemplateCatalogJSONSupportsPromptStepAndPhaseRefs(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"prompts": {
				"quality": "Run tests"
			},
			"steps": {
				"quality_checks": {
					"id": "quality_checks",
					"name": "quality checks",
					"prompt_ref": "quality"
				}
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
			"phases": [{
				"phase_template_ref": "delivery"
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	step := parsed.Templates[0].Phases[0].Steps[0]
	if step.ID != "quality_checks" || step.Prompt != "Run tests" {
		t.Fatalf("unexpected expanded step: %#v", step)
	}
}

func TestParseWorkflowTemplateCatalogJSONLocalStepOverrideWorks(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"prompts": {
				"base": "base prompt",
				"override": "override prompt"
			},
			"steps": {
				"implementation": {
					"id": "implementation",
					"name": "implementation",
					"prompt_ref": "base",
					"runtime_options": {
						"model": "gpt-5.2-codex",
						"reasoning": "high"
					}
				}
			}
		},
		"templates": [{
			"id": "override_case",
			"name": "Override Case",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{
					"step_ref": "implementation",
					"name": "implementation override",
					"prompt_ref": "override",
					"runtime_options": {
						"model": "gpt-5.3-codex",
						"reasoning": "extra_high"
					}
				}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	step := parsed.Templates[0].Phases[0].Steps[0]
	if step.Name != "implementation override" {
		t.Fatalf("expected name override, got %q", step.Name)
	}
	if step.Prompt != "override prompt" {
		t.Fatalf("expected prompt override, got %q", step.Prompt)
	}
	if step.RuntimeOptions == nil || step.RuntimeOptions.Model != "gpt-5.3-codex" || step.RuntimeOptions.Reasoning != types.ReasoningExtraHigh {
		t.Fatalf("expected runtime options override, got %#v", step.RuntimeOptions)
	}
}

func TestParseWorkflowTemplateCatalogJSONPreservesExpandedStepOrder(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"steps": {
				"a": {"id": "a", "name": "A", "prompt": "A prompt"},
				"b": {"id": "b", "name": "B", "prompt": "B prompt"}
			}
		},
		"templates": [{
			"id": "ordered",
			"name": "Ordered",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"step_refs": ["b", "a"],
				"steps": [{"id": "c", "name": "C", "prompt": "C prompt"}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	steps := parsed.Templates[0].Phases[0].Steps
	got := []string{steps[0].ID, steps[1].ID, steps[2].ID}
	want := []string{"b", "a", "c"}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("unexpected step order: got=%v want=%v", got, want)
		}
	}
}

func TestParseWorkflowTemplateCatalogJSONMixedInlineAndReferencedConfig(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"steps": {
				"quality_checks": {"id": "quality_checks", "name": "quality checks", "prompt": "run tests"}
			}
		},
		"templates": [{
			"id": "mixed",
			"name": "Mixed",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"step_refs": ["quality_checks"],
				"steps": [{"id": "commit", "name": "commit", "prompt": "commit changes"}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	if len(parsed.Templates[0].Phases[0].Steps) != 2 {
		t.Fatalf("expected two steps, got %d", len(parsed.Templates[0].Phases[0].Steps))
	}
}

func TestParseWorkflowTemplateCatalogJSONUnknownRefsFail(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad",
			"name": "Bad",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt_ref": "missing"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "unknown prompt_ref") {
		t.Fatalf("expected unknown prompt_ref error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad2",
			"name": "Bad2",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"step_refs": ["missing_step"]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "unknown step_ref") {
		t.Fatalf("expected unknown step_ref error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad3",
			"name": "Bad3",
			"phases": [{
				"phase_template_ref": "missing_phase"
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "unknown phase_template_ref") {
		t.Fatalf("expected unknown phase_template_ref error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONRejectsAmbiguousPromptDefinition(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {"prompts": {"p": "prompt"}},
		"templates": [{
			"id": "bad",
			"name": "Bad",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "inline", "prompt_ref": "p"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "both prompt and prompt_ref") {
		t.Fatalf("expected ambiguous prompt error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONDefinitionsStepRequiresExplicitID(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"steps": {
				"implementation": {
					"name": "implementation",
					"prompt": "do it"
				}
			}
		},
		"templates": [{
			"id": "t1",
			"name": "T1",
			"phases": [{"id":"p1","name":"P1","step_refs":["implementation"]}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "definitions.steps[implementation].id is required") {
		t.Fatalf("expected missing step id error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONRejectsPhaseTemplateRefWithInlineSteps(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"phase_templates": {
				"delivery": {
					"id": "phase_delivery",
					"name": "Delivery",
					"steps": [{"id":"s1","name":"S1","prompt":"do it"}]
				}
			}
		},
		"templates": [{
			"id": "bad",
			"name": "Bad",
			"phases": [{
				"phase_template_ref": "delivery",
				"steps": [{"id":"s2","name":"S2","prompt":"also do it"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "cannot combine phase_template_ref with steps/step_refs") {
		t.Fatalf("expected phase_template_ref combine error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONRejectsPhaseTemplateRefWithStepRefs(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"steps": {
				"implementation": {"id":"implementation","name":"implementation","prompt":"do it"}
			},
			"phase_templates": {
				"delivery": {
					"id": "phase_delivery",
					"name": "Delivery",
					"step_refs": ["implementation"]
				}
			}
		},
		"templates": [{
			"id": "bad",
			"name": "Bad",
			"phases": [{
				"phase_template_ref": "delivery",
				"step_refs": ["implementation"]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "cannot combine phase_template_ref with steps/step_refs") {
		t.Fatalf("expected phase_template_ref combine error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONRejectsStepRefWithConflictingIDOverride(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"steps": {
				"implementation": {"id":"implementation","name":"implementation","prompt":"do it"}
			}
		},
		"templates": [{
			"id": "bad",
			"name": "Bad",
			"phases": [{
				"id":"p1",
				"name":"P1",
				"steps": [{
					"step_ref": "implementation",
					"id": "other_step_id"
				}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "cannot override step id when using step_ref") {
		t.Fatalf("expected step_ref id override error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONDefinitionsPhaseTemplateRequiresExplicitIDAndName(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"phase_templates": {
				"delivery": {
					"step_refs": []
				}
			}
		},
		"templates": [{
			"id":"t1",
			"name":"T1",
			"phases":[{"phase_template_ref":"delivery"}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "definitions.phase_templates[delivery].id is required") {
		t.Fatalf("expected missing phase template id error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"phase_templates": {
				"delivery": {
					"id": "phase_delivery",
					"step_refs": []
				}
			}
		},
		"templates": [{
			"id":"t1",
			"name":"T1",
			"phases":[{"phase_template_ref":"delivery"}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "definitions.phase_templates[delivery].name is required") {
		t.Fatalf("expected missing phase template name error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONDuplicateIDsFail(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "dup",
			"name": "Dup",
			"phases": [{"id": "p1", "name": "Phase", "steps": [{"id": "s1", "name": "A", "prompt": "a"}, {"id": "s1", "name": "B", "prompt": "b"}]}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate step id") {
		t.Fatalf("expected duplicate step id error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "dup",
			"name": "Dup",
			"phases": [
				{"id": "p1", "name": "A", "steps": [{"id": "s1", "name": "S1", "prompt": "a"}]},
				{"id": "p1", "name": "B", "steps": [{"id": "s2", "name": "S2", "prompt": "b"}]}
			]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate phase id") {
		t.Fatalf("expected duplicate phase id error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [
			{"id":"dup_template","name":"A","phases":[{"id":"p1","name":"P1","steps":[{"id":"s1","name":"S1","prompt":"a"}]}]},
			{"id":"dup_template","name":"B","phases":[{"id":"p2","name":"P2","steps":[{"id":"s2","name":"S2","prompt":"b"}]}]}
		]
	}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate template id") {
		t.Fatalf("expected duplicate template id error, got %v", err)
	}
}
