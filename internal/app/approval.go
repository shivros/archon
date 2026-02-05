package app

import (
	"encoding/json"
	"strings"

	"control/internal/types"
)

type ApprovalRequest struct {
	RequestID int
	Method    string
	Summary   string
	Detail    string
}

func approvalSummary(method string, params map[string]any) (string, string) {
	switch method {
	case "item/commandExecution/requestApproval":
		cmd := asString(params["parsedCmd"])
		if cmd == "" {
			cmd = asString(params["command"])
		}
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			return "command execution", ""
		}
		return "command", cmd
	case "item/fileChange/requestApproval":
		reason := strings.TrimSpace(asString(params["reason"]))
		if reason != "" {
			return "file change", reason
		}
		return "file change", ""
	case "tool/requestUserInput":
		if questions, ok := params["questions"].([]any); ok {
			for _, q := range questions {
				if qMap, ok := q.(map[string]any); ok {
					text := strings.TrimSpace(asString(qMap["text"]))
					if text != "" {
						return "user input", text
					}
				}
			}
		}
		return "user input", ""
	default:
	}
	return "approval", ""
}

func approvalFromRecord(record *types.Approval) *ApprovalRequest {
	if record == nil {
		return nil
	}
	params := map[string]any{}
	if len(record.Params) > 0 {
		_ = json.Unmarshal(record.Params, &params)
	}
	summary, detail := approvalSummary(record.Method, params)
	return &ApprovalRequest{
		RequestID: record.RequestID,
		Method:    record.Method,
		Summary:   summary,
		Detail:    detail,
	}
}
