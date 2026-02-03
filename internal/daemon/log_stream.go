package daemon

import (
	"errors"
	"sync"
	"time"

	"control/internal/types"
)

const (
	logChunkBytes     = 16 * 1024
	logBufferMaxBytes = 1 * 1024 * 1024
	logFlushInterval  = 150 * time.Millisecond
	maxChunksPerFlush = 8
)

type logBuffer struct {
	mu       sync.Mutex
	buf      []byte
	maxBytes int
}

func newLogBuffer(maxBytes int) *logBuffer {
	if maxBytes <= 0 {
		maxBytes = logBufferMaxBytes
	}
	return &logBuffer{
		buf:      make([]byte, 0, maxBytes),
		maxBytes: maxBytes,
	}
}

func (b *logBuffer) Append(p []byte) {
	if len(p) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(p) >= b.maxBytes {
		b.buf = append(b.buf[:0], p[len(p)-b.maxBytes:]...)
		return
	}

	overflow := len(b.buf) + len(p) - b.maxBytes
	if overflow > 0 {
		b.buf = b.buf[overflow:]
	}
	b.buf = append(b.buf, p...)
}

func (b *logBuffer) Drain(max int) []byte {
	if max <= 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.buf) == 0 {
		return nil
	}
	if max > len(b.buf) {
		max = len(b.buf)
	}
	out := make([]byte, max)
	copy(out, b.buf[:max])
	b.buf = b.buf[max:]
	return out
}

func (b *logBuffer) Clear() {
	b.mu.Lock()
	b.buf = b.buf[:0]
	b.mu.Unlock()
}

func (b *logBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.buf)
}

type bufferWriter struct {
	buffer *logBuffer
}

func (w *bufferWriter) Write(p []byte) (int, error) {
	if w.buffer == nil {
		return len(p), nil
	}
	w.buffer.Append(p)
	return len(p), nil
}

type subscriber struct {
	id     int
	stream string
	ch     chan types.LogEvent
}

type subscriberHub struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]*subscriber
}

func newSubscriberHub() *subscriberHub {
	return &subscriberHub{
		subs: make(map[int]*subscriber),
	}
}

func (h *subscriberHub) Add(stream string) (<-chan types.LogEvent, func(), error) {
	if stream == "" {
		stream = "combined"
	}
	if stream != "combined" && stream != "stdout" && stream != "stderr" {
		return nil, nil, errors.New("invalid stream")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	id := h.nextID
	ch := make(chan types.LogEvent, 256)
	h.subs[id] = &subscriber{
		id:     id,
		stream: stream,
		ch:     ch,
	}
	cancel := func() {
		h.mu.Lock()
		sub, ok := h.subs[id]
		if ok {
			delete(h.subs, id)
		}
		h.mu.Unlock()
		if ok {
			close(sub.ch)
		}
	}
	return ch, cancel, nil
}

func (h *subscriberHub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

func (h *subscriberHub) Broadcast(event types.LogEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sub := range h.subs {
		if sub.stream != "combined" && sub.stream != event.Stream {
			continue
		}
		select {
		case sub.ch <- event:
		default:
		}
	}
}
