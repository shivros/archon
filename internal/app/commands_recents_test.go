package app

import (
	"encoding/json"
	"testing"
)

func TestIsTurnCompletedEventMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
		want   bool
	}{
		{name: "slash", method: "turn/completed", want: true},
		{name: "dot", method: "turn.completed", want: true},
		{name: "underscore", method: "turn_completed", want: true},
		{name: "trimmed", method: "  turn/completed  ", want: true},
		{name: "other", method: "item/completed", want: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := isTurnCompletedEventMethod(tt.method); got != tt.want {
				t.Fatalf("isTurnCompletedEventMethod(%q)=%v want=%v", tt.method, got, tt.want)
			}
		})
	}
}

func TestParseRecentsCompletionTurnID(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "nested turn id", raw: `{"turn":{"id":"turn-a"}}`, want: "turn-a"},
		{name: "turn_id", raw: `{"turn_id":"turn-b"}`, want: "turn-b"},
		{name: "turnID", raw: `{"turnID":"turn-c"}`, want: "turn-c"},
		{name: "root id", raw: `{"id":"turn-d"}`, want: "turn-d"},
		{name: "invalid payload", raw: `{"turn":`, want: ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := parseRecentsCompletionTurnID(json.RawMessage(tt.raw))
			if got != tt.want {
				t.Fatalf("parseRecentsCompletionTurnID(%q)=%q want=%q", tt.raw, got, tt.want)
			}
		})
	}
}
