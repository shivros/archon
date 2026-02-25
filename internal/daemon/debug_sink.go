package daemon

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

type debugSink struct {
	file      *os.File
	sessionID string
	provider  string
	mu        sync.Mutex
	seq       uint64
	hub       *debugHub
	buffer    *debugBuffer
}

func newDebugSink(path, sessionID, provider string, hub *debugHub, buffer *debugBuffer) (*debugSink, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &debugSink{
		file:      file,
		sessionID: strings.TrimSpace(sessionID),
		provider:  strings.TrimSpace(provider),
		hub:       hub,
		buffer:    buffer,
	}, nil
}

func (s *debugSink) Write(stream string, data []byte) {
	if s == nil || s.file == nil || len(data) == 0 {
		return
	}
	s.mu.Lock()
	s.seq++
	event := types.DebugEvent{
		Type:      "debug",
		SessionID: s.sessionID,
		Provider:  s.provider,
		Stream:    normalizeDebugStream(stream),
		Chunk:     string(data),
		TS:        time.Now().UTC().Format(time.RFC3339Nano),
		Seq:       s.seq,
	}
	encoded, err := json.Marshal(event)
	if err == nil {
		_, _ = s.file.Write(encoded)
		_, _ = s.file.Write([]byte("\n"))
	}
	s.mu.Unlock()
	if s.buffer != nil {
		s.buffer.Append(event)
	}
	if s.hub != nil {
		s.hub.Broadcast(event)
	}
}

func (s *debugSink) Close() {
	if s == nil || s.file == nil {
		return
	}
	_ = s.file.Close()
}

func normalizeDebugStream(stream string) string {
	stream = strings.ToLower(strings.TrimSpace(stream))
	if stream == "" {
		return "stdout"
	}
	return stream
}
