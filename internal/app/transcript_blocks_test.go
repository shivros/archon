package app

import (
	"encoding/json"
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestTranscriptBlocksToChatBlocksMapsRolesAndMetadata(t *testing.T) {
	created := "2026-03-03T05:00:00Z"
	blocks := transcriptBlocksToChatBlocks([]transcriptdomain.Block{
		{
			ID:   "a1",
			Kind: "assistant_message",
			Role: "assistant",
			Text: "reply",
			Meta: map[string]any{
				"turn_id":             "turn-1",
				"provider_message_id": "msg-1",
				"request_id":          json.Number("42"),
				"created_at":          created,
			},
		},
		{
			ID:   "u1",
			Kind: "user_message",
			Text: "hello",
		},
		{
			ID:   "r1",
			Kind: "reasoning_section",
			Text: "thinking",
		},
		{
			ID:   "x1",
			Kind: "approval_resolved",
			Text: "approved",
		},
		{
			ID:   "s1",
			Kind: "unknown_kind",
			Text: "system fallback",
		},
	})
	if len(blocks) != 5 {
		t.Fatalf("expected 5 mapped blocks, got %#v", blocks)
	}
	if blocks[0].Role != ChatRoleAgent || blocks[0].TurnID != "turn-1" || blocks[0].ProviderMessageID != "msg-1" || blocks[0].RequestID != 42 || blocks[0].CreatedAt.IsZero() {
		t.Fatalf("unexpected assistant block mapping: %#v", blocks[0])
	}
	if blocks[1].Role != ChatRoleUser {
		t.Fatalf("expected user role mapping, got %#v", blocks[1])
	}
	if blocks[2].Role != ChatRoleReasoning {
		t.Fatalf("expected reasoning role mapping, got %#v", blocks[2])
	}
	if blocks[3].Role != ChatRoleApprovalResolved {
		t.Fatalf("expected approval_resolved mapping, got %#v", blocks[3])
	}
	if blocks[4].Role != ChatRoleSystem {
		t.Fatalf("expected system fallback mapping, got %#v", blocks[4])
	}
}

func TestTranscriptBlocksToChatBlocksSkipsExactEmptyText(t *testing.T) {
	blocks := transcriptBlocksToChatBlocks([]transcriptdomain.Block{
		{Kind: "assistant_message", Role: "assistant", Text: ""},
		{Kind: "assistant_message", Role: "assistant", Text: "ok"},
	})
	if len(blocks) != 1 || blocks[0].Text != "ok" {
		t.Fatalf("expected only non-empty blocks, got %#v", blocks)
	}
}

func TestTranscriptBlocksToChatBlocksCoalescesAdjacentAssistantFragmentsWithSameID(t *testing.T) {
	blocks := transcriptBlocksToChatBlocks([]transcriptdomain.Block{
		{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "stream "},
		{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "fragment"},
		{ID: "msg-2", Kind: "assistant_delta", Role: "assistant", Text: "separate"},
	})
	if len(blocks) != 2 {
		t.Fatalf("expected adjacent fragments to coalesce, got %#v", blocks)
	}
	if blocks[0].Text != "stream fragment" {
		t.Fatalf("expected merged first fragment, got %#v", blocks[0])
	}
	if blocks[1].Text != "separate" {
		t.Fatalf("expected second message to remain separate, got %#v", blocks[1])
	}
}

func TestTranscriptBlocksToChatBlocksPreservesWhitespaceOnlyFragmentsWithinMessage(t *testing.T) {
	blocks := transcriptBlocksToChatBlocks([]transcriptdomain.Block{
		{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "Here's"},
		{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: " "},
		{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "the plan"},
		{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "\n\n"},
		{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "Rules:"},
	})
	if len(blocks) != 1 {
		t.Fatalf("expected whitespace fragments to coalesce into one block, got %#v", blocks)
	}
	if blocks[0].Text != "Here's the plan\n\nRules:" {
		t.Fatalf("expected whitespace fragments to be preserved, got %#v", blocks[0])
	}
}

func TestTranscriptAnyIntSupportsCommonShapes(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{name: "int", value: 4, want: 4, ok: true},
		{name: "int64", value: int64(5), want: 5, ok: true},
		{name: "float64", value: float64(6), want: 6, ok: true},
		{name: "string", value: "7", want: 7, ok: true},
		{name: "json number", value: json.Number("8"), want: 8, ok: true},
		{name: "invalid", value: "x", want: 0, ok: false},
	}
	for _, tc := range cases {
		got, ok := transcriptAnyInt(tc.value)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("%s: got (%d,%v) want (%d,%v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}
