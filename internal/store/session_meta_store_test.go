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

func TestSessionMetaStoreUpsertPreservesTitleLock(t *testing.T) {
	ctx := context.Background()
	store := NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))

	meta := &types.SessionMeta{SessionID: "s1", Title: "Custom", TitleLocked: true}
	if _, err := store.Upsert(ctx, meta); err != nil {
		t.Fatalf("upsert locked title: %v", err)
	}

	// Simulate an automated metadata update that should not clear lock state.
	if _, err := store.Upsert(ctx, &types.SessionMeta{SessionID: "s1", LastTurnID: "turn-1"}); err != nil {
		t.Fatalf("upsert metadata: %v", err)
	}

	loaded, ok, err := store.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || loaded == nil {
		t.Fatalf("expected meta")
	}
	if !loaded.TitleLocked {
		t.Fatalf("expected title lock to persist")
	}
	if loaded.Title != "Custom" {
		t.Fatalf("expected title to persist, got %q", loaded.Title)
	}
}
