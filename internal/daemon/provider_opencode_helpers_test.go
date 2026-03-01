package daemon

import (
	"encoding/json"
	"testing"
)

func TestOpenCodeMissingHistoryItemsSkipsEquivalentRemoteWhenLocalLacksProviderID(t *testing.T) {
	local := []map[string]any{
		{
			"type": "userMessage",
			"content": []map[string]any{
				{"type": "text", "text": "What's the current git status?"},
			},
		},
	}
	remote := []map[string]any{
		{
			"type":                "userMessage",
			"provider_message_id": "msg_123",
			"provider_created_at": "2026-02-27T04:09:18Z",
			"content": []map[string]any{
				{"type": "text", "text": "What's the current git status?"},
			},
		},
	}
	missing := openCodeMissingHistoryItems(local, remote)
	if len(missing) != 0 {
		t.Fatalf("expected no missing items, got %#v", missing)
	}
}

func TestOpenCodeMissingHistoryItemsPreservesCardinalityAcrossEquivalentText(t *testing.T) {
	local := []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Done."},
				},
			},
		},
	}
	remote := []map[string]any{
		{
			"type":                "assistant",
			"provider_message_id": "msg_1",
			"message": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Done."},
				},
			},
		},
		{
			"type":                "assistant",
			"provider_message_id": "msg_2",
			"message": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Done."},
				},
			},
		},
	}
	missing := openCodeMissingHistoryItems(local, remote)
	if len(missing) != 1 {
		t.Fatalf("expected exactly one missing item, got %#v", missing)
	}
	if got := asString(missing[0]["provider_message_id"]); got != "msg_2" {
		t.Fatalf("expected second remote item to remain missing, got %#v", missing[0])
	}
}

func TestOpenCodeMissingHistoryItemsKeepsDifferentText(t *testing.T) {
	local := []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Done."},
				},
			},
		},
	}
	remote := []map[string]any{
		{
			"type":                "assistant",
			"provider_message_id": "msg_new",
			"message": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Still working..."},
				},
			},
		},
	}
	missing := openCodeMissingHistoryItems(local, remote)
	if len(missing) != 1 {
		t.Fatalf("expected one missing item, got %#v", missing)
	}
}

func TestExtractOpenCodePartsTextIncludesReasoningAndTools(t *testing.T) {
	text := extractOpenCodePartsText([]map[string]any{
		{"type": "reasoning", "summary": "planning"},
		{"type": "tool-call", "name": "grep"},
		{"type": "tool-result", "output": "found 3 matches"},
	})
	if text == "" {
		t.Fatalf("expected canonical text for non-text parts")
	}
	if text != "planning\n[tool call: grep]\nfound 3 matches" {
		t.Fatalf("unexpected canonical text: %q", text)
	}
}

func TestOpenCodeLatestAssistantSnapshotUsesCanonicalNonTextContent(t *testing.T) {
	messages := []openCodeSessionMessage{
		{
			Info: map[string]any{
				"role": "assistant",
				"id":   "msg-reasoning",
			},
			Parts: []map[string]any{
				{"type": "reasoning", "summary": "thinking"},
			},
		},
	}
	snapshot := openCodeLatestAssistantSnapshot(messages)
	if snapshot.MessageID != "msg-reasoning" {
		t.Fatalf("expected assistant message id, got %q", snapshot.MessageID)
	}
	if snapshot.Text != "thinking" {
		t.Fatalf("expected canonical reasoning snapshot text, got %q", snapshot.Text)
	}
}

func TestOpenCodeCompactShadowItemsDropsNoIDShadowWhenProviderIDExists(t *testing.T) {
	items := []map[string]any{
		{
			"type": "userMessage",
			"content": []map[string]any{
				{"type": "text", "text": "What's the current git status?"},
			},
		},
		{
			"type":                "userMessage",
			"provider_message_id": "msg_1",
			"content": []map[string]any{
				{"type": "text", "text": "What's the current git status?"},
			},
		},
	}
	compacted := openCodeCompactShadowItems(items)
	if len(compacted) != 1 {
		t.Fatalf("expected one compacted item, got %#v", compacted)
	}
	if got := asString(compacted[0]["provider_message_id"]); got != "msg_1" {
		t.Fatalf("expected provider-id item to remain, got %#v", compacted[0])
	}
}

func TestOpenCodeCompactShadowItemsKeepsAllDistinctMessages(t *testing.T) {
	items := []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Done."},
				},
			},
		},
		{
			"type":                "assistant",
			"provider_message_id": "msg_done",
			"message": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Done."},
				},
			},
		},
		{
			"type":                "assistant",
			"provider_message_id": "msg_more",
			"message": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Still working..."},
				},
			},
		},
	}
	compacted := openCodeCompactShadowItems(items)
	if len(compacted) != 2 {
		t.Fatalf("expected two compacted items, got %#v", compacted)
	}
	if asString(compacted[0]["provider_message_id"]) == "" || asString(compacted[1]["provider_message_id"]) == "" {
		t.Fatalf("expected provider-id items to remain, got %#v", compacted)
	}
}

