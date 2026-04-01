package guidedworkflows

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"control/internal/types"
)

func TestStepPromptDispatchRequestIsStepOnly(t *testing.T) {
	reqType := reflect.TypeOf(StepPromptDispatchRequest{})
	if _, ok := reqType.FieldByName("StepID"); !ok {
		t.Fatal("expected step dispatch request to include StepID")
	}
	if _, ok := reqType.FieldByName("GateID"); ok {
		t.Fatal("expected step dispatch request to exclude GateID")
	}
	if _, ok := reqType.FieldByName("GateKind"); ok {
		t.Fatal("expected step dispatch request to exclude GateKind")
	}
	if _, ok := reqType.FieldByName("Boundary"); ok {
		t.Fatal("expected step dispatch request to exclude Boundary")
	}
}

func TestGateDispatchRequestIsGateOnly(t *testing.T) {
	reqType := reflect.TypeOf(GateDispatchRequest{})
	if _, ok := reqType.FieldByName("GateID"); !ok {
		t.Fatal("expected gate dispatch request to include GateID")
	}
	if _, ok := reqType.FieldByName("GateKind"); !ok {
		t.Fatal("expected gate dispatch request to include GateKind")
	}
	if _, ok := reqType.FieldByName("Boundary"); !ok {
		t.Fatal("expected gate dispatch request to include Boundary")
	}
	if _, ok := reqType.FieldByName("StepID"); ok {
		t.Fatal("expected gate dispatch request to exclude StepID")
	}
}

func TestStepPromptDispatchRequestJSONOmitsGateFields(t *testing.T) {
	payload, err := json.Marshal(StepPromptDispatchRequest{
		RunID:       "run-1",
		PhaseID:     "phase-1",
		StepID:      "step-1",
		Prompt:      "do the step",
		SessionID:   "sess-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model: "gpt-5.3-codex",
		},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	jsonText := string(payload)
	if strings.Contains(jsonText, "gate_id") || strings.Contains(jsonText, "gate_kind") || strings.Contains(jsonText, "boundary") {
		t.Fatalf("expected step dispatch JSON to exclude gate fields, got %s", jsonText)
	}
	if !strings.Contains(jsonText, `"step_id":"step-1"`) {
		t.Fatalf("expected step dispatch JSON to include step_id, got %s", jsonText)
	}
}

func TestGateDispatchRequestJSONOmitsStepFields(t *testing.T) {
	payload, err := json.Marshal(GateDispatchRequest{
		RunID:       "run-1",
		PhaseID:     "phase-1",
		GateID:      "gate-1",
		GateKind:    WorkflowGateKindLLMJudge,
		Boundary:    WorkflowGateBoundaryPhaseEnd,
		Prompt:      "judge the phase",
		SessionID:   "sess-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	jsonText := string(payload)
	if strings.Contains(jsonText, "step_id") {
		t.Fatalf("expected gate dispatch JSON to exclude step_id, got %s", jsonText)
	}
	if !strings.Contains(jsonText, `"gate_id":"gate-1"`) || !strings.Contains(jsonText, `"gate_kind":"llm_judge"`) || !strings.Contains(jsonText, `"boundary":"phase_end"`) {
		t.Fatalf("expected gate dispatch JSON to include gate fields, got %s", jsonText)
	}
}
