package daemon

import "testing"

func TestCanonicalizeOpenCodeSessionMessageReasoningVariant(t *testing.T) {
	message := openCodeSessionMessage{
		Info: map[string]any{
			"role": "assistant",
			"id":   "msg-1",
		},
		Parts: []map[string]any{
			{"type": "reasoning", "summary": "thinking"},
		},
	}
	canonical, ok := canonicalizeOpenCodeSessionMessage(message)
	if !ok {
		t.Fatalf("expected canonical message")
	}
	if canonical.Variant != "reasoning" {
		t.Fatalf("expected reasoning variant, got %q", canonical.Variant)
	}
	if canonical.Text != "thinking" {
		t.Fatalf("unexpected canonical text: %q", canonical.Text)
	}
}

func TestCanonicalizeOpenCodeSessionMessageToolVariant(t *testing.T) {
	message := openCodeSessionMessage{
		Info: map[string]any{
			"role": "assistant",
			"id":   "msg-2",
		},
		Parts: []map[string]any{
			{"type": "tool-call", "name": "grep"},
		},
	}
	canonical, ok := canonicalizeOpenCodeSessionMessage(message)
	if !ok {
		t.Fatalf("expected canonical message")
	}
	if canonical.Variant != "tool_call" {
		t.Fatalf("expected tool_call variant, got %q", canonical.Variant)
	}
}

func TestCanonicalizeOpenCodeSessionMessageRejectsEmptyText(t *testing.T) {
	message := openCodeSessionMessage{
		Info: map[string]any{
			"role": "assistant",
			"id":   "msg-empty",
		},
		Parts: []map[string]any{
			{"type": "text", "text": ""},
		},
	}
	if _, ok := canonicalizeOpenCodeSessionMessage(message); ok {
		t.Fatalf("expected canonicalization to reject empty text")
	}
}

func TestCanonicalizeOpenCodeSessionMessageRejectsMissingRole(t *testing.T) {
	message := openCodeSessionMessage{
		Info: map[string]any{"id": "msg-norole"},
		Parts: []map[string]any{
			{"type": "text", "text": "hi"},
		},
	}
	if _, ok := canonicalizeOpenCodeSessionMessage(message); ok {
		t.Fatalf("expected canonicalization to reject missing role")
	}
}

func TestCanonicalOpenCodeVariantFallsBackToNonText(t *testing.T) {
	message := openCodeSessionMessage{
		Info: map[string]any{
			"role": "assistant",
			"id":   "msg-weird",
		},
		Parts: []map[string]any{
			{"type": "custom-block", "text": "custom"},
		},
	}
	canonical, ok := canonicalizeOpenCodeSessionMessage(message)
	if !ok {
		t.Fatalf("expected canonical message")
	}
	if canonical.Variant != "non_text" {
		t.Fatalf("expected non_text variant, got %q", canonical.Variant)
	}
}