func TestMapOpenCodeEventToCodexSessionErrorEmitsTurnCompleted(t *testing.T) {
	tests := []struct {
		name           string
		errorName      string
		wantStatus     string
		wantEventCount int
	}{
		{
			name:           "api_error_produces_failed_turn",
			errorName:      "APIError",
			wantStatus:     "failed",
			wantEventCount: 2,
		},
		{
			name:           "abort_produces_interrupted_turn",
			errorName:      "MessageAbortedError",
			wantStatus:     "interrupted",
			wantEventCount: 2,
		},
		{
			name:           "generic_error_produces_failed_turn",
			errorName:      "",
			wantStatus:     "failed",
			wantEventCount: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errPayload := map[string]any{
				"type": "session.error",
				"properties": map[string]any{
					"sessionID": "ses_test",
					"error": map[string]any{
						"name": tc.errorName,
						"data": map[string]any{
							"message": "something went wrong",
						},
					},
				},
			}
			raw, _ := json.Marshal(errPayload)
			events := mapOpenCodeEventToCodex(string(raw), "ses_test", nil)
			if len(events) != tc.wantEventCount {
				t.Fatalf("expected %d events, got %d: %+v", tc.wantEventCount, len(events), events)
			}
			// First event should be "error".
			if events[0].Method != "error" {
				t.Fatalf("expected first event method 'error', got %q", events[0].Method)
			}
			// Second event should be "turn/completed" with the expected status.
			if events[1].Method != "turn/completed" {
				t.Fatalf("expected second event method 'turn/completed', got %q", events[1].Method)
			}
			var params map[string]any
			if err := json.Unmarshal(events[1].Params, &params); err != nil {
				t.Fatalf("unmarshal turn/completed params: %v", err)
			}
			turn, _ := params["turn"].(map[string]any)
			if turn == nil {
				t.Fatalf("expected turn in params, got %+v", params)
			}
			if got := asString(turn["status"]); got != tc.wantStatus {
				t.Fatalf("expected status %q, got %q", tc.wantStatus, got)
			}
		})
	}
}

func TestNormalizeOpenCodeSessionMessagesFromMapDataKey(t *testing.T) {
	payload := map[string]any{
		"data": []any{
			map[string]any{
				"role":      "assistant",
				"id":        "msg-1",
				"createdAt": "2026-01-01T00:00:00Z",
				"parts": []any{
					map[string]any{"type": "text", "text": "hello"},
				},
			},
		},
	}
	out := normalizeOpenCodeSessionMessages(payload)
	if len(out) != 1 {
		t.Fatalf("expected one normalized message, got %#v", out)
	}
	if role := openCodeSessionMessageRole(out[0]); role != "assistant" {
		t.Fatalf("expected assistant role, got %q", role)
	}
}

func TestNormalizeOpenCodeSessionMessagesFromSingleEntryMap(t *testing.T) {
	payload := map[string]any{
		"role": "user",
		"id":   "msg-u1",
		"parts": []any{
			map[string]any{"type": "text", "text": "hi"},
		},
	}
	out := normalizeOpenCodeSessionMessages(payload)
	if len(out) != 1 {
		t.Fatalf("expected one normalized message, got %#v", out)
	}
	if role := openCodeSessionMessageRole(out[0]); role != "user" {
		t.Fatalf("expected user role, got %q", role)
	}
}

func TestNormalizeOpenCodeSessionMessagesInvalidShape(t *testing.T) {
	if out := normalizeOpenCodeSessionMessages("invalid"); len(out) != 0 {
		t.Fatalf("expected empty output for invalid payload, got %#v", out)
	}
}

func TestParseOpenCodeSessionMessageDerivesPartsFromMessageContent(t *testing.T) {
	raw := map[string]any{
		"info": map[string]any{
			"role": "assistant",
			"id":   "msg-a1",
		},
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "from content"},
			},
		},
	}
	parsed, ok := parseOpenCodeSessionMessage(raw)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if text := extractOpenCodeSessionMessageText(parsed); text != "from content" {
		t.Fatalf("expected extracted content text, got %q", text)
	}
}

func TestParseOpenCodeSessionMessageDerivesRoleFromTypeField(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"id":   "msg-a2",
		"parts": []any{
			map[string]any{"type": "text", "text": "typed assistant content"},
		},
	}
	parsed, ok := parseOpenCodeSessionMessage(raw)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if role := openCodeSessionMessageRole(parsed); role != "assistant" {
		t.Fatalf("expected assistant role from type field, got %q", role)
	}
	if text := extractOpenCodeSessionMessageText(parsed); text != "typed assistant content" {
		t.Fatalf("expected text extraction from typed message, got %q", text)
	}
}
