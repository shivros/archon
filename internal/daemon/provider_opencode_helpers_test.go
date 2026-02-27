package daemon

import "testing"

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
