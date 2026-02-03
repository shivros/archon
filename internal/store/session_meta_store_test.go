package store

import (
	"context"
	"path/filepath"
	"testing"

	"control/internal/types"
)

func TestSessionMetaStoreUpsert(t *testing.T) {
	ctx := context.Background()
	store := NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))

	meta := &types.SessionMeta{SessionID: "s1", Title: "First"}
	if _, err := store.Upsert(ctx, meta); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	loaded, ok, err := store.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("expected meta")
	}
	if loaded.Title != "First" {
		t.Fatalf("expected title")
	}

	meta.Title = "Updated"
	if _, err := store.Upsert(ctx, meta); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	loaded, ok, _ = store.Get(ctx, "s1")
	if !ok || loaded.Title != "Updated" {
		t.Fatalf("expected updated title")
	}
}
