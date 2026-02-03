package app

import (
	"encoding/json"
	"fmt"
	"strings"
)

func itemsToLines(items []map[string]any) []string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, formatItem(item)...)
	}
	return lines
}

func formatItem(item map[string]any) []string {
	if item == nil {
		return nil
	}
	typ, _ := item["type"].(string)
	switch typ {
	case "log":
		if text := asString(item["text"]); text != "" {
			return []string{text}
		}
	case "userMessage":
		if text := extractContentText(item["content"]); text != "" {
			return []string{"User: " + text}
		}
		if text := asString(item["text"]); text != "" {
			return []string{"User: " + text}
		}
	case "agentMessage":
		if text := asString(item["text"]); text != "" {
			return []string{"Agent: " + text}
		}
		if text := extractContentText(item["content"]); text != "" {
			return []string{"Agent: " + text}
		}
	case "commandExecution":
		cmd := extractCommand(item["command"])
		status := asString(item["status"])
		line := "Command"
		if cmd != "" {
			line += ": " + cmd
		}
		if status != "" {
			line += " (" + status + ")"
		}
		return []string{line}
	case "fileChange":
		paths := extractChangePaths(item["changes"])
		if len(paths) > 0 {
			return []string{"File change: " + strings.Join(paths, ", ")}
		}
	case "enteredReviewMode":
		if text := asString(item["review"]); text != "" {
			return []string{"Review started: " + text}
		}
	case "exitedReviewMode":
		if text := asString(item["review"]); text != "" {
			return []string{"Review completed: " + text}
		}
	}
	if typ != "" {
		if data, err := json.Marshal(item); err == nil {
			return []string{fmt.Sprintf("%s: %s", typ, string(data))}
		}
	}
	if data, err := json.Marshal(item); err == nil {
		return []string{string(data)}
	}
	return nil
}

func extractContentText(raw any) string {
	items, ok := raw.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, entry := range items {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := m["type"].(string); typ == "text" {
			if text := asString(m["text"]); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}

func extractCommand(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, entry := range value {
			if text := asString(entry); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func extractChangePaths(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	var paths []string
	for _, entry := range items {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if path := asString(m["path"]); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}
