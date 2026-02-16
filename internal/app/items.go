package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func itemsToBlocks(items []map[string]any) []ChatBlock {
	transcript := NewChatTranscript(0)
	for _, item := range items {
		transcript.AppendItem(item)
	}
	return transcript.Blocks()
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

func chatItemCreatedAt(item map[string]any) time.Time {
	if item == nil {
		return time.Time{}
	}
	for _, key := range []string{"provider_created_at", "created_at", "createdAt", "ts", "timestamp"} {
		if when := parseChatTimestamp(item[key]); !when.IsZero() {
			return when
		}
	}
	if message, ok := item["message"].(map[string]any); ok && message != nil {
		for _, key := range []string{"created_at", "createdAt", "ts", "timestamp"} {
			if when := parseChatTimestamp(message[key]); !when.IsZero() {
				return when
			}
		}
	}
	if info, ok := item["info"].(map[string]any); ok && info != nil {
		for _, key := range []string{"created_at", "createdAt", "ts", "timestamp"} {
			if when := parseChatTimestamp(info[key]); !when.IsZero() {
				return when
			}
		}
	}
	if clock, ok := item["time"].(map[string]any); ok && clock != nil {
		for _, key := range []string{"created", "created_at", "ts"} {
			if when := parseChatTimestamp(clock[key]); !when.IsZero() {
				return when
			}
		}
	}
	return time.Time{}
}

func parseChatTimestamp(raw any) time.Time {
	parseUnix := func(value int64) time.Time {
		switch {
		case value >= 1_000_000_000_000_000_000:
			return time.Unix(0, value).UTC()
		case value >= 1_000_000_000_000_000:
			return time.UnixMicro(value).UTC()
		case value >= 1_000_000_000_000:
			return time.UnixMilli(value).UTC()
		case value > 0:
			return time.Unix(value, 0).UTC()
		default:
			return time.Time{}
		}
	}
	switch typed := raw.(type) {
	case time.Time:
		return typed.UTC()
	case string:
		value := strings.TrimSpace(typed)
		if value == "" {
			return time.Time{}
		}
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC()
		}
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parseUnix(n)
		}
	case int64:
		return parseUnix(typed)
	case int:
		return parseUnix(int64(typed))
	case float64:
		return parseUnix(int64(typed))
	case jsonNumberLike:
		if n, err := typed.Int64(); err == nil {
			return parseUnix(n)
		}
	}
	return time.Time{}
}

type jsonNumberLike interface {
	Int64() (int64, error)
}
