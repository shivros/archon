package guidedworkflows

import (
	"strings"
	"testing"
)

func TestDecodeWorkflowTemplateCatalogJSONSupportsTypedGateEncoding(t *testing.T) {
	parsed, err := DecodeWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "typed_gate_catalog",
			"name": "Typed Gate Catalog",
			"phases": [{
				"id": "p1",
				"name": "Phase 1",
				"steps": [{"id": "s1", "name": "Step 1", "prompt": "hello"}],
				"gates": [{
					"id": "g1",
					"kind": "manual_review",
					"boundary": {
						"boundary": "phase_end",
						"phase_id": "p1"
					},
					"routes": [{"id": "continue", "target": {"kind": "next_step"}}]
				}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("DecodeWorkflowTemplateCatalogJSON: %v", err)
	}
	if len(parsed.Templates) != 1 {
		t.Fatalf("expected one template, got %d", len(parsed.Templates))
	}
	gates := parsed.Templates[0].Phases[0].Gates
	if len(gates) != 1 {
		t.Fatalf("expected one gate, got %#v", gates)
	}
	if gates[0].Boundary.Boundary != WorkflowGateBoundaryPhaseEnd || gates[0].Boundary.PhaseID != "p1" {
		t.Fatalf("expected typed boundary object to round-trip, got %#v", gates[0].Boundary)
	}
	if len(gates[0].Routes) != 1 || gates[0].Routes[0].Target.Kind != WorkflowGateRouteTargetNextStep {
		t.Fatalf("expected routes to decode from typed encoding, got %#v", gates[0].Routes)
	}
}

func TestDecodeWorkflowTemplateCatalogJSONSupportsTypedConfigDetection(t *testing.T) {
	parsed, err := DecodeWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "typed_config_catalog",
			"name": "Typed Config Catalog",
			"phases": [{
				"id": "p1",
				"name": "Phase 1",
				"steps": [{"id": "s1", "name": "Step 1", "prompt": "hello"}],
				"gates": [{
					"id": "g1",
					"kind": "llm_judge",
					"boundary": {
						"boundary": "phase_end",
						"phase_id": "p1"
					},
					"llm_judge_config": {
						"prompt": "Judge this phase"
					}
				}]
			}]
		}]
	}`))
	if err != nil {
		t.Fatalf("DecodeWorkflowTemplateCatalogJSON: %v", err)
	}
	if len(parsed.Templates) != 1 || len(parsed.Templates[0].Phases[0].Gates) != 1 {
		t.Fatalf("expected typed config catalog to decode, got %#v", parsed.Templates)
	}
	if parsed.Templates[0].Phases[0].Gates[0].LLMJudgeConfig == nil || parsed.Templates[0].Phases[0].Gates[0].LLMJudgeConfig.Prompt != "Judge this phase" {
		t.Fatalf("expected llm_judge_config prompt to round-trip, got %#v", parsed.Templates[0].Phases[0].Gates[0].LLMJudgeConfig)
	}
}

func TestDecodeWorkflowTemplateCatalogJSONDoesNotFallbackWhenDefinitionsExist(t *testing.T) {
	_, err := DecodeWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"definitions": {
			"prompts": {"judge": "Decide"}
		},
		"templates": [{
			"id": "mixed_catalog",
			"name": "Mixed Catalog",
			"phases": [{
				"id": "p1",
				"name": "Phase 1",
				"steps": [{"id": "s1", "name": "Step 1", "prompt": "hello"}],
				"gates": [{
					"id": "g1",
					"kind": "llm_judge",
					"llm_judge_config": {"prompt": "Judge this phase"}
				}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "llm_judge has unknown field(s): llm_judge_config") {
		t.Fatalf("expected original composition error when definitions exist, got %v", err)
	}
}

func TestDecodeWorkflowTemplateCatalogJSONReturnsParseErrorWhenCatalogIsNotTyped(t *testing.T) {
	_, err := DecodeWorkflowTemplateCatalogJSON([]byte(`{"version":1,"templates":[`))
	if err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func TestDecodeWorkflowTemplateCatalogJSONReturnsTypedNormalizationError(t *testing.T) {
	_, err := DecodeWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "typed_invalid",
			"name": "Typed Invalid",
			"phases": [{
				"id": "p1",
				"name": "Phase 1",
				"steps": [{"id": "s1", "name": "Step 1", "prompt": "hello"}],
				"gates": [{
					"id": "g1",
					"kind": "manual_review",
					"boundary": {
						"boundary": "phase_end",
						"phase_id": "other_phase"
					}
				}]
			}]
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "boundary.phase_id must match containing phase") {
		t.Fatalf("expected typed normalization error, got %v", err)
	}
}

func TestDecodeWorkflowTemplateCatalogJSONPreservesCompositionErrors(t *testing.T) {
	_, err := DecodeWorkflowTemplateCatalogJSON([]byte(`{
		"version": 1,
		"templates": [{
			"id": "bad_composition",
			"name": "Bad Composition",
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
		t.Fatalf("expected composition validation error, got %v", err)
	}
}
