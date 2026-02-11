package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"control/internal/store"
	"control/internal/types"
)

func TestSessionServiceListWithMetaMigratesAndDedupesCodexAliases(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	internalID := "75925eb44c64e0717f145a33"
	threadID := "019c3f57-bd61-7bd3-8188-0d00f6122bb3"
	now := time.Now().UTC()

	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        internalID,
			Provider:  "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now.Add(-2 * time.Minute),
			Title:     "Internal session",
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert internal session: %v", err)
	}
	_, err = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        threadID,
			Provider:  "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now.Add(-1 * time.Minute),
			Title:     "Thread alias",
		},
		Source: sessionSourceCodex,
	})
	if err != nil {
		t.Fatalf("upsert codex thread session: %v", err)
	}
	lastActive := now.Add(-30 * time.Second)
	_, err = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:    internalID,
		ThreadID:     threadID,
		LastActiveAt: &lastActive,
	})
	if err != nil {
		t.Fatalf("upsert internal meta: %v", err)
	}
	_, err = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:    threadID,
		ThreadID:     threadID,
		LastActiveAt: &lastActive,
	})
	if err != nil {
		t.Fatalf("upsert codex meta: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)

	sessions, _, err := service.ListWithMeta(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after migration, got %d", len(sessions))
	}
	// After migration, the dual entries are reconciled under the thread ID.
	if sessions[0].ID != threadID {
		t.Fatalf("expected session to be reconciled under thread ID %q, got %q", threadID, sessions[0].ID)
	}
	// The internal session's title should be preserved.
	if sessions[0].Title != "Internal session" {
		t.Fatalf("expected internal session title to be preserved, got %q", sessions[0].Title)
	}

	// The old internal entry should be gone from the store.
	_, oldExists, _ := sessionStore.GetRecord(ctx, internalID)
	if oldExists {
		t.Fatalf("old internal session record should have been migrated away")
	}
}

func TestSessionServiceListWithMetaNormalizesDetachedActiveSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))

	sessionID := "2524b2a8dc9079f135cdf9fe"
	createdAt := time.Now().UTC()
	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "codex",
			Status:    types.SessionStatusRunning,
			PID:       4242,
			CreatedAt: createdAt,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert active session: %v", err)
	}

	service := NewSessionService(nil, &Stores{Sessions: sessionStore}, nil, nil)
	sessions, _, err := service.ListWithMeta(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Status != types.SessionStatusInactive {
		t.Fatalf("expected detached session to be inactive, got %s", sessions[0].Status)
	}
	if sessions[0].PID != 0 {
		t.Fatalf("expected detached session pid to be cleared, got %d", sessions[0].PID)
	}

	record, ok, err := sessionStore.GetRecord(ctx, sessionID)
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if !ok || record == nil || record.Session == nil {
		t.Fatalf("expected session record to exist")
	}
	if record.Session.Status != types.SessionStatusInactive {
		t.Fatalf("expected persisted session status to be inactive, got %s", record.Session.Status)
	}
}
