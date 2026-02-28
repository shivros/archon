package daemon

import (
	"strings"
	"time"
)

func parseClaudeRateLimitItem(payload map[string]any) (map[string]any, bool) {
	if payload == nil {
		return nil, false
	}
	info, _ := payload["rate_limit_info"].(map[string]any)
	if info == nil {
		return nil, false
	}
	status := strings.ToLower(strings.TrimSpace(asStringAny(info["status"])))
	if status == "" || status == "allowed" {
		return nil, false
	}
	item := map[string]any{
		"type":     "rateLimit",
		"provider": "claude",
		"status":   status,
	}
	if limitType := strings.TrimSpace(asStringAny(info["rateLimitType"])); limitType != "" {
		item["limit_type"] = limitType
	}
	if overage := strings.TrimSpace(asStringAny(info["overageStatus"])); overage != "" {
		item["overage_status"] = overage
	}
	if reason := strings.TrimSpace(asStringAny(info["overageDisabledReason"])); reason != "" {
		item["overage_disabled_reason"] = reason
	}
	if usingOverage, ok := info["isUsingOverage"].(bool); ok {
		item["is_using_overage"] = usingOverage
	}
	if retryAt := parsePersistedTimestamp(info["resetsAt"]); !retryAt.IsZero() {
		item["retry_unix"] = retryAt.UTC().Unix()
		item["retry_at"] = retryAt.UTC().Format(time.RFC3339Nano)
	}
	return item, true
}

func asStringAny(value any) string {
	return strings.TrimSpace(asString(value))
}
