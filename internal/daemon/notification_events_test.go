package daemon

import (
	"testing"
)

func TestParseTurnIDFromEventParams(t *testing.T) {
	got := parseTurnIDFromEventParams([]byte(`{"turn":{"id":"turn-nested"}}`))
	if got != "turn-nested" {
		t.Fatalf("expected nested turn id, got %q", got)
	}
	got = parseTurnIDFromEventParams([]byte(`{"turn_id":"turn-top-level"}`))
	if got != "turn-top-level" {
		t.Fatalf("expected top-level turn id, got %q", got)
	}
}

func TestParseTurnEventFromParamsAndTurnErrorMessageVariants(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantTurnID string
		wantStatus string
		wantError  string
	}{
		{
			name:       "nested turn with nested error message",
			raw:        `{"turn":{"id":"turn-1","status":"failed","error":{"message":"unsupported model"}}}`,
			wantTurnID: "turn-1",
			wantStatus: "failed",
			wantError:  "unsupported model",
		},
		{
			name:       "top level fields with string error",
			raw:        `{"turn_id":"turn-2","status":"error","error":"provider timeout"}`,
			wantTurnID: "turn-2",
			wantStatus: "error",
			wantError:  "provider timeout",
		},
		{
			name:       "top level map error data message",
			raw:        `{"turn_id":"turn-3","status":"failed","error":{"data":{"message":"model not supported"}}}`,
			wantTurnID: "turn-3",
			wantStatus: "failed",
			wantError:  "model not supported",
		},
		{
			name:       "top level map error string field",
			raw:        `{"turn_id":"turn-3b","status":"failed","error":{"error":"backend unavailable"}}`,
			wantTurnID: "turn-3b",
			wantStatus: "failed",
			wantError:  "backend unavailable",
		},
		{
			name:       "turn fallback to top level error",
			raw:        `{"turn":{"id":"turn-4","status":"failed"},"error":{"message":"quota exceeded"}}`,
			wantTurnID: "turn-4",
			wantStatus: "failed",
			wantError:  "quota exceeded",
		},
		{
			name:       "invalid json returns zero value",
			raw:        `{`,
			wantTurnID: "",
			wantStatus: "",
			wantError:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTurnEventFromParams([]byte(tc.raw))
			if got.TurnID != tc.wantTurnID || got.Status != tc.wantStatus || got.Error != tc.wantError {
				t.Fatalf("unexpected parsed turn event: got=%#v wantTurnID=%q wantStatus=%q wantError=%q", got, tc.wantTurnID, tc.wantStatus, tc.wantError)
			}
		})
	}
}

func TestTurnErrorMessageFallbacks(t *testing.T) {
	if got := turnErrorMessage(nil); got != "" {
		t.Fatalf("expected empty message for nil, got %q", got)
	}
	if got := turnErrorMessage(123); got != "" {
		t.Fatalf("expected empty message for unsupported type, got %q", got)
	}
}
