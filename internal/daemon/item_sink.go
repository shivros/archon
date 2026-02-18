package daemon

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type itemSink struct {
	file    *os.File
	mu      sync.Mutex
	hub     *itemHub
	metrics itemTimestampMetricsSink
}

func newItemSink(path string, hub *itemHub, metrics itemTimestampMetricsSink) (*itemSink, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &itemSink{file: file, hub: hub, metrics: metrics}, nil
}

func (s *itemSink) Append(item map[string]any) {
	if s == nil || s.file == nil || item == nil {
		return
	}
	prepared, classification := prepareItemForPersistenceWithClassification(item, time.Now().UTC())
	data, err := json.Marshal(prepared)
	if err != nil {
		return
	}
	s.mu.Lock()
	_, _ = s.file.Write(data)
	_, _ = s.file.Write([]byte("\n"))
	s.mu.Unlock()
	if s.hub != nil {
		s.hub.Broadcast(prepared)
	}
	if s.metrics != nil {
		s.metrics.Record(classification)
	}
}

func (s *itemSink) Close() {
	if s == nil || s.file == nil {
		return
	}
	if s.metrics != nil {
		s.metrics.Close()
	}
	_ = s.file.Close()
}
