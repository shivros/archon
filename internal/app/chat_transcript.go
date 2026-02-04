package app

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ChatTranscript struct {
	lines             []string
	maxLines          int
	activeAgentLine   int
	pendingAgentBlock bool
}

func NewChatTranscript(maxLines int) *ChatTranscript {
	return &ChatTranscript{
		maxLines:        maxLines,
		activeAgentLine: -1,
	}
}

func (t *ChatTranscript) Reset() {
	if t == nil {
		return
	}
	t.lines = nil
	t.activeAgentLine = -1
	t.pendingAgentBlock = false
}

func (t *ChatTranscript) SetLines(lines []string) {
	if t == nil {
		return
	}
	t.lines = trimLines(lines, t.maxLines)
	t.activeAgentLine = -1
	t.pendingAgentBlock = false
}

func (t *ChatTranscript) Lines() []string {
	if t == nil {
		return nil
	}
	return t.lines
}

func (t *ChatTranscript) AppendUserMessage(text string) {
	if t == nil || strings.TrimSpace(text) == "" {
		return
	}
	t.lines = append(t.lines, "### User", "")
	t.lines = append(t.lines, text, "")
	t.trim()
}

func (t *ChatTranscript) StartAgentBlock() {
	if t == nil {
		return
	}
	t.lines = append(t.lines, "### Agent", "")
	t.lines = append(t.lines, "")
	t.activeAgentLine = len(t.lines) - 1
	t.pendingAgentBlock = true
	t.trim()
}

func (t *ChatTranscript) AppendAgentDelta(delta string) {
	if t == nil {
		return
	}
	if t.activeAgentLine < 0 || t.activeAgentLine >= len(t.lines) {
		if !t.pendingAgentBlock {
			t.StartAgentBlock()
		}
	}
	if t.activeAgentLine < 0 || t.activeAgentLine >= len(t.lines) {
		return
	}
	parts := strings.Split(delta, "\n")
	if len(parts) == 0 {
		return
	}
	t.lines[t.activeAgentLine] += parts[0]
	for _, part := range parts[1:] {
		t.lines = append(t.lines, part)
		t.activeAgentLine = len(t.lines) - 1
	}
	t.trim()
}

func (t *ChatTranscript) FinishAgentBlock() {
	if t == nil {
		return
	}
	t.activeAgentLine = -1
	t.pendingAgentBlock = false
	t.trim()
}

func (t *ChatTranscript) AppendItem(item map[string]any) {
	if t == nil || item == nil {
		return
	}
	typ, _ := item["type"].(string)
	switch typ {
	case "log":
		if text := asString(item["text"]); text != "" {
			t.appendLines(escapeMarkdown(text))
		}
	case "userMessage":
		if text := extractContentText(item["content"]); text != "" {
			t.AppendUserMessage(text)
			return
		}
		if text := asString(item["text"]); text != "" {
			t.AppendUserMessage(text)
		}
	case "agentMessage":
		if text := asString(item["text"]); text != "" {
			t.appendLines("### Agent", "", text, "")
			return
		}
		if text := extractContentText(item["content"]); text != "" {
			t.appendLines("### Agent", "", text, "")
		}
	case "commandExecution":
		cmd := extractCommand(item["command"])
		status := asString(item["status"])
		lines := []string{"### Command"}
		if cmd != "" {
			lines = append(lines, "", escapeMarkdown(cmd))
		}
		if status != "" {
			lines = append(lines, "", "Status: "+escapeMarkdown(status))
		}
		lines = append(lines, "")
		t.appendLines(lines...)
	case "fileChange":
		paths := extractChangePaths(item["changes"])
		if len(paths) > 0 {
			t.appendLines("### File change", "", escapeMarkdown(strings.Join(paths, ", ")), "")
		}
	case "enteredReviewMode":
		if text := asString(item["review"]); text != "" {
			t.appendLines("### Review started", "", text, "")
		}
	case "exitedReviewMode":
		if text := asString(item["review"]); text != "" {
			t.appendLines("### Review completed", "", text, "")
		}
	case "reasoning":
		summary := extractStringList(item["summary"])
		if len(summary) > 0 {
			lines := []string{"> ### Reasoning (summary)", ">"}
			for _, entry := range summary {
				for _, line := range strings.Split(entry, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					lines = append(lines, "> - "+escapeMarkdown(line))
				}
			}
			lines = append(lines, ">")
			t.appendLines(lines...)
			return
		}
		if text := extractContentText(item["content"]); text != "" {
			lines := []string{"> ### Reasoning", ">"}
			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					lines = append(lines, ">")
					continue
				}
				lines = append(lines, "> "+escapeMarkdown(line))
			}
			lines = append(lines, ">")
			t.appendLines(lines...)
		}
	default:
		if typ != "" {
			if data, err := json.Marshal(item); err == nil {
				t.appendLines(escapeMarkdown(fmt.Sprintf("%s: %s", typ, string(data))))
				return
			}
		}
		if data, err := json.Marshal(item); err == nil {
			t.appendLines(escapeMarkdown(string(data)))
		}
	}
}

func (t *ChatTranscript) appendLines(lines ...string) {
	if t == nil || len(lines) == 0 {
		return
	}
	t.lines = append(t.lines, lines...)
	t.trim()
}

func (t *ChatTranscript) trim() {
	if t.maxLines <= 0 || len(t.lines) <= t.maxLines {
		return
	}
	drop := len(t.lines) - t.maxLines
	t.lines = t.lines[drop:]
	if t.activeAgentLine >= 0 {
		t.activeAgentLine -= drop
		if t.activeAgentLine < 0 {
			t.activeAgentLine = len(t.lines) - 1
		}
	}
}
