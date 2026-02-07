package daemon

import "testing"

func TestClaudeParseAssistantLine(t *testing.T) {
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"pong"}]}}`
	items, _, err := ParseClaudeLine(line, &ClaudeParseState{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected items")
	}
	found := false
	for _, item := range items {
		if item == nil {
			continue
		}
		if typ, _ := item["type"].(string); typ == "agentMessage" {
			if text, _ := item["text"].(string); text == "pong" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("agentMessage not found in items: %#v", items)
	}
}
