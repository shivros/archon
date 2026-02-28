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

func TestClaudeParseRateLimitEventAllowedIsHidden(t *testing.T) {
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1771844400,"rateLimitType":"five_hour"}}`
	items, _, err := ParseClaudeLine(line, &ClaudeParseState{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items for allowed rate limit event, got %#v", items)
	}
}

func TestClaudeParseRateLimitEventLimitedCreatesRateLimitItem(t *testing.T) {
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"limited","resetsAt":1771844400,"rateLimitType":"five_hour","overageStatus":"rejected"}}`
	items, _, err := ParseClaudeLine(line, &ClaudeParseState{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %#v", items)
	}
	item := items[0]
	if got, _ := item["type"].(string); got != "rateLimit" {
		t.Fatalf("expected rateLimit item type, got %#v", item["type"])
	}
	if got, _ := item["provider"].(string); got != "claude" {
		t.Fatalf("expected provider claude, got %#v", item["provider"])
	}
	if got, _ := item["retry_at"].(string); got == "" {
		t.Fatalf("expected retry_at to be populated, got %#v", item)
	}
}

func TestClaudeParseRateLimitEventParsesStringResetTimestamp(t *testing.T) {
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"limited","resetsAt":"1771844400","rateLimitType":"five_hour"}}`
	items, _, err := ParseClaudeLine(line, &ClaudeParseState{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %#v", items)
	}
	item := items[0]
	if got, _ := item["retry_unix"].(int64); got != 1771844400 {
		t.Fatalf("expected retry_unix to use shared timestamp parsing, got %#v", item["retry_unix"])
	}
	if got, _ := item["retry_at"].(string); got == "" {
		t.Fatalf("expected retry_at from string timestamp, got %#v", item)
	}
}

func TestClaudeParseRateLimitEventMissingInfoIsIgnored(t *testing.T) {
	line := `{"type":"rate_limit_event"}`
	items, _, err := ParseClaudeLine(line, &ClaudeParseState{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items when rate_limit_info missing, got %#v", items)
	}
}

func TestClaudeParseRateLimitEventEmptyStatusIsIgnored(t *testing.T) {
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"   ","resetsAt":1771844400}}`
	items, _, err := ParseClaudeLine(line, &ClaudeParseState{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items for empty status, got %#v", items)
	}
}

func TestClaudeParseRateLimitEventLimitedWithoutResetStillEmitsItem(t *testing.T) {
	line := `{"type":"rate_limit_event","rate_limit_info":{"status":"limited","rateLimitType":"five_hour"}}`
	items, _, err := ParseClaudeLine(line, &ClaudeParseState{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %#v", items)
	}
	item := items[0]
	if got, _ := item["type"].(string); got != "rateLimit" {
		t.Fatalf("expected rateLimit item, got %#v", item)
	}
	if _, ok := item["retry_at"]; ok {
		t.Fatalf("expected no retry_at when reset absent, got %#v", item)
	}
	if _, ok := item["retry_unix"]; ok {
		t.Fatalf("expected no retry_unix when reset absent, got %#v", item)
	}
}
