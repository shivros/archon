package daemon

import (
	"path/filepath"
	"testing"
	"time"

	"control/internal/types"
)

func TestDebugBatcherRespectsFlushPolicy(t *testing.T) {
	batcher := newDebugBatcher(DebugBatchPolicy{
		FlushInterval:  50 * time.Millisecond,
		MaxBatchBytes:  16,
		FlushOnNewline: true,
	})
	now := time.Now()
	if batches := batcher.Append("stdout", []byte("abc"), now); len(batches) != 0 {
		t.Fatalf("expected no flush before boundary, got %#v", batches)
	}
	if batches := batcher.Flush(now.Add(60 * time.Millisecond)); len(batches) != 1 {
		t.Fatalf("expected interval flush, got %#v", batches)
	}
}

func TestDebugBatcherFlushesOnSizeBoundary(t *testing.T) {
	batcher := newDebugBatcher(DebugBatchPolicy{
		FlushInterval:  time.Hour,
		MaxBatchBytes:  4,
		FlushOnNewline: true,
	})
	now := time.Now()
	batches := batcher.Append("stdout", []byte("abcd"), now)
	if len(batches) != 1 {
		t.Fatalf("expected size-triggered flush, got %#v", batches)
	}
	if string(batches[0].data) != "abcd" {
		t.Fatalf("unexpected batch payload %q", string(batches[0].data))
	}
}

func TestDebugBatcherHonorsFlushOnNewlineToggle(t *testing.T) {
	batcher := newDebugBatcher(DebugBatchPolicy{
		FlushInterval:  time.Hour,
		MaxBatchBytes:  64,
		FlushOnNewline: false,
	})
	now := time.Now()
	batches := batcher.Append("stdout", []byte("line\n"), now)
	if len(batches) != 0 {
		t.Fatalf("expected newline not to flush when disabled, got %#v", batches)
	}
	batches = batcher.Flush(now)
	if len(batches) != 1 {
		t.Fatalf("expected force flush to emit pending batch, got %#v", batches)
	}
}

func TestDebugBatcherSeparatesStreams(t *testing.T) {
	batcher := newDebugBatcher(DebugBatchPolicy{
		FlushInterval:  time.Hour,
		MaxBatchBytes:  64,
		FlushOnNewline: true,
	})
	now := time.Now()
	_ = batcher.Append("stdout", []byte("a"), now)
	batches := batcher.Append("stderr", []byte("b\n"), now)
	if len(batches) != 1 {
		t.Fatalf("expected one flushed stream, got %#v", batches)
	}
	if batches[0].stream != "stderr" || string(batches[0].data) != "b\n" {
		t.Fatalf("unexpected flushed stream payload %#v", batches[0])
	}
	if forced := batcher.Flush(now); len(forced) != 1 || forced[0].stream != "stdout" {
		t.Fatalf("expected force flush to emit remaining stdout batch, got %#v", forced)
	}
}

func TestDebugBatcherNilAndEmptyInputs(t *testing.T) {
	var nilBatcher *debugBatcher
	if got := nilBatcher.Append("stdout", []byte("x"), time.Now()); got != nil {
		t.Fatalf("expected nil batcher append to return nil, got %#v", got)
	}
	if got := nilBatcher.Flush(time.Now()); got != nil {
		t.Fatalf("expected nil batcher flush to return nil, got %#v", got)
	}

	batcher := newDebugBatcher(defaultDebugBatchPolicy())
	if got := batcher.Append("stdout", nil, time.Now()); got != nil {
		t.Fatalf("expected nil data append to return nil, got %#v", got)
	}
}

func TestDebugJSONLWriterWriteAndCloseBehavior(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.jsonl")
	writer, err := newDebugJSONLWriter(path)
	if err != nil {
		t.Fatalf("newDebugJSONLWriter: %v", err)
	}
	event := types.DebugEvent{Type: "debug", SessionID: "s1", Chunk: "line"}
	if err := writer.WriteEvent(event); err != nil {
		t.Fatalf("WriteEvent: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := writer.WriteEvent(event); err == nil {
		t.Fatalf("expected write after close to fail")
	}
}
