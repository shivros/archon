package daemon

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ClaudeParseState struct {
	SawDelta   bool
	SawMessage bool
}

func ParseClaudeLine(line string, state *ClaudeParseState) ([]map[string]any, string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, "", nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return nil, "", err
	}
	typ, _ := payload["type"].(string)
	items := make([]map[string]any, 0, 2)
	sessionID := ""

	switch typ {
	case "user":
		// Claude can emit internal user-role events (e.g. tool/result or thinking
		// traces) that are not authored by the human user. Human user messages are
		// already appended at send-time, so do not mirror plain user echoes here.
		reasoningID, reasoning := extractClaudeReasoning(payload["message"])
		if reasoning == "" {
			return nil, "", nil
		}
		reasoningItem := map[string]any{
			"type": "reasoning",
			"content": []map[string]any{
				{"type": "text", "text": reasoning},
			},
		}
		if reasoningID != "" {
			reasoningItem["id"] = reasoningID
		}
		items = append(items, reasoningItem)
	case "assistant":
		text := extractClaudeMessageText(payload["message"])
		if text == "" {
			if state != nil && state.SawDelta {
				items = append(items, map[string]any{
					"type": "agentMessageEnd",
				})
				state.SawDelta = false
				state.SawMessage = true
			}
			return items, "", nil
		}
		if state != nil && state.SawDelta {
			items = append(items, map[string]any{
				"type": "agentMessageEnd",
			})
			state.SawDelta = false
			state.SawMessage = true
			return items, "", nil
		}
		items = append(items, map[string]any{
			"type": "agentMessage",
			"text": text,
		})
		if state != nil {
			state.SawMessage = true
		}
	case "system":
		if subtype, _ := payload["subtype"].(string); subtype == "init" {
			if id, _ := payload["session_id"].(string); strings.TrimSpace(id) != "" {
				sessionID = id
			}
		}
		items = append(items, payload)
	case "stream_event":
		event, _ := payload["event"].(map[string]any)
		if event == nil {
			return nil, "", nil
		}
		if eventType, _ := event["type"].(string); eventType == "message_start" {
			if state != nil {
				state.SawDelta = false
				state.SawMessage = false
			}
			return nil, "", nil
		}
		delta, _ := event["delta"].(map[string]any)
		if delta == nil {
			return nil, "", nil
		}
		text, _ := delta["text"].(string)
		if strings.TrimSpace(text) == "" {
			return nil, "", nil
		}
		if state != nil {
			state.SawDelta = true
		}
		if item := newAgentMessageDeltaItem(text); item != nil {
			items = append(items, item)
		}
	case "result":
		if state != nil && state.SawDelta {
			items = append(items, map[string]any{
				"type": "agentMessageEnd",
			})
			state.SawDelta = false
			state.SawMessage = true
			return items, "", nil
		}
		if state != nil && state.SawMessage {
			if id, _ := payload["session_id"].(string); strings.TrimSpace(id) != "" {
				sessionID = id
			}
			return items, sessionID, nil
		}
		if result, _ := payload["result"].(string); strings.TrimSpace(result) != "" {
			items = append(items, map[string]any{
				"type": "agentMessage",
				"text": result,
			})
			if state != nil {
				state.SawMessage = true
			}
			return items, "", nil
		}
		if resultObj, _ := payload["result"].(map[string]any); resultObj != nil {
			if text, _ := resultObj["result"].(string); strings.TrimSpace(text) != "" {
				items = append(items, map[string]any{
					"type": "agentMessage",
					"text": text,
				})
				if id, _ := resultObj["session_id"].(string); strings.TrimSpace(id) != "" {
					sessionID = id
				}
				if state != nil {
					state.SawMessage = true
				}
				return items, sessionID, nil
			}
		}
		items = append(items, payload)
	case "rate_limit_event":
		if item, ok := parseClaudeRateLimitItem(payload); ok {
			items = append(items, item)
		}
	case "error":
		if item := parseClaudeErrorItem(payload); item != nil {
			items = append(items, item)
		}
	default:
		items = append(items, map[string]any{
			"type": "log",
			"text": line,
		})
	}

	return items, sessionID, nil
}

