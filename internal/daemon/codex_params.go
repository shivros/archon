package daemon

import (
	"os"
	"strconv"
	"strings"
)

const (
	codexApprovalPolicyEnv = "ARCHON_CODEX_APPROVAL_POLICY"
	codexSandboxPolicyEnv  = "ARCHON_CODEX_SANDBOX_POLICY"
	codexNetworkAccessEnv  = "ARCHON_CODEX_NETWORK_ACCESS"
)

func codexTurnOptionsFromEnv() map[string]any {
	opts := map[string]any{}
	if policy := strings.TrimSpace(os.Getenv(codexApprovalPolicyEnv)); policy != "" {
		opts["approvalPolicy"] = policy
	}
	if sandbox := strings.TrimSpace(os.Getenv(codexSandboxPolicyEnv)); sandbox != "" {
		policy := map[string]any{"type": codexSandboxTurnType(sandbox)}
		if raw := strings.TrimSpace(os.Getenv(codexNetworkAccessEnv)); raw != "" {
			if val, err := strconv.ParseBool(raw); err == nil {
				policy["networkAccess"] = val
			}
		}
		opts["sandboxPolicy"] = policy
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

func codexThreadOptionsFromEnv() map[string]any {
	opts := map[string]any{}
	if policy := strings.TrimSpace(os.Getenv(codexApprovalPolicyEnv)); policy != "" {
		opts["approvalPolicy"] = policy
	}
	if sandbox := strings.TrimSpace(os.Getenv(codexSandboxPolicyEnv)); sandbox != "" {
		opts["sandbox"] = codexSandboxThreadType(sandbox)
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

func codexSandboxTurnType(raw string) string {
	switch strings.TrimSpace(raw) {
	case "workspace-write":
		return "workspaceWrite"
	case "read-only":
		return "readOnly"
	case "danger-full-access":
		return "dangerFullAccess"
	case "external-sandbox":
		return "externalSandbox"
	default:
		return raw
	}
}

func codexSandboxThreadType(raw string) string {
	switch strings.TrimSpace(raw) {
	case "workspaceWrite":
		return "workspace-write"
	case "readOnly":
		return "read-only"
	case "dangerFullAccess":
		return "danger-full-access"
	case "externalSandbox":
		return "external-sandbox"
	default:
		return raw
	}
}
