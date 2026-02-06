package daemon

import (
	"encoding/json"
	"os"
	"sync"
)

type itemSink struct {
	file *os.File
	mu   sync.Mutex
	hub  *itemHub
}

func newItemSink(path string, hub *itemHub) (*itemSink, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &itemSink{file: file, hub: hub}, nil
}

func (s *itemSink) Append(item map[string]any) {
	if s == nil || s.file == nil || item == nil {
		return
	}
	data, err := json.Marshal(item)
	if err != nil {
		return
	}
	s.mu.Lock()
	_, _ = s.file.Write(data)
	_, _ = s.file.Write([]byte("\n"))
	s.mu.Unlock()
	if s.hub != nil {
		s.hub.Broadcast(item)
	}
}

func (s *itemSink) Close() {
	if s == nil || s.file == nil {
		return
	}
	_ = s.file.Close()
}
