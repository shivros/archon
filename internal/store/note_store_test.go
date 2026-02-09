package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"control/internal/types"
)

func TestNoteStoreListEmpty(t *testing.T) {
	store := NewFileNoteStore(filepath.Join(t.TempDir(), "notes.json"))
	notes, err := store.List(context.Background(), NoteFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("expected empty notes, got %d", len(notes))
	}
}

func TestNoteStoreCRUDAndFilter(t *testing.T) {
	ctx := context.Background()
	store := NewFileNoteStore(filepath.Join(t.TempDir(), "notes.json"))

	created, err := store.Upsert(ctx, &types.Note{
		Kind:        types.NoteKindPin,
		Scope:       types.NoteScopeSession,
		WorkspaceID: "ws1",
		SessionID:   "s1",
		Body:        "important snippet",
		Tags:        []string{"mvp", "idea"},
		Source: &types.NoteSource{
			SessionID: "s1",
			BlockID:   "agent-1",
			Role:      "assistant",
			Snippet:   "Use versioned schema",
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected id")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be set")
	}

	got, ok, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || got == nil {
		t.Fatalf("expected note to exist")
	}
	if got.Source == nil || got.Source.Snippet == "" {
		t.Fatalf("expected source metadata")
	}

	got.Tags[0] = "mutated"
	again, ok, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("second get: %v", err)
	}
	if !ok || again == nil {
		t.Fatalf("expected note")
	}
	if again.Tags[0] != "mvp" {
		t.Fatalf("expected clone semantics, got %q", again.Tags[0])
	}

	createdAt := created.CreatedAt
	time.Sleep(10 * time.Millisecond)
	updated, err := store.Upsert(ctx, &types.Note{
		ID:          created.ID,
		Kind:        types.NoteKindNote,
		Scope:       types.NoteScopeWorkspace,
		WorkspaceID: "ws1",
		Title:       "Refactor later",
		Body:        "move note to workspace",
		Status:      types.NoteStatusTodo,
		Tags:        []string{"todo"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created_at to remain unchanged")
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("expected updated_at to advance")
	}

	byScope, err := store.List(ctx, NoteFilter{Scope: types.NoteScopeWorkspace})
	if err != nil {
		t.Fatalf("list by scope: %v", err)
	}
	if len(byScope) != 1 || byScope[0].ID != created.ID {
		t.Fatalf("unexpected scope filter result: %#v", byScope)
	}

	bySession, err := store.List(ctx, NoteFilter{SessionID: "s1"})
	if err != nil {
		t.Fatalf("list by session: %v", err)
	}
	if len(bySession) != 0 {
		t.Fatalf("expected no session-scoped notes after move, got %d", len(bySession))
	}

	if err := store.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, ok, err = store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if ok {
		t.Fatalf("expected note to be deleted")
	}

	err = store.Delete(ctx, created.ID)
	if !errors.Is(err, ErrNoteNotFound) {
		t.Fatalf("expected ErrNoteNotFound, got %v", err)
	}
}
