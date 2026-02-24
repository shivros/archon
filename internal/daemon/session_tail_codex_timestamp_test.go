package daemon

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

func TestHydrateCodexHistoryItemTimestampsPersistsAcrossReads(t *testing.T) {
	service := &SessionService{
		manager: &SessionManager{baseDir: t.TempDir()},
		logger:  logging.Nop(),
	}
	sessionID := "session-codex-ts-persist"
	items := []map[string]any{
		{"id": "item-1", "type": "assistant", "text": "hello"},
		{"id": "item-2", "type": "userMessage", "text": "hi"},
	}

	first, firstStats, err := service.hydrateCodexHistoryItemTimestamps(sessionID, items)
	if err != nil {
		t.Fatalf("first hydrate: %v", err)
	}
	if firstStats.DaemonFilledCount != 2 {
		t.Fatalf("expected first hydrate to fill 2 daemon timestamps, got %+v", firstStats)
	}
	firstTS := []string{
		strings.TrimSpace(asString(first[0]["created_at"])),
		strings.TrimSpace(asString(first[1]["created_at"])),
	}
	if firstTS[0] == "" || firstTS[1] == "" {
		t.Fatalf("expected created_at timestamps on first hydrate, got %#v", first)
	}

	time.Sleep(2 * time.Millisecond)
	second, secondStats, err := service.hydrateCodexHistoryItemTimestamps(sessionID, items)
	if err != nil {
		t.Fatalf("second hydrate: %v", err)
	}
	if secondStats.CacheHitCount != 2 {
		t.Fatalf("expected second hydrate to hit cache for both items, got %+v", secondStats)
	}
	secondTS := []string{
		strings.TrimSpace(asString(second[0]["created_at"])),
		strings.TrimSpace(asString(second[1]["created_at"])),
	}
	if firstTS[0] != secondTS[0] || firstTS[1] != secondTS[1] {
		t.Fatalf("expected stable timestamps across reads, first=%v second=%v", firstTS, secondTS)
	}
}

func TestHydrateCodexHistoryItemTimestampsRespectsNativeTimestamp(t *testing.T) {
	service := &SessionService{
		manager: &SessionManager{baseDir: t.TempDir()},
		logger:  logging.Nop(),
	}
	items := []map[string]any{
		{
			"id":        "item-native",
			"type":      "assistant",
			"createdAt": "2026-02-18T14:00:00Z",
		},
	}

	hydrated, stats, err := service.hydrateCodexHistoryItemTimestamps("session-codex-native", items)
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if stats.NativeTimestampCount != 1 {
		t.Fatalf("expected native timestamp count to be 1, got %+v", stats)
	}
	if stats.DaemonFilledCount != 0 {
		t.Fatalf("expected daemon filled count to be 0, got %+v", stats)
	}
	if got := strings.TrimSpace(asString(hydrated[0]["created_at"])); got != "2026-02-18T14:00:00Z" {
		t.Fatalf("expected created_at to preserve native timestamp, got %q", got)
	}
}

func TestCodexHistoryItemTimestampKeyIgnoresTimestampFields(t *testing.T) {
	item := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "output_text", "text": "hello"},
			},
		},
	}
	keyBefore := codexHistoryItemTimestampKey(item)
	if keyBefore == "" {
		t.Fatalf("expected non-empty cache key before timestamp mutation")
	}

	mutated := cloneItemMap(item)
	mutated["created_at"] = "2026-02-18T15:00:00Z"
	mutated["timestamp"] = "2026-02-18T15:00:00Z"
	keyAfter := codexHistoryItemTimestampKey(mutated)
	if keyBefore != keyAfter {
		t.Fatalf("expected stable cache key after timestamp mutation, before=%q after=%q", keyBefore, keyAfter)
	}
}

func TestTailCodexThreadEmitsTimestampTelemetry(t *testing.T) {
	var logs bytes.Buffer
	thread := &codexThread{
		ID: "thread-telemetry",
		Turns: []codexTurn{
			{
				ID: "turn-1",
				Items: []map[string]any{
					{"id": "i1", "type": "userMessage", "text": "hello"},
					{"id": "i2", "type": "agentMessage", "text": "hi"},
				},
			},
		},
	}
	service := NewSessionService(
		&SessionManager{baseDir: t.TempDir()},
		nil,
		logging.New(&logs, logging.Info),
		WithCodexHistoryPool(&staticCodexHistoryPool{thread: thread}),
	)
	session := &types.Session{
		ID:        "session-codex-telemetry",
		Provider:  "codex",
		Cwd:       t.TempDir(),
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}

	first, err := service.tailCodexThread(context.Background(), session, thread.ID, 50)
	if err != nil {
		t.Fatalf("first tailCodexThread: %v", err)
	}
	second, err := service.tailCodexThread(context.Background(), session, thread.ID, 50)
	if err != nil {
		t.Fatalf("second tailCodexThread: %v", err)
	}
	if strings.TrimSpace(asString(first[0]["created_at"])) == "" || strings.TrimSpace(asString(second[0]["created_at"])) == "" {
		t.Fatalf("expected hydrated created_at values, first=%#v second=%#v", first, second)
	}
	if strings.TrimSpace(asString(first[0]["created_at"])) != strings.TrimSpace(asString(second[0]["created_at"])) {
		t.Fatalf("expected stable cached timestamp, first=%q second=%q", asString(first[0]["created_at"]), asString(second[0]["created_at"]))
	}

	out := logs.String()
	if strings.Count(out, "msg=codex_history_timestamp_stats") != 2 {
		t.Fatalf("expected telemetry log for each tail call, got %q", out)
	}
	if !strings.Contains(out, "daemon_filled_count=2") {
		t.Fatalf("expected daemon-filled telemetry in first call, got %q", out)
	}
	if !strings.Contains(out, "cache_hit_count=2") {
		t.Fatalf("expected cache-hit telemetry in second call, got %q", out)
	}
}

func TestFlattenCodexItemsAnnotatesTurnID(t *testing.T) {
	thread := &codexThread{
		ID: "thread-turn-map",
		Turns: []codexTurn{
			{
				ID: "turn-a",
				Items: []map[string]any{
					{"type": "userMessage", "content": []any{map[string]any{"type": "text", "text": "hello"}}},
					{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "hi"}}}},
				},
			},
		},
	}
	items := flattenCodexItems(thread)
	if len(items) != 2 {
		t.Fatalf("expected 2 flattened items, got %#v", items)
	}
	for i := range items {
		if got := strings.TrimSpace(asString(items[i]["turn_id"])); got != "turn-a" {
			t.Fatalf("expected flattened item %d to include turn_id turn-a, got %#v", i, items[i]["turn_id"])
		}
	}
}

type staticCodexHistoryPool struct {
	thread *codexThread
}

func (s *staticCodexHistoryPool) ReadThread(context.Context, string, string, string) (*codexThread, error) {
	if s == nil {
		return nil, nil
	}
	return s.thread, nil
}

func (s *staticCodexHistoryPool) Close() {}
