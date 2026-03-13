package transcriptadapters

import (
	"encoding/json"

	"control/internal/daemon/transcriptdomain"
)

type codexTranscriptTextExtractor struct{}

func newCodexTranscriptTextExtractor() TranscriptTextExtractor {
	return codexTranscriptTextExtractor{}
}

func (codexTranscriptTextExtractor) Extract(raw any) string {
	switch typed := raw.(type) {
	case nil:
		return ""
	case string:
		return transcriptdomain.PreserveText(typed)
	case json.Number:
		return transcriptdomain.PreserveText(typed.String())
	case map[string]any:
		return extractCodexTextFromMap(typed)
	case []map[string]any:
		return extractCodexTextFromMapSlice(typed)
	case []any:
		return extractCodexTextFromAnySlice(typed)
	default:
		return ""
	}
}

func extractCodexTextFromMap(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if text := firstNonEmptyExtracted(newCodexTranscriptTextExtractor(),
		raw["text"],
		raw["delta"],
		raw["thinking"],
	); !transcriptdomain.IsSemanticallyEmpty(text) {
		return text
	}
	return firstNonEmptyExtracted(newCodexTranscriptTextExtractor(),
		raw["content"],
		raw["message"],
		raw["result"],
	)
}

func extractCodexTextFromMapSlice(raw []map[string]any) string {
	if len(raw) == 0 {
		return ""
	}
	parts := make([]string, 0, len(raw))
	for _, entry := range raw {
		parts = append(parts, extractCodexTextFragmentFromMap(entry))
	}
	return joinLosslessText(parts)
}

func extractCodexTextFromAnySlice(raw []any) string {
	if len(raw) == 0 {
		return ""
	}
	parts := make([]string, 0, len(raw))
	for _, entry := range raw {
		text, _ := extractCodexTextFragmentValue(entry)
		parts = append(parts, text)
	}
	return joinLosslessText(parts)
}

func extractCodexTextFragmentFromMap(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	if text, ok := extractCodexTextFragmentValue(raw["text"]); ok {
		return text
	}
	if text, ok := extractCodexTextFragmentValue(raw["delta"]); ok {
		return text
	}
	if text, ok := extractCodexTextFragmentValue(raw["thinking"]); ok {
		return text
	}
	if text, ok := extractCodexTextFragmentValue(raw["content"]); ok {
		return text
	}
	if text, ok := extractCodexTextFragmentValue(raw["message"]); ok {
		return text
	}
	if text, ok := extractCodexTextFragmentValue(raw["result"]); ok {
		return text
	}
	return ""
}

func extractCodexTextFragmentValue(raw any) (string, bool) {
	switch typed := raw.(type) {
	case nil:
		return "", false
	case string:
		return transcriptdomain.PreserveText(typed), true
	case json.Number:
		return transcriptdomain.PreserveText(typed.String()), true
	case map[string]any:
		return extractCodexTextFragmentFromMap(typed), true
	case []map[string]any:
		return extractCodexTextFromMapSlice(typed), true
	case []any:
		return extractCodexTextFromAnySlice(typed), true
	default:
		return "", false
	}
}

func joinLosslessText(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	out := ""
	for _, part := range parts {
		out += part
	}
	return out
}
