package daemon

import (
	"strings"

	"control/internal/guidedworkflows"
)

type turnOutcome struct {
	Status   string
	Error    string
	Terminal bool
	Failed   bool
}

func classifyTurnOutcome(status, errMsg string) turnOutcome {
	out := turnOutcome{
		Status: strings.TrimSpace(status),
		Error:  strings.TrimSpace(errMsg),
	}
	if out.Error != "" {
		out.Terminal = true
		out.Failed = true
		return out
	}
	if guidedworkflows.IsFailedTurnStatus(out.Status) {
		out.Terminal = true
		out.Failed = true
		return out
	}
	out.Terminal = guidedworkflows.IsTerminalTurnStatus(out.Status)
	return out
}
