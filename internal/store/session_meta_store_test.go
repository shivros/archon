package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

func TestSessionMetaStoreUpsertPreservesDismissedAt(t *testing.T) {
	ctx := context.Background()
	store := NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))

	dismissedAt := time.Now().UTC().Add(-5 * time.Minute)
	meta := &types.SessionMeta{SessionID: "s1", DismissedAt: &dismissedAt}
	if _, err := store.Upsert(ctx, meta); err != nil {
		t.Fatalf("upsert dismissed meta: %v", err)
	}

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
	if loaded.DismissedAt == nil || !loaded.DismissedAt.Equal(dismissedAt) {
		t.Fatalf("expected dismissed_at to persist, got %#v", loaded.DismissedAt)
	}
}

func TestSessionMetaStoreUpsertClearsDismissedAtWithZeroTime(t *testing.T) {
	ctx := context.Background()
	store := NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))

	dismissedAt := time.Now().UTC().Add(-time.Minute)
	if _, err := store.Upsert(ctx, &types.SessionMeta{SessionID: "s1", DismissedAt: &dismissedAt}); err != nil {
		t.Fatalf("upsert dismissed meta: %v", err)
	}

	clear := time.Time{}
	if _, err := store.Upsert(ctx, &types.SessionMeta{SessionID: "s1", DismissedAt: &clear}); err != nil {
		t.Fatalf("clear dismissed meta: %v", err)
	}

	loaded, ok, err := store.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || loaded == nil {
		t.Fatalf("expected meta")
	}
	if loaded.DismissedAt != nil {
		t.Fatalf("expected dismissed_at to clear, got %#v", loaded.DismissedAt)
	}
}

func TestSessionMetaStoreUpsertPreservesWorkflowRunID(t *testing.T) {
	ctx := context.Background()
	store := NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))

	if _, err := store.Upsert(ctx, &types.SessionMeta{
		SessionID:     "s1",
		WorkflowRunID: "gwf-1",
	}); err != nil {
		t.Fatalf("upsert workflow-owned meta: %v", err)
	}

	if _, err := store.Upsert(ctx, &types.SessionMeta{
		SessionID:  "s1",
		LastTurnID: "turn-1",
	}); err != nil {
		t.Fatalf("upsert metadata: %v", err)
	}

	loaded, ok, err := store.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || loaded == nil {
		t.Fatalf("expected meta")
	}
	if loaded.WorkflowRunID != "gwf-1" {
		t.Fatalf("expected workflow_run_id to persist, got %q", loaded.WorkflowRunID)
	}
}
