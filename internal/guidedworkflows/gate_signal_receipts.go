package guidedworkflows

import "strings"

type gateSignalReceiptRef struct {
	RunID     string
	Transport string
	SignalID  string
}

func buildGateSignalReceiptRefForSignal(runID string, signal GateSignal) (gateSignalReceiptRef, bool) {
	ref := gateSignalReceiptRef{
		RunID:     strings.TrimSpace(runID),
		Transport: normalizeGateReceiptTransportLegacyCompatible(signal.Transport),
		SignalID:  strings.TrimSpace(signal.SignalID),
	}
	if ref.RunID == "" || ref.SignalID == "" {
		return gateSignalReceiptRef{}, false
	}
	return ref, true
}

func buildGateSignalReceiptRefForGate(runID string, gate WorkflowGateRun) (gateSignalReceiptRef, bool) {
	signalID := strings.TrimSpace(gate.SignalID)
	if signalID == "" && gate.LastSignal != nil {
		signalID = strings.TrimSpace(gate.LastSignal.SignalID)
	}
	if signalID == "" && gate.Execution != nil {
		signalID = strings.TrimSpace(gate.Execution.SignalID)
	}
	if signalID == "" {
		return gateSignalReceiptRef{}, false
	}
	ref := gateSignalReceiptRef{
		RunID:     strings.TrimSpace(runID),
		Transport: normalizeGateReceiptTransportLegacyCompatible(gateReceiptTransport(gate)),
		SignalID:  signalID,
	}
	if ref.RunID == "" {
		return gateSignalReceiptRef{}, false
	}
	return ref, true
}

func (ref gateSignalReceiptRef) Key() string {
	if ref.RunID == "" || ref.SignalID == "" {
		return ""
	}
	return strings.Join([]string{
		ref.RunID,
		normalizeGateReceiptTransportLegacyCompatible(ref.Transport),
		ref.SignalID,
	}, "|")
}

func normalizeGateReceiptTransportLegacyCompatible(transport string) string {
	transport = strings.ToLower(strings.TrimSpace(transport))
	if transport == "" {
		return "session_turn"
	}
	return transport
}

func gateReceiptTransport(gate WorkflowGateRun) string {
	if gate.LastSignal != nil {
		if transport := strings.TrimSpace(gate.LastSignal.Transport); transport != "" {
			return transport
		}
	}
	if gate.Execution != nil {
		if transport := strings.TrimSpace(gate.Execution.Transport); transport != "" {
			return transport
		}
	}
	for i := len(gate.ExecutionAttempts) - 1; i >= 0; i-- {
		if transport := strings.TrimSpace(gate.ExecutionAttempts[i].Transport); transport != "" {
			return transport
		}
	}
	return ""
}
