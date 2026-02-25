package daemon

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"time"

	"control/internal/types"
)

type debugEventWriter interface {
	WriteEvent(event types.DebugEvent) error
	Close() error
}

type debugJSONLWriter struct {
	file *os.File
}

func newDebugJSONLWriter(path string) (*debugJSONLWriter, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &debugJSONLWriter{file: file}, nil
}

func (w *debugJSONLWriter) WriteEvent(event types.DebugEvent) error {
	if w == nil || w.file == nil {
		return nil
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := w.file.Write(encoded); err != nil {
		return err
	}
	_, err = w.file.Write([]byte("\n"))
	return err
}

func (w *debugJSONLWriter) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	return w.file.Close()
}

type debugEventFactory struct {
	sessionID string
	provider  string
	seq       uint64
}

func newDebugEventFactory(sessionID, provider string) *debugEventFactory {
	return &debugEventFactory{
		sessionID: strings.TrimSpace(sessionID),
		provider:  strings.TrimSpace(provider),
	}
}

func (f *debugEventFactory) Next(stream string, chunk []byte, now time.Time) types.DebugEvent {
	f.seq++
	return types.DebugEvent{
		Type:      "debug",
		SessionID: f.sessionID,
		Provider:  f.provider,
		Stream:    normalizeDebugStream(stream),
		Chunk:     string(chunk),
		TS:        now.UTC().Format(time.RFC3339Nano),
		Seq:       f.seq,
	}
}

type debugBatch struct {
	stream string
	data   []byte
}

type debugBatcher struct {
	policy  DebugBatchPolicy
	pending map[string]pendingDebugChunk
}

type pendingDebugChunk struct {
	data      []byte
	firstSeen time.Time
}

func newDebugBatcher(policy DebugBatchPolicy) *debugBatcher {
	return &debugBatcher{
		policy:  policy.normalize(),
		pending: make(map[string]pendingDebugChunk, 3),
	}
}

func (b *debugBatcher) Append(stream string, data []byte, now time.Time) []debugBatch {
	if b == nil || len(data) == 0 {
		return nil
	}
	stream = normalizeDebugStream(stream)
	pending := b.pending[stream]
	pending.data = append(pending.data, data...)
	if pending.firstSeen.IsZero() {
		pending.firstSeen = now
	}
	b.pending[stream] = pending
	return b.collect(now, false)
}

func (b *debugBatcher) Flush(now time.Time) []debugBatch {
	if b == nil {
		return nil
	}
	return b.collect(now, true)
}

func (b *debugBatcher) collect(now time.Time, force bool) []debugBatch {
	batches := make([]debugBatch, 0, len(b.pending))
	for stream, pending := range b.pending {
		if len(pending.data) == 0 {
			delete(b.pending, stream)
			continue
		}
		flush := force || (b.policy.FlushOnNewline && bytes.Contains(pending.data, []byte{'\n'}))
		if !flush && b.policy.MaxBatchBytes > 0 {
			flush = len(pending.data) >= b.policy.MaxBatchBytes
		}
		if !flush && b.policy.FlushInterval > 0 && !pending.firstSeen.IsZero() {
			flush = now.Sub(pending.firstSeen) >= b.policy.FlushInterval
		}
		if !flush {
			continue
		}
		batches = append(batches, debugBatch{stream: stream, data: pending.data})
		delete(b.pending, stream)
	}
	return batches
}
