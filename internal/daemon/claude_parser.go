package daemon

import (
	"encoding/json"
	"strings"
)

type ClaudeParseState struct {
	SawDelta bool
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
		text := extractClaudeMessageText(payload["message"])
		if text == "" {
			return nil, "", nil
		}
		items = append(items, map[string]any{
			"type": "userMessage",
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		})
	case "assistant":
		text := extractClaudeMessageText(payload["message"])
		if text == "" {
			if state != nil && state.SawDelta {
				items = append(items, map[string]any{
					"type": "agentMessageEnd",
				})
				state.SawDelta = false
			}
			return items, "", nil
		}
		if state != nil && state.SawDelta {
			items = append(items, map[string]any{
				"type": "agentMessageEnd",
			})
			state.SawDelta = false
			return items, "", nil
		}
		items = append(items, map[string]any{
			"type": "agentMessage",
			"text": text,
		})
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
		delta, _ := event["delta"].(map[string]any)
		if delta == nil {
			return nil, "", nil
		}
		if deltaType, _ := delta["type"].(string); deltaType != "text_delta" {
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
		items = append(items, payload)
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
		if typ, _ := block["type"].(string); typ != "text" && typ != "text_delta" {
			continue
		}
		if text, _ := block["text"].(string); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}
