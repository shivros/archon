package daemon

import (
	"encoding/json"
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
		items = append(items, map[string]any{
			"type":  "agentMessageDelta",
			"delta": text,
		})
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
