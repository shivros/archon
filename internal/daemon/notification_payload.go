package daemon

import "strings"

func cloneNotificationPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		out[trimmed] = deepCloneNotificationPayloadValue(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func deepCloneNotificationPayloadValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneNotificationPayload(v)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = deepCloneNotificationPayloadValue(v[i])
		}
		return out
	default:
		return value
	}
}

func notificationPayloadBool(payload map[string]any, key string) bool {
	if payload == nil || strings.TrimSpace(key) == "" {
		return false
	}
	raw, ok := payload[key]
	if !ok {
		return false
	}
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		normalized := strings.ToLower(strings.TrimSpace(value))
		return normalized == "true" || normalized == "1" || normalized == "yes"
	default:
		return false
	}
}

func notificationPayloadInt(payload map[string]any, key string) int {
	if payload == nil || strings.TrimSpace(key) == "" {
		return 0
	}
	raw, ok := payload[key]
	if !ok {
		return 0
	}
	if parsed, ok := asInt(raw); ok {
		return parsed
	}
	return 0
}
