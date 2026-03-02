package app

import "testing"

func TestDefaultStatusHistoryKeyPolicyHandlesExpectedKeysOnly(t *testing.T) {
	policy := defaultStatusHistoryKeyPolicy{}
	cases := map[string]statusHistoryKeyAction{
		"esc":    statusHistoryKeyActionClose,
		"up":     statusHistoryKeyActionMoveUp,
		"k":      statusHistoryKeyActionMoveUp,
		"down":   statusHistoryKeyActionMoveDown,
		"j":      statusHistoryKeyActionMoveDown,
		"pgup":   statusHistoryKeyActionPageUp,
		"pgdown": statusHistoryKeyActionPageDown,
		"home":   statusHistoryKeyActionHome,
		"end":    statusHistoryKeyActionEnd,
		"enter":  statusHistoryKeyActionCopy,
		"c":      statusHistoryKeyActionCopy,
	}
	for key, want := range cases {
		got, ok := policy.ActionFor(key)
		if !ok {
			t.Fatalf("expected key %q to be handled", key)
		}
		if got != want {
			t.Fatalf("expected key %q action %v, got %v", key, want, got)
		}
	}
	if _, ok := policy.ActionFor("q"); ok {
		t.Fatalf("expected q to remain unhandled")
	}
}
