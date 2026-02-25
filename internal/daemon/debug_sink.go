package daemon

import (
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

type debugSink struct {
	mu      sync.Mutex
	batcher *debugBatcher
	factory *debugEventFactory
	writer  debugEventWriter
	store   debugEventStore
	bus     debugEventBus
}

func newDebugSink(path, sessionID, provider string, hub debugEventBus, buffer debugEventStore) (*debugSink, error) {
	return newDebugSinkWithPolicy(path, sessionID, provider, hub, buffer, defaultDebugBatchPolicy())
}

func newDebugSinkWithPolicy(path, sessionID, provider string, hub debugEventBus, buffer debugEventStore, policy DebugBatchPolicy) (*debugSink, error) {
	writer, err := newDebugJSONLWriter(path)
	if err != nil {
		return nil, err
	}
	if writer == nil {
		return nil, nil
	}
	return &debugSink{
		batcher: newDebugBatcher(policy),
		factory: newDebugEventFactory(sessionID, provider),
		writer:  writer,
		store:   buffer,
		bus:     hub,
	}, nil
}

func (s *debugSink) Write(stream string, data []byte) {
	if s == nil || len(data) == 0 {
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	batches := s.batcher.Append(stream, data, now)
	events := s.emitLocked(batches, now)
	s.mu.Unlock()
	s.publish(events)
}

func (s *debugSink) Close() {
	if s == nil {
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	events := s.emitLocked(s.batcher.Flush(now), now)
	if s.writer != nil {
		_ = s.writer.Close()
	}
	s.mu.Unlock()
	s.publish(events)
}

func (s *debugSink) emitLocked(batches []debugBatch, now time.Time) []types.DebugEvent {
	if len(batches) == 0 {
		return nil
	}
	events := make([]types.DebugEvent, 0, len(batches))
	for i := range batches {
		event := s.factory.Next(batches[i].stream, batches[i].data, now)
		if s.writer != nil {
			_ = s.writer.WriteEvent(event)
		}
		events = append(events, event)
	}
	return events
}

func (s *debugSink) publish(events []types.DebugEvent) {
	for i := range events {
		event := events[i]
		if s.store != nil {
			s.store.Append(event)
		}
		if s.bus != nil {
			s.bus.Broadcast(event)
		}
	}
}

func normalizeDebugStream(stream string) string {
	stream = strings.ToLower(strings.TrimSpace(stream))
	if stream == "" {
		return "stdout"
	}
	return stream
}
