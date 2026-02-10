package daemon

import (
	"os"
	"strconv"
	"strings"

	"control/internal/types"
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

func codexTurnOptions(runtimeOptions *types.SessionRuntimeOptions) map[string]any {
	return mergeOptionMaps(codexTurnOptionsFromEnv(), codexTurnOptionsFromRuntime(runtimeOptions))
}

func codexThreadOptions(runtimeOptions *types.SessionRuntimeOptions) map[string]any {
	return mergeOptionMaps(codexThreadOptionsFromEnv(), codexThreadOptionsFromRuntime(runtimeOptions))
}

func codexTurnOptionsFromRuntime(runtimeOptions *types.SessionRuntimeOptions) map[string]any {
	if runtimeOptions == nil {
		return nil
	}
	opts := map[string]any{}
	if policy, sandbox := codexAccessToTurnPolicies(runtimeOptions.Access); policy != "" || sandbox != "" {
		if policy != "" {
			opts["approvalPolicy"] = policy
		}
		if sandbox != "" {
			policy := map[string]any{"type": codexSandboxTurnType(sandbox)}
			opts["sandboxPolicy"] = policy
		}
	}
	if effort := codexReasoningEffort(runtimeOptions.Reasoning); effort != "" {
		opts["reasoningEffort"] = effort
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

func codexThreadOptionsFromRuntime(runtimeOptions *types.SessionRuntimeOptions) map[string]any {
	if runtimeOptions == nil {
		return nil
	}
	opts := map[string]any{}
	if policy, sandbox := codexAccessToThreadPolicies(runtimeOptions.Access); policy != "" || sandbox != "" {
		if policy != "" {
			opts["approvalPolicy"] = policy
		}
		if sandbox != "" {
			opts["sandbox"] = codexSandboxThreadType(sandbox)
		}
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

func codexReasoningEffort(level types.ReasoningLevel) string {
	switch level {
	case types.ReasoningLow:
		return "low"
	case types.ReasoningMedium:
		return "medium"
	case types.ReasoningHigh:
		return "high"
	case types.ReasoningExtraHigh:
		// Codex currently supports low/medium/high; map extra-high to high.
		return "high"
	default:
		return ""
	}
}

func codexAccessToTurnPolicies(level types.AccessLevel) (approvalPolicy string, sandbox string) {
	switch level {
	case types.AccessReadOnly:
		return "on-request", "read-only"
	case types.AccessOnRequest:
		return "on-request", "workspace-write"
	case types.AccessFull:
		return "never", "danger-full-access"
	default:
		return "", ""
	}
}

func codexAccessToThreadPolicies(level types.AccessLevel) (approvalPolicy string, sandbox string) {
	switch level {
	case types.AccessReadOnly:
		return "on-request", "read-only"
	case types.AccessOnRequest:
		return "on-request", "workspace-write"
	case types.AccessFull:
		return "never", "danger-full-access"
	default:
		return "", ""
	}
}

func mergeOptionMaps(base, overrides map[string]any) map[string]any {
	if len(base) == 0 && len(overrides) == 0 {
		return nil
	}
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

func shouldRetryWithoutModel(err error) bool {
	if err == nil {
		return false
	}
	raw := strings.ToLower(strings.TrimSpace(err.Error()))
	if raw == "" {
		return false
	}
	if strings.Contains(raw, "invalid params") {
		return true
	}
	if strings.Contains(raw, "unknown") && strings.Contains(raw, "model") {
		return true
	}
	if strings.Contains(raw, "unsupported") && strings.Contains(raw, "model") {
		return true
	}
	if strings.Contains(raw, "unrecognized") && strings.Contains(raw, "model") {
		return true
	}
	return false
}
