package daemon

import (
	"strings"
	"time"
)

type canonicalMessage struct {
	Role       string
	Text       string
	MessageID  string
	CreatedAt  time.Time
	Variant    string
	RawMessage map[string]any
}

func canonicalizeOpenCodeSessionMessage(message openCodeSessionMessage) (canonicalMessage, bool) {
	role := strings.ToLower(strings.TrimSpace(openCodeSessionMessageRole(message)))
	if role == "" {
		return canonicalMessage{}, false
	}
	text := strings.TrimSpace(extractOpenCodeSessionMessageText(message))
	if text == "" {
		return canonicalMessage{}, false
	}
	variant := canonicalOpenCodeVariant(message)
	return canonicalMessage{
		Role:      role,
		Text:      text,
		MessageID: strings.TrimSpace(openCodeSessionMessageID(message)),
		CreatedAt: openCodeSessionMessageCreatedAt(message),
		Variant:   variant,
	}, true
}

func canonicalOpenCodeVariant(message openCodeSessionMessage) string {
	for _, part := range message.Parts {
		if part == nil {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(asString(part["type"])))
		switch kind {
		case "text":
			continue
		case "reasoning":
			return "reasoning"
		case "tool-call", "tool_use", "tool-use", "tool":
			return "tool_call"
		case "tool-result", "tool_output", "tool-output":
			return "tool_result"
		default:
			if kind != "" {
				return "non_text"
			}
		}
	}
	return "text"
}
