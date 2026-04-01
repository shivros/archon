package guidedworkflows

import "testing"

func TestStrictGateSignalMatcherUsesExecutionSignalFallback(t *testing.T) {
	matcher := strictGateSignalMatcher{}
	gate := &WorkflowGateRun{
		Execution: &GateExecutionRef{
			SignalID:  "sig-1",
			SessionID: "sess-1",
		},
	}

	if !matcher.Matches(gate, GateSignal{
		SignalID:  "sig-1",
		SessionID: "sess-1",
	}) {
		t.Fatal("expected matcher to use execution signal fallback when gate signal id is blank")
	}
}

func TestStrictGateSignalMatcherRejectsSessionMismatch(t *testing.T) {
	matcher := strictGateSignalMatcher{}
	gate := &WorkflowGateRun{
		SignalID: "sig-1",
		Execution: &GateExecutionRef{
			SessionID: "sess-1",
		},
	}

	if matcher.Matches(gate, GateSignal{
		SignalID:  "sig-1",
		SessionID: "sess-2",
	}) {
		t.Fatal("expected matcher to reject mismatched sessions when both sides provide them")
	}
}