func extractClaudeMessageText(raw any) string {
	if raw == nil {
		return ""
	}
	payload, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	if text, ok := payload["content"].(string); ok {
		return strings.TrimSpace(text)
	}
	content, ok := payload["content"].([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, entry := range content {
		block, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if text, _ := block["text"].(string); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func extractClaudeReasoning(raw any) (id string, reasoning string) {
	payload, ok := raw.(map[string]any)
	if !ok || payload == nil {
		return "", ""
	}
	if messageID, _ := payload["id"].(string); strings.TrimSpace(messageID) != "" {
		id = strings.TrimSpace(messageID)
	}
	content, ok := payload["content"].([]any)
	if !ok || len(content) == 0 {
		return id, ""
	}
	var parts []string
	for _, entry := range content {
		block, ok := entry.(map[string]any)
		if !ok || block == nil {
			continue
		}
		blockType, _ := block["type"].(string)
		switch strings.ToLower(strings.TrimSpace(blockType)) {
		case "thinking", "reasoning", "redacted_thinking":
		default:
			continue
		}
		if text, _ := block["thinking"].(string); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
			continue
		}
		if text, _ := block["text"].(string); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
			continue
		}
	}
	return id, strings.TrimSpace(strings.Join(parts, "\n"))
}

// parseClaudeErrorItem handles {"type":"error",...} JSON events from Claude CLI.
// These are emitted for authentication failures, model errors, etc.
func parseClaudeErrorItem(payload map[string]any) map[string]any {
	if payload == nil {
		return nil
	}

	// Extract the error object — Claude Code wraps it as {"type":"error","error":{...}}
	var errObj map[string]any
	if raw, ok := payload["error"]; ok {
		errObj, _ = raw.(map[string]any)
	}

	errorType := ""
	errorMessage := ""
	if errObj != nil {
		errorType, _ = errObj["type"].(string)
		errorMessage, _ = errObj["message"].(string)
	}

	// Also check for top-level message field
	if strings.TrimSpace(errorMessage) == "" {
		if msg, ok := payload["message"].(string); ok {
			errorMessage = msg
		}
	}

	// Classify the error for user-facing presentation
	isAuthError := false
	userMessage := strings.TrimSpace(errorMessage)

	switch {
	case errorType == "authentication_error",
		strings.Contains(strings.ToLower(errorMessage), "invalid authentication"),
		strings.Contains(strings.ToLower(errorMessage), "api key"),
		strings.Contains(strings.ToLower(errorMessage), "unauthorized"):
		isAuthError = true
		userMessage = "Authentication failed. Please run: claude /login"
	}

	item := map[string]any{
		"type":          "providerError",
		"provider":      "claude",
		"error_type":    errorType,
		"error_message": userMessage,
		"raw_message":   errorMessage,
		"is_auth_error": isAuthError,
	}

	return item
}

// parseClaudeNonJSONErrorLine detects provider errors in non-JSON output lines.
// Claude Code CLI may emit plain-text error messages on stderr when it crashes
// before entering stream-json mode (e.g., auth errors, missing binary).
func parseClaudeNonJSONErrorLine(line string) map[string]any {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	lower := strings.ToLower(line)

	// Detect authentication errors
	isAuthError := false
	switch {
	case strings.Contains(lower, "api error: 401"),
		strings.Contains(lower, "authentication_error"),
		strings.Contains(lower, "invalid authentication"),
		strings.Contains(lower, "please run /login"),
		strings.Contains(lower, "please run: /login"),
		strings.Contains(lower, "api key"):
		isAuthError = true
	}

	// Detect other notable error patterns
	isAPIError := strings.Contains(lower, "api error:")
	isConnectionError := strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "network error") ||
		strings.Contains(lower, "timeout")

	if !isAuthError && !isAPIError && !isConnectionError {
		return nil
	}

	item := map[string]any{
		"type":     "providerError",
		"provider": "claude",
		"raw_line": line,
	}

	if isAuthError {
		item["error_type"] = "authentication_error"
		item["error_message"] = "Authentication failed. Please run: claude /login"
		item["is_auth_error"] = true
	} else if isAPIError {
		item["error_type"] = "api_error"
		item["error_message"] = fmt.Sprintf("API error: %s", extractShortError(line))
		item["is_auth_error"] = false
	} else {
		item["error_type"] = "connection_error"
		item["error_message"] = fmt.Sprintf("Connection error: %s", extractShortError(line))
		item["is_auth_error"] = false
	}

	return item
}

// extractShortError extracts a human-readable error summary from a potentially
// long error line that may contain embedded JSON.
func extractShortError(line string) string {
	// For "API Error: 401 {...} · Please run /login", extract the actionable part
	if idx := strings.Index(line, " · "); idx >= 0 {
		return strings.TrimSpace(line[idx+len(" · "):])
	}
	// For "API Error: NNN message", extract the HTTP status message
	if idx := strings.Index(line, "API Error: "); idx >= 0 {
		rest := line[idx+len("API Error: "):]
		// Remove embedded JSON if present
		if jsonStart := strings.Index(rest, "{"); jsonStart >= 0 {
			return strings.TrimSpace(rest[:jsonStart])
		}
		return rest
	}
	// Truncate if too long
	if len(line) > 200 {
		return line[:200] + "..."
	}
	return line
}
