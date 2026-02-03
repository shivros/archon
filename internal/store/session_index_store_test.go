package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"control/internal/types"
)

func TestSessionIndexStoreUpsertList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions_index.json")
	store := NewFileSessionIndexStore(path)

	record := &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-1",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusInactive,
			CreatedAt: time.Now().UTC(),
		},
		Source: "codex",
	}

	_, err := store.UpsertRecord(context.Background(), record)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	list, err := store.ListRecords(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 record, got %d", len(list))
	}

	got, ok, err := store.GetRecord(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || got.Session == nil || got.Session.ID != "sess-1" {
		t.Fatalf("unexpected record")
	}

	if err := store.DeleteRecord(context.Background(), "sess-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}
