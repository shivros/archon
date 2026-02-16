package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"control/internal/types"
)

func BenchmarkBboltSessionIndexListLarge(b *testing.B) {
	repo, err := NewBboltRepository(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatalf("NewBboltRepository: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()
	for i := 0; i < 5000; i++ {
		_, err := repo.SessionIndex().UpsertRecord(ctx, &types.SessionRecord{
			Session: &types.Session{
				ID:        fmt.Sprintf("s-%06d", i),
				Provider:  "codex",
				Status:    types.SessionStatusRunning,
				CreatedAt: time.Now().UTC().Add(-time.Duration(i) * time.Second),
			},
			Source: "internal",
		})
		if err != nil {
			b.Fatalf("seed session %d: %v", i, err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		records, err := repo.SessionIndex().ListRecords(ctx)
		if err != nil {
			b.Fatalf("ListRecords: %v", err)
		}
		if len(records) != 5000 {
			b.Fatalf("unexpected records length: %d", len(records))
		}
	}
}

func BenchmarkBboltNotesListLarge(b *testing.B) {
	repo, err := NewBboltRepository(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatalf("NewBboltRepository: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		_, err := repo.Notes().Upsert(ctx, &types.Note{
			Scope:       types.NoteScopeSession,
			WorkspaceID: "ws-1",
			SessionID:   fmt.Sprintf("s-%03d", i%100),
			Title:       fmt.Sprintf("note %d", i),
			Body:        "benchmark",
			Status:      types.NoteStatusIdea,
		})
		if err != nil {
			b.Fatalf("seed note %d: %v", i, err)
		}
	}

	filter := NoteFilter{SessionID: "s-042"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		notes, err := repo.Notes().List(ctx, filter)
		if err != nil {
			b.Fatalf("List notes: %v", err)
		}
		if len(notes) == 0 {
			b.Fatalf("expected filtered notes")
		}
	}
}
