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

func TestParseWorkflowTemplateCatalogJSONSupportsPhaseGatePromptRefs(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"prompts": {
				"judge_phase": "Decide whether the phase succeeded."
			}
		},
		"templates": [{
			"id": "judge_ref",
			"name": "Judge Ref",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{
					"id": "s1",
					"name": "Step",
					"prompt": "hello"
				}],
				"gates": [{
					"id": "g1",
					"kind": "llm_judge",
					"boundary": "phase_end",
					"prompt_ref": "judge_phase"
				}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	gates := parsed.Templates[0].Phases[0].Gates
	if len(gates) != 1 || gates[0].LLMJudgeConfig == nil || gates[0].LLMJudgeConfig.Prompt != "Decide whether the phase succeeded." {
		t.Fatalf("unexpected parsed phase gates: %#v", gates)
	}
}

func TestParseWorkflowTemplateCatalogJSONSupportsManualAndLLMPhaseGates(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "gated",
			"name": "Gated",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{
					"id": "s1",
					"name": "Step",
					"prompt": "hello"
				}],
				"gates": [{
					"id": "manual_1",
					"kind": "manual_review",
					"boundary": "phase_end",
					"reason": "human sign-off required"
				}, {
					"id": "judge_1",
					"kind": "llm_judge",
					"boundary": "phase_end",
					"prompt": "Judge this phase"
				}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	gates := parsed.Templates[0].Phases[0].Gates
	if len(gates) != 2 {
		t.Fatalf("expected two gates, got %d", len(gates))
	}
	if gates[0].Kind != WorkflowGateKindManualReview || gates[1].Kind != WorkflowGateKindLLMJudge {
		t.Fatalf("unexpected gate kinds: %#v", gates)
	}
	if gates[0].ManualReviewConfig == nil || gates[0].ManualReviewConfig.Reason != "human sign-off required" {
		t.Fatalf("expected manual review config to resolve reason, got %#v", gates[0])
	}
	if gates[1].LLMJudgeConfig == nil || gates[1].LLMJudgeConfig.Prompt != "Judge this phase" {
		t.Fatalf("expected llm judge config to resolve prompt, got %#v", gates[1])
	}
}

func TestParseWorkflowTemplateCatalogJSONRejectsInvalidGateKindsAndConfig(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad_kind",
			"name": "Bad Kind",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [{"id":"g1","kind":"webhook","boundary":"phase_end"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "is not supported") {
		t.Fatalf("expected unsupported gate kind error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad_manual",
			"name": "Bad Manual",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [{"id":"g1","kind":"manual_review","prompt":"nope"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "manual_review has unknown field(s): prompt") {
		t.Fatalf("expected manual_review config error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad_judge_reason",
			"name": "Bad Judge Reason",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [{"id":"g1","kind":"llm_judge","prompt":"judge","reason":"nope"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "llm_judge has unknown field(s): reason") {
		t.Fatalf("expected llm_judge reason config error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad_manual_unknown",
			"name": "Bad Manual Unknown",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [{"id":"g1","kind":"manual_review","unexpected":"value"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "manual_review has unknown field(s): unexpected") {
		t.Fatalf("expected manual_review unknown-field error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONRejectsMissingGateKindAndUnsupportedBoundary(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "missing_kind",
			"name": "Missing Kind",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [{"id":"g1","boundary":"phase_end"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), ".kind is required") {
		t.Fatalf("expected missing kind error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad_boundary",
			"name": "Bad Boundary",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [{"id":"g1","kind":"manual_review","boundary":"step_end"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), ".boundary") {
		t.Fatalf("expected unsupported boundary error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONGateDefaultsAutoIDAndBoundary(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "gate_defaults",
			"name": "Gate Defaults",
			"phases": [{
				"id": "phase_alpha",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [{
					"kind": "manual_review",
					"reason": "check this"
				}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	gates := parsed.Templates[0].Phases[0].Gates
	if len(gates) != 1 {
		t.Fatalf("expected one gate, got %d", len(gates))
	}
	if gates[0].ID != "phase_alpha_gate_1" {
		t.Fatalf("expected generated gate id phase_alpha_gate_1, got %q", gates[0].ID)
	}
	if gates[0].Boundary.Boundary != WorkflowGateBoundaryPhaseEnd || gates[0].Boundary.PhaseID != "phase_alpha" {
		t.Fatalf("expected default phase_end boundary with phase id, got %#v", gates[0].Boundary)
	}
}

func TestParseWorkflowTemplateCatalogJSONRejectsDuplicateGateIDAndMissingJudgePrompt(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "duplicate_gate_ids",
			"name": "Duplicate Gate IDs",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [
					{"id":"same","kind":"manual_review"},
					{"id":"same","kind":"llm_judge","prompt":"judge"}
				]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate gate id") {
		t.Fatalf("expected duplicate gate id error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "judge_prompt_required",
			"name": "Judge Prompt Required",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [{"id":"judge_1","kind":"llm_judge"}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "llm_judge resolved prompt is required") {
		t.Fatalf("expected llm_judge prompt required error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONSupportsGateRoutes(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "route_template",
			"name": "Route Template",
			"phases": [{
				"id": "p1",
				"name": "Phase 1",
				"steps": [
					{"id": "s1", "name": "Step 1", "prompt": "one"},
					{"id": "s2", "name": "Step 2", "prompt": "two"}
				],
				"gates": [{
					"id": "manual_1",
					"kind": "manual_review",
					"routes": [
						{"id": "continue", "target": {"kind": "next_step"}},
						{"id": "retry", "target": {"kind": "step", "step_id": "s1"}},
						{"id": "finish", "target": {"kind": "complete_phase"}}
					]
				}, {
					"id": "judge_1",
					"kind": "llm_judge",
					"prompt": "Judge this phase",
					"routes": [
						{"id": "accept", "target": {"kind": "next_step"}}
					]
				}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	gates := parsed.Templates[0].Phases[0].Gates
	if len(gates) != 2 {
		t.Fatalf("expected two gates, got %d", len(gates))
	}
	if len(gates[0].Routes) != 3 {
		t.Fatalf("expected three manual_review routes, got %#v", gates[0].Routes)
	}
	if gates[0].Routes[1].Target.Kind != WorkflowGateRouteTargetStep || gates[0].Routes[1].Target.StepID != "s1" {
		t.Fatalf("unexpected named-step route target: %#v", gates[0].Routes[1])
	}
	if len(gates[1].Routes) != 1 || gates[1].Routes[0].Target.Kind != WorkflowGateRouteTargetNextStep {
		t.Fatalf("unexpected llm_judge routes: %#v", gates[1].Routes)
	}
}

func TestParseWorkflowTemplateCatalogJSONRejectsMalformedGateRoutes(t *testing.T) {
	_, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "dup_route_ids",
			"name": "Duplicate Route IDs",
			"phases": [{
				"id": "p1",
				"name": "Phase 1",
				"steps": [{"id": "s1", "name": "Step 1", "prompt": "hello"}],
				"gates": [{
					"id": "g1",
					"kind": "manual_review",
					"routes": [
						{"id": "same", "target": {"kind": "next_step"}},
						{"id": "same", "target": {"kind": "complete_phase"}}
					]
				}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "duplicate route id") {
		t.Fatalf("expected duplicate route id error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "missing_step_id",
			"name": "Missing Step ID",
			"phases": [{
				"id": "p1",
				"name": "Phase 1",
				"steps": [{"id": "s1", "name": "Step 1", "prompt": "hello"}],
				"gates": [{
					"id": "g1",
					"kind": "manual_review",
					"routes": [{"id": "retry", "target": {"kind": "step"}}]
				}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "target.step_id is required") {
		t.Fatalf("expected missing step_id error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad_route_target",
			"name": "Bad Route Target",
			"phases": [{
				"id": "p1",
				"name": "Phase 1",
				"steps": [{"id": "s1", "name": "Step 1", "prompt": "hello"}],
				"gates": [{
					"id": "g1",
					"kind": "manual_review",
					"routes": [{"id": "retry", "target": {"kind": "next_step", "step_id": "s1"}}]
				}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "does not accept step_id") {
		t.Fatalf("expected invalid next_step route error, got %v", err)
	}

	_, err = ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "unknown_route_step",
			"name": "Unknown Route Step",
			"phases": [{
				"id": "p1",
				"name": "Phase 1",
				"steps": [{"id": "s1", "name": "Step 1", "prompt": "hello"}],
				"gates": [{
					"id": "g1",
					"kind": "manual_review",
					"routes": [{"id": "retry", "target": {"kind": "step", "step_id": "missing"}}]
				}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "does not match any template step") {
		t.Fatalf("expected unknown route target step error, got %v", err)
	}
}

func TestParseWorkflowTemplateCatalogJSONGateRoutesAreOptional(t *testing.T) {
	parsed, err := ParseWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "route_less",
			"name": "Route Less",
			"phases": [{
				"id": "p1",
				"name": "Phase",
				"steps": [{"id": "s1", "name": "Step", "prompt": "hello"}],
				"gates": [{"id":"g1","kind":"manual_review","reason":"sign off"}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("ParseWorkflowTemplateCatalogJSON: %v", err)
	}
	gates := parsed.Templates[0].Phases[0].Gates
	if len(gates) != 1 {
		t.Fatalf("expected one gate, got %d", len(gates))
	}
	if len(gates[0].Routes) != 0 {
		t.Fatalf("expected route-less gate to preserve empty routes, got %#v", gates[0].Routes)
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
	if err == nil || !strings.Contains(err.Error(), "cannot combine phase_template_ref with steps/step_refs/gates") {
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
	if err == nil || !strings.Contains(err.Error(), "cannot combine phase_template_ref with steps/step_refs/gates") {
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
