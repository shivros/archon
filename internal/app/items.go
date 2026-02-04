package app

import (
	"fmt"
	"strings"
)

func itemsToLines(items []map[string]any) []string {
	transcript := NewChatTranscript(0)
	for _, item := range items {
		transcript.AppendItem(item)
	}
	return transcript.Lines()
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

func extractStringList(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, entry := range items {
		if text := asString(entry); text != "" {
			result = append(result, text)
		}
	}
	return result
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
