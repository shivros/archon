package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

type recordingDebugEventStore struct {
	events []types.DebugEvent
}

func (s *recordingDebugEventStore) Append(event types.DebugEvent) {
	s.events = append(s.events, event)
}

type recordingDebugEventBus struct {
	events []types.DebugEvent
}

func (b *recordingDebugEventBus) Broadcast(event types.DebugEvent) {
	b.events = append(b.events, event)
}

type failingDebugEventWriter struct{}

func (f failingDebugEventWriter) WriteEvent(types.DebugEvent) error {
	return errors.New("write failed")
}
func (f failingDebugEventWriter) Close() error { return nil }

func TestDebugSinkBatchesChunksUntilFlushBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.jsonl")
	buffer := newDebugBufferWithBytes(16, 1024)
	sink, err := newDebugSinkWithPolicy(path, "s1", "codex", nil, buffer, DebugBatchPolicy{
		FlushInterval:  time.Hour,
		MaxBatchBytes:  4096,
		FlushOnNewline: true,
	})
	if err != nil {
		t.Fatalf("newDebugSink: %v", err)
	}

	sink.Write("stdout", []byte("a"))
	if got := buffer.Snapshot(0); len(got) != 0 {
		t.Fatalf("expected pending write to remain unflushed, got %#v", got)
	}

	sink.Write("stdout", []byte("b\n"))
	events := buffer.Snapshot(0)
	if len(events) != 1 {
		t.Fatalf("expected one batched event, got %d", len(events))
	}
	if events[0].Chunk != "ab\n" {
		t.Fatalf("expected merged chunk, got %q", events[0].Chunk)
	}
}

func TestDebugSinkCloseFlushesPendingChunks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.jsonl")
	buffer := newDebugBufferWithBytes(16, 1024)
	sink, err := newDebugSinkWithPolicy(path, "s1", "codex", nil, buffer, DebugBatchPolicy{
		FlushInterval:  time.Hour,
		MaxBatchBytes:  4096,
		FlushOnNewline: true,
	})
	if err != nil {
		t.Fatalf("newDebugSink: %v", err)
	}

	sink.Write("stdout", []byte("tail"))
	sink.Close()

	events := buffer.Snapshot(0)
	if len(events) != 1 || events[0].Chunk != "tail" {
		t.Fatalf("expected close flush to emit pending event, got %#v", events)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if lines := strings.Count(strings.TrimSpace(string(raw)), "\n") + 1; lines != 1 {
		t.Fatalf("expected one jsonl line, got %d (%q)", lines, string(raw))
	}
}

func TestNewDebugSinkWithPolicyPathValidation(t *testing.T) {
	sink, err := newDebugSinkWithPolicy("", "s1", "codex", nil, nil, defaultDebugBatchPolicy())
	if err != nil {
		t.Fatalf("expected empty path to return nil sink without error, got %v", err)
	}
	if sink != nil {
		t.Fatalf("expected nil sink for empty path, got %#v", sink)
	}

	dir := t.TempDir()
	invalidPath := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(invalidPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if _, err := newDebugSinkWithPolicy(invalidPath, "s1", "codex", nil, nil, defaultDebugBatchPolicy()); err == nil {
		t.Fatalf("expected error when path points to directory")
	}
}

func TestDebugSinkPublishesEventsEvenWhenWriterFails(t *testing.T) {
	store := &recordingDebugEventStore{}
	bus := &recordingDebugEventBus{}
	sink := &debugSink{
		batcher: newDebugBatcher(DebugBatchPolicy{
			FlushInterval:  time.Hour,
			MaxBatchBytes:  8,
			FlushOnNewline: true,
		}),
		factory: newDebugEventFactory("s1", "codex"),
		writer:  failingDebugEventWriter{},
		store:   store,
		bus:     bus,
	}

	sink.Write("stdout", []byte("line\n"))
	if len(store.events) != 1 {
		t.Fatalf("expected store append despite writer error, got %d events", len(store.events))
	}
	if len(bus.events) != 1 {
		t.Fatalf("expected bus broadcast despite writer error, got %d events", len(bus.events))
	}
}
