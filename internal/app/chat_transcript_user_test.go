package app

import "testing"

func TestChatTranscriptAppendUserMessageTrimsLeadingNewlines(t *testing.T) {
	tp := NewChatTranscript(0)
	if tp == nil {
		t.Fatalf("expected transcript")
	}

	idx := tp.AppendUserMessage("\r\n\nhello from user")
	if idx != 0 {
		t.Fatalf("expected user block index 0, got %d", idx)
	}

	blocks := tp.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one block, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleUser {
		t.Fatalf("expected user role, got %s", blocks[0].Role)
	}
	if blocks[0].Text != "hello from user" {
		t.Fatalf("expected trimmed user text, got %q", blocks[0].Text)
	}
}

func TestChatTranscriptAppendItemUserMessageTrimsLeadingNewlines(t *testing.T) {
	tp := NewChatTranscript(0)
	if tp == nil {
		t.Fatalf("expected transcript")
	}

	tp.AppendItem(map[string]any{
		"type": "userMessage",
		"content": []any{
			map[string]any{"type": "text", "text": "\n\nhello from stream"},
		},
	})

	blocks := tp.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one block, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleUser {
		t.Fatalf("expected user role, got %s", blocks[0].Role)
	}
	if blocks[0].Text != "hello from stream" {
		t.Fatalf("expected trimmed user text, got %q", blocks[0].Text)
	}
}
