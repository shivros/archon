package guidedworkflows

import "testing"

func TestTurnStatusClassifiers(t *testing.T) {
	if !IsTerminalTurnStatus("completed") {
		t.Fatalf("expected completed to be terminal")
	}
	if !IsTerminalTurnStatus("failed") {
		t.Fatalf("expected failed to be terminal")
	}
	if IsTerminalTurnStatus("in_progress") {
		t.Fatalf("expected in_progress to be non-terminal")
	}
	if !IsFailedTurnStatus("error") {
		t.Fatalf("expected error to be failed")
	}
	if IsFailedTurnStatus("completed") {
		t.Fatalf("expected completed to be non-failed")
	}
}

func TestTurnSignalFailureDetail(t *testing.T) {
	if msg, failed := TurnSignalFailureDetail(TurnSignal{Error: "unsupported model"}); !failed || msg != "unsupported model" {
		t.Fatalf("expected explicit error to fail, got failed=%v msg=%q", failed, msg)
	}
	if msg, failed := TurnSignalFailureDetail(TurnSignal{Terminal: true, Status: "failed"}); !failed || msg == "" {
		t.Fatalf("expected failed terminal status to fail, got failed=%v msg=%q", failed, msg)
	}
	if msg, failed := TurnSignalFailureDetail(TurnSignal{Terminal: true, Status: "completed"}); failed || msg != "" {
		t.Fatalf("expected completed terminal status to remain non-failed, got failed=%v msg=%q", failed, msg)
	}
}
