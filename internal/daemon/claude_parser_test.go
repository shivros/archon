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

func TestClaudeParseUserLineIgnoresPlainEcho(t *testing.T) {
	line := `{"type":"user","message":{"id":"msg_u1","role":"user","content":[{"type":"text","text":"hello"}]}}`
	items, _, err := ParseClaudeLine(line, &ClaudeParseState{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected plain user echo to be ignored, got %#v", items)
	}
}

func TestClaudeParseUserThinkingMapsToReasoning(t *testing.T) {
	line := `{"type":"user","message":{"id":"msg_r1","role":"user","content":[{"type":"thinking","thinking":"draft plan"},{"type":"text","text":"ignored text block"}]}}`
	items, _, err := ParseClaudeLine(line, &ClaudeParseState{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one reasoning item, got %#v", items)
	}
	item := items[0]
	if typ, _ := item["type"].(string); typ != "reasoning" {
		t.Fatalf("expected reasoning item type, got %#v", item)
	}
	if id, _ := item["id"].(string); id != "msg_r1" {
		t.Fatalf("expected reasoning id msg_r1, got %#v", item["id"])
	}
	content, ok := item["content"].([]map[string]any)
	if !ok || len(content) != 1 {
		t.Fatalf("expected reasoning content array, got %#v", item["content"])
	}
	if text, _ := content[0]["text"].(string); text != "draft plan" {
		t.Fatalf("expected reasoning text, got %#v", content[0]["text"])
	}
}
