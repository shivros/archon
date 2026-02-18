package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrepareItemForPersistenceBackfillsCreatedAt(t *testing.T) {
	fallback := time.Date(2026, 2, 18, 12, 34, 56, 0, time.UTC)
	item := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "hello"},
			},
		},
	}

	prepared := prepareItemForPersistence(item, fallback)
	if prepared == nil {
		t.Fatalf("expected prepared item")
	}
	if got := strings.TrimSpace(asString(prepared["created_at"])); got != fallback.Format(time.RFC3339Nano) {
		t.Fatalf("expected created_at %q, got %q", fallback.Format(time.RFC3339Nano), got)
	}
	if _, ok := item["created_at"]; ok {
		t.Fatalf("expected original item to remain unchanged")
	}
}

func TestPrepareItemForPersistenceUsesProviderTimestamp(t *testing.T) {
	item := map[string]any{
		"type":                "userMessage",
		"provider_created_at": "2026-02-18T13:30:00Z",
	}

	prepared := prepareItemForPersistence(item, time.Time{})
	if prepared == nil {
		t.Fatalf("expected prepared item")
	}
	got := strings.TrimSpace(asString(prepared["created_at"]))
	if got != "2026-02-18T13:30:00Z" {
		t.Fatalf("expected created_at to mirror provider timestamp, got %q", got)
	}
}

func TestPrepareItemForPersistenceClassification(t *testing.T) {
	item := map[string]any{
		"type": "userMessage",
	}
	fallback := time.Date(2026, 2, 18, 13, 45, 0, 0, time.UTC)
	prepared, classification := prepareItemForPersistenceWithClassification(item, fallback)
	if prepared == nil {
		t.Fatalf("expected prepared item")
	}
	if classification.HasProviderTimestamp {
		t.Fatalf("expected provider timestamp to be missing")
	}
	if !classification.UsedDaemonTimestamp {
		t.Fatalf("expected daemon timestamp to be used")
	}
}

func TestPrepareItemForPersistenceUsesNestedTimestamp(t *testing.T) {
	item := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"createdAt": "2026-02-18T13:40:00Z",
			"content": []map[string]any{
				{"type": "text", "text": "nested"},
			},
		},
	}

	prepared := prepareItemForPersistence(item, time.Time{})
	if prepared == nil {
		t.Fatalf("expected prepared item")
	}
	got := strings.TrimSpace(asString(prepared["created_at"]))
	if got != "2026-02-18T13:40:00Z" {
		t.Fatalf("expected created_at from nested message timestamp, got %q", got)
	}
}

func TestItemSinkAppendPersistsCreatedAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.jsonl")
	sink, err := newItemSink(path, nil, nil)
	if err != nil {
		t.Fatalf("newItemSink: %v", err)
	}
	t.Cleanup(sink.Close)

	sink.Append(map[string]any{
		"type": "userMessage",
		"content": []map[string]any{
			{"type": "text", "text": "hello"},
		},
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		t.Fatalf("expected persisted item line")
	}
	var item map[string]any
	if err := json.Unmarshal([]byte(line), &item); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if strings.TrimSpace(asString(item["created_at"])) == "" {
		t.Fatalf("expected created_at in persisted item, got %#v", item)
	}
}
