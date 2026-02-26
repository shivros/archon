package guidedworkflows

import "strings"

func IsTerminalTurnStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "done", "failed", "error", "interrupted", "cancelled", "canceled", "rejected", "aborted", "stopped":
		return true
	default:
		return false
	}
}

func IsFailedTurnStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error", "interrupted", "cancelled", "canceled", "rejected", "aborted", "stopped":
		return true
	default:
		return false
	}
}

func TurnSignalFailureDetail(signal TurnSignal) (string, bool) {
	errMsg := strings.TrimSpace(signal.Error)
	if errMsg != "" {
		return errMsg, true
	}
	if !signal.Terminal {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(signal.Status))
	if !IsFailedTurnStatus(status) {
		return "", false
	}
	return "step failed: turn status " + status, true
}
