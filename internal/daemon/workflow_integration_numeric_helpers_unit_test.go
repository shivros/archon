package daemon

import "testing"

func TestExtractStrictNumber(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		want  int
		found bool
	}{
		{name: "plain", text: "24", want: 24, found: true},
		{name: "whitespace", text: "  24  \n", want: 24, found: true},
		{name: "mixed text", text: "result is 24", found: false},
		{name: "negative", text: "-24", found: false},
		{name: "decimal", text: "24.0", found: false},
		{name: "too many digits", text: "1234567", found: false},
		{name: "empty", text: "", found: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractStrictNumber(tc.text)
			if ok != tc.found {
				t.Fatalf("found mismatch: got=%v want=%v (value=%d)", ok, tc.found, got)
			}
			if tc.found && got != tc.want {
				t.Fatalf("value mismatch: got=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestStrictNumberFromHistoryItem(t *testing.T) {
	t.Run("rejects non-agent item", func(t *testing.T) {
		item := map[string]any{"type": "userMessage", "text": "24"}
		if _, ok := strictNumberFromHistoryItem(item); ok {
			t.Fatalf("expected non-agent item to be ignored")
		}
	})

	t.Run("reads direct text payload", func(t *testing.T) {
		item := map[string]any{"type": "assistant", "text": "24"}
		got, ok := strictNumberFromHistoryItem(item)
		if !ok || got != 24 {
			t.Fatalf("expected strict number from direct text, got=%d ok=%v", got, ok)
		}
	})

	t.Run("reads nested assistant message content", func(t *testing.T) {
		item := map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "24"},
				},
			},
		}
		got, ok := strictNumberFromHistoryItem(item)
		if !ok || got != 24 {
			t.Fatalf("expected strict number from nested content, got=%d ok=%v", got, ok)
		}
	})
}

func TestFindStrictNumberInHistoryItems(t *testing.T) {
	items := []map[string]any{
		{"type": "assistant", "text": "hello"},
		{"type": "assistant", "text": "24"},
		{"type": "assistant", "text": "31"},
	}
	got, ok := findStrictNumberInHistoryItems(items, 20, 30)
	if !ok || got != 24 {
		t.Fatalf("expected first in-range number=24, got=%d ok=%v", got, ok)
	}

	if _, ok := findStrictNumberInHistoryItems(items, 40, 50); ok {
		t.Fatalf("expected no number in [40,50]")
	}
}

func TestHasStrictNumberInHistoryItems(t *testing.T) {
	items := []map[string]any{
		{"type": "assistant", "text": "24"},
		{"type": "assistant", "text": "31"},
	}
	if !hasStrictNumberInHistoryItems(items, 31) {
		t.Fatalf("expected to find 31 in history items")
	}
	if hasStrictNumberInHistoryItems(items, 99) {
		t.Fatalf("did not expect to find 99 in history items")
	}
}

func TestHistoryItemContainsExpectedNumber(t *testing.T) {
	t.Run("matches numeric token in agent text", func(t *testing.T) {
		item := map[string]any{"type": "assistant", "text": "The answer is 84."}
		if !historyItemContainsExpectedNumber(item, 84) {
			t.Fatalf("expected token match for 84")
		}
	})

	t.Run("does not match different number", func(t *testing.T) {
		item := map[string]any{"type": "assistant", "text": "The answer is 42."}
		if historyItemContainsExpectedNumber(item, 84) {
			t.Fatalf("did not expect token match for 84")
		}
	})

	t.Run("ignores non-agent items", func(t *testing.T) {
		item := map[string]any{"type": "userMessage", "text": "84"}
		if historyItemContainsExpectedNumber(item, 84) {
			t.Fatalf("did not expect user item token match")
		}
	})
}

func TestHasExpectedNumberInHistoryItems(t *testing.T) {
	items := []map[string]any{
		{"type": "assistant", "text": "I got 12 first."},
		{"type": "assistant", "text": "Final result is 29."},
	}
	if !hasExpectedNumberInHistoryItems(items, 29) {
		t.Fatalf("expected to find expected number token in history")
	}
	if hasExpectedNumberInHistoryItems(items, 88) {
		t.Fatalf("did not expect to find missing expected number token")
	}
}

func TestIsAgentHistoryItem(t *testing.T) {
	if isAgentHistoryItem(nil) {
		t.Fatalf("nil item should not be considered agent history item")
	}
	for _, typ := range []string{"agentMessage", "agentMessageDelta", "assistant"} {
		if !isAgentHistoryItem(map[string]any{"type": typ}) {
			t.Fatalf("expected %q to be recognized as agent history item", typ)
		}
	}
	if isAgentHistoryItem(map[string]any{"type": "userMessage"}) {
		t.Fatalf("userMessage should not be recognized as agent history item")
	}
}
