package daemon

import "testing"

func TestCloneNotificationPayloadDeepClone(t *testing.T) {
	original := map[string]any{
		"trace_id": "trace-1",
		"nested": map[string]any{
			"status": "ok",
		},
		"items": []any{
			map[string]any{"k": "v"},
			"stable",
		},
		"": "ignored",
	}
	cloned := cloneNotificationPayload(original)
	original["trace_id"] = "mutated"
	original["nested"].(map[string]any)["status"] = "mutated"
	original["items"].([]any)[0].(map[string]any)["k"] = "mutated"
	if cloned["trace_id"] != "trace-1" {
		t.Fatalf("expected cloned trace id to remain unchanged, got %#v", cloned["trace_id"])
	}
	if cloned["nested"].(map[string]any)["status"] != "ok" {
		t.Fatalf("expected nested clone to remain unchanged, got %#v", cloned["nested"])
	}
	if cloned["items"].([]any)[0].(map[string]any)["k"] != "v" {
		t.Fatalf("expected nested array clone to remain unchanged, got %#v", cloned["items"])
	}
	if _, ok := cloned[""]; ok {
		t.Fatalf("expected empty key to be dropped")
	}
}

func TestNotificationPayloadBoolParsing(t *testing.T) {
	payload := map[string]any{
		"bool_true":   true,
		"bool_false":  false,
		"str_true":    "true",
		"str_yes":     "yes",
		"str_one":     "1",
		"str_invalid": "no",
	}
	if !notificationPayloadBool(payload, "bool_true") {
		t.Fatalf("expected true bool to parse as true")
	}
	if notificationPayloadBool(payload, "bool_false") {
		t.Fatalf("expected false bool to parse as false")
	}
	if !notificationPayloadBool(payload, "str_true") || !notificationPayloadBool(payload, "str_yes") || !notificationPayloadBool(payload, "str_one") {
		t.Fatalf("expected true-like strings to parse as true")
	}
	if notificationPayloadBool(payload, "str_invalid") {
		t.Fatalf("expected invalid string to parse as false")
	}
	if notificationPayloadBool(payload, "missing") {
		t.Fatalf("expected missing key to parse as false")
	}
}

func TestNotificationPayloadIntParsing(t *testing.T) {
	payload := map[string]any{
		"int_value":    3,
		"float_value":  float64(4),
		"string_value": "7",
		"invalid":      "x",
	}
	if got := notificationPayloadInt(payload, "int_value"); got != 3 {
		t.Fatalf("expected int 3, got %d", got)
	}
	if got := notificationPayloadInt(payload, "float_value"); got != 4 {
		t.Fatalf("expected float64(4) to parse as 4, got %d", got)
	}
	if got := notificationPayloadInt(payload, "string_value"); got != 7 {
		t.Fatalf("expected numeric string to parse as 7, got %d", got)
	}
	if got := notificationPayloadInt(payload, "invalid"); got != 0 {
		t.Fatalf("expected invalid to parse as 0, got %d", got)
	}
	if got := notificationPayloadInt(payload, "missing"); got != 0 {
		t.Fatalf("expected missing to parse as 0, got %d", got)
	}
}
