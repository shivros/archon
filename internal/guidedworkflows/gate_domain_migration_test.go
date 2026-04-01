package guidedworkflows

import (
	"reflect"
	"testing"
)

func TestWorkflowGateRunUsesGateNativeTransportTypes(t *testing.T) {
	gateType := reflect.TypeOf(WorkflowGateRun{})
	if _, ok := gateType.FieldByName("TurnID"); ok {
		t.Fatalf("expected WorkflowGateRun to no longer expose TurnID")
	}
	if _, ok := gateType.FieldByName("LastTurnSignal"); ok {
		t.Fatalf("expected WorkflowGateRun to no longer expose LastTurnSignal")
	}
	execField, ok := gateType.FieldByName("Execution")
	if !ok {
		t.Fatalf("expected WorkflowGateRun.Execution field")
	}
	if execField.Type.String() != "*guidedworkflows.GateExecutionRef" {
		t.Fatalf("expected gate execution ref type, got %s", execField.Type)
	}
}

func TestWorkflowGateSpecUsesKindSpecificConfig(t *testing.T) {
	specType := reflect.TypeOf(WorkflowGateSpec{})
	if _, ok := specType.FieldByName("Prompt"); ok {
		t.Fatalf("expected WorkflowGateSpec to no longer expose Prompt")
	}
	if _, ok := specType.FieldByName("Reason"); ok {
		t.Fatalf("expected WorkflowGateSpec to no longer expose Reason")
	}
	if _, ok := specType.FieldByName("ManualReviewConfig"); !ok {
		t.Fatalf("expected WorkflowGateSpec.ManualReviewConfig")
	}
	if _, ok := specType.FieldByName("LLMJudgeConfig"); !ok {
		t.Fatalf("expected WorkflowGateSpec.LLMJudgeConfig")
	}
	if _, ok := specType.FieldByName("Routes"); !ok {
		t.Fatalf("expected WorkflowGateSpec.Routes")
	}
}
