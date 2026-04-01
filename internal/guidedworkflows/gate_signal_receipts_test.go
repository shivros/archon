package guidedworkflows

import "testing"

func TestBuildGateSignalReceiptRefForSignal(t *testing.T) {
	tests := []struct {
		name    string
		runID   string
		signal  GateSignal
		wantOK  bool
		wantKey string
	}{
		{
			name:    "explicit transport",
			runID:   "run-1",
			signal:  GateSignal{Transport: "webhook", SignalID: "sig-1"},
			wantOK:  true,
			wantKey: "run-1|webhook|sig-1",
		},
		{
			name:    "blank transport uses session turn compatibility default",
			runID:   "run-1",
			signal:  GateSignal{SignalID: "sig-1"},
			wantOK:  true,
			wantKey: "run-1|session_turn|sig-1",
		},
		{
			name:    "normalizes whitespace and case",
			runID:   " run-1 ",
			signal:  GateSignal{Transport: " Session_Turn ", SignalID: " sig-1 "},
			wantOK:  true,
			wantKey: "run-1|session_turn|sig-1",
		},
		{
			name:   "missing signal id",
			runID:  "run-1",
			signal: GateSignal{Transport: "webhook"},
			wantOK: false,
		},
		{
			name:   "missing run id",
			runID:  " ",
			signal: GateSignal{Transport: "webhook", SignalID: "sig-1"},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref, ok := buildGateSignalReceiptRefForSignal(tc.runID, tc.signal)
			if ok != tc.wantOK {
				t.Fatalf("expected ok=%v, got %v", tc.wantOK, ok)
			}
			if !tc.wantOK {
				return
			}
			if got := ref.Key(); got != tc.wantKey {
				t.Fatalf("expected key %q, got %q", tc.wantKey, got)
			}
		})
	}
}

func TestBuildGateSignalReceiptRefForGate(t *testing.T) {
	tests := []struct {
		name    string
		runID   string
		gate    WorkflowGateRun
		wantOK  bool
		wantKey string
	}{
		{
			name:  "prefers last signal transport",
			runID: "run-1",
			gate: WorkflowGateRun{
				SignalID: "sig-1",
				LastSignal: &GateSignalContext{
					Transport: "webhook",
					SignalID:  "sig-1",
				},
				Execution: &GateExecutionRef{
					Transport: "session_turn",
					SignalID:  "sig-1",
				},
			},
			wantOK:  true,
			wantKey: "run-1|webhook|sig-1",
		},
		{
			name:  "falls back to execution transport",
			runID: "run-1",
			gate: WorkflowGateRun{
				Execution: &GateExecutionRef{
					Transport: "script",
					SignalID:  "sig-2",
				},
			},
			wantOK:  true,
			wantKey: "run-1|script|sig-2",
		},
		{
			name:  "falls back to latest execution attempt",
			runID: "run-1",
			gate: WorkflowGateRun{
				SignalID: "sig-3",
				ExecutionAttempts: []GateExecutionRef{
					{Transport: "session_turn", SignalID: "sig-3"},
					{Transport: "webhook", SignalID: "sig-3"},
				},
			},
			wantOK:  true,
			wantKey: "run-1|webhook|sig-3",
		},
		{
			name:  "uses compatibility transport default for legacy gate",
			runID: "run-1",
			gate: WorkflowGateRun{
				SignalID: "sig-4",
			},
			wantOK:  true,
			wantKey: "run-1|session_turn|sig-4",
		},
		{
			name:   "missing signal id",
			runID:  "run-1",
			gate:   WorkflowGateRun{},
			wantOK: false,
		},
		{
			name:  "missing run id",
			runID: " ",
			gate: WorkflowGateRun{
				SignalID: "sig-5",
			},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref, ok := buildGateSignalReceiptRefForGate(tc.runID, tc.gate)
			if ok != tc.wantOK {
				t.Fatalf("expected ok=%v, got %v", tc.wantOK, ok)
			}
			if !tc.wantOK {
				return
			}
			if got := ref.Key(); got != tc.wantKey {
				t.Fatalf("expected key %q, got %q", tc.wantKey, got)
			}
		})
	}
}

func TestGateSignalReceiptRefKeyReturnsEmptyWhenIncomplete(t *testing.T) {
	tests := []struct {
		name string
		ref  gateSignalReceiptRef
	}{
		{
			name: "missing run id",
			ref: gateSignalReceiptRef{
				Transport: "webhook",
				SignalID:  "sig-1",
			},
		},
		{
			name: "missing signal id",
			ref: gateSignalReceiptRef{
				RunID:     "run-1",
				Transport: "webhook",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ref.Key(); got != "" {
				t.Fatalf("expected empty key, got %q", got)
			}
		})
	}
}
