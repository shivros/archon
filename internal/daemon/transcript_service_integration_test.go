package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
	"control/internal/store"
	"control/internal/types"
)

type fixedTranscriptMapper struct {
	itemEvents  []transcriptdomain.TranscriptEvent
	eventEvents []transcriptdomain.TranscriptEvent
}

func (m fixedTranscriptMapper) MapItem(string, transcriptadapters.MappingContext, map[string]any) []transcriptdomain.TranscriptEvent {
	out := make([]transcriptdomain.TranscriptEvent, len(m.itemEvents))
	copy(out, m.itemEvents)
	return out
}

func (m fixedTranscriptMapper) MapEvent(string, transcriptadapters.MappingContext, types.CodexEvent) []transcriptdomain.TranscriptEvent {
	out := make([]transcriptdomain.TranscriptEvent, len(m.eventEvents))
	copy(out, m.eventEvents)
	return out
}

type fixedTranscriptTransportSelector struct {
	transport transcriptTransport
	err       error
}

func (s fixedTranscriptTransportSelector) Select(context.Context, string, string) (transcriptTransport, error) {
	if s.err != nil {
		return transcriptTransport{}, s.err
	}
	return s.transport, nil
}

func TestSessionServiceGetTranscriptSnapshotFromHistoryLogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessionID := "sess-transcript"
	sessionsDir := filepath.Join(home, ".archon", "sessions", sessionID)
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "stdout.log"), []byte("hello from log\n"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "stderr.log"), []byte(""), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "custom",
			Cmd:       "custom",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	svc := NewSessionService(nil, &Stores{Sessions: index}, nil)
	snapshot, err := svc.GetTranscriptSnapshot(context.Background(), sessionID, 200)
	if err != nil {
		t.Fatalf("GetTranscriptSnapshot: %v", err)
	}
	if snapshot.SessionID != sessionID {
		t.Fatalf("expected session id %q, got %q", sessionID, snapshot.SessionID)
	}
	if len(snapshot.Blocks) == 0 {
		t.Fatalf("expected transcript blocks from history logs")
	}
	if snapshot.Revision.String() == "" {
		t.Fatalf("expected revision token")
	}
}

func TestSessionServiceGetTranscriptSnapshotRequiresSessionID(t *testing.T) {
	svc := NewSessionService(nil, nil, nil)
	if _, err := svc.GetTranscriptSnapshot(context.Background(), " ", 200); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestSessionServiceSubscribeTranscriptAcceptsLexicalAfterRevision(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s1",
			Provider:  "custom",
			Cmd:       "custom",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	events := make(chan types.CodexEvent)
	close(events)
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptMapper(fixedTranscriptMapper{}),
		WithTranscriptTransportSelector(fixedTranscriptTransportSelector{transport: transcriptTransport{eventsCh: events}}),
	)

	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s1", transcriptdomain.MustParseRevisionToken("rev-A"))
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()
	first, ok := <-ch
	if !ok {
		t.Fatalf("expected ready event")
	}
	if first.Kind != transcriptdomain.TranscriptEventStreamStatus {
		t.Fatalf("expected stream status event, got %q", first.Kind)
	}
	if _, err := transcriptdomain.ParseRevisionToken(first.Revision.String()); err != nil {
		t.Fatalf("expected valid revision token: %v", err)
	}
}

func TestSessionServiceSubscribeTranscriptUsesMapperAndSelector(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s2",
			Provider:  "custom",
			Cmd:       "custom",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	events := make(chan types.CodexEvent, 1)
	events <- types.CodexEvent{Method: "turn/started"}
	close(events)
	mapper := fixedTranscriptMapper{
		eventEvents: []transcriptdomain.TranscriptEvent{{
			Kind: transcriptdomain.TranscriptEventDelta,
			Delta: []transcriptdomain.Block{{
				Kind: "assistant",
				Text: "mapped",
			}},
		}},
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptMapper(mapper),
		WithTranscriptTransportSelector(fixedTranscriptTransportSelector{transport: transcriptTransport{eventsCh: events}}),
	)

	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s2", "")
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()

	seenDelta := false
	for event := range ch {
		if event.Kind == transcriptdomain.TranscriptEventDelta {
			seenDelta = true
			break
		}
	}
	if !seenDelta {
		t.Fatalf("expected mapped delta event to flow through transcript stream")
	}
}

func TestSessionServiceSubscribeTranscriptTransportError(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{ID: "s3", Provider: "custom", Cmd: "custom", Status: types.SessionStatusInactive, CreatedAt: now},
		Source:  sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptTransportSelector(fixedTranscriptTransportSelector{err: invalidError("boom", nil)}),
	)
	if _, _, err := svc.SubscribeTranscript(context.Background(), "s3", ""); err == nil {
		t.Fatalf("expected transport selection error")
	}
}

func TestSessionServiceProviderSupportsTranscriptStreaming(t *testing.T) {
	svc := NewSessionService(nil, nil, nil)
	if !svc.providerSupportsTranscriptStreaming("codex") {
		t.Fatalf("expected codex to support transcript streaming")
	}
	if svc.providerSupportsTranscriptStreaming("custom") {
		t.Fatalf("expected custom to not support transcript streaming")
	}
}

func TestSessionServiceGetTranscriptSnapshotMappingProducesValidJSON(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{ID: "s4", Provider: "custom", Cmd: "custom", Status: types.SessionStatusInactive, CreatedAt: now},
		Source:  sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionDir := filepath.Join(home, ".archon", "sessions", "s4")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "stdout.log"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "stderr.log"), []byte(""), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil)
	snapshot, err := svc.GetTranscriptSnapshot(context.Background(), "s4", 10)
	if err != nil {
		t.Fatalf("GetTranscriptSnapshot: %v", err)
	}
	if _, err := json.Marshal(snapshot); err != nil {
		t.Fatalf("expected snapshot JSON marshal to succeed: %v", err)
	}
}
