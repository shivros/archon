package daemon

import (
	"sync"

	"control/internal/types"
)

const debugMaxEvents = 2048
const debugMaxBufferedBytes = 512 * 1024

type debugSubscriber struct {
	id int
	ch chan types.DebugEvent
}

type debugHub struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]*debugSubscriber
}

func newDebugHub() *debugHub {
	return &debugHub{
		subs: make(map[int]*debugSubscriber),
	}
}

func (h *debugHub) Add() (<-chan types.DebugEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	id := h.nextID
	ch := make(chan types.DebugEvent, 256)
	h.subs[id] = &debugSubscriber{id: id, ch: ch}
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
	return ch, cancel
}

func (h *debugHub) Broadcast(event types.DebugEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sub := range h.subs {
		select {
		case sub.ch <- event:
		default:
		}
	}
}

type debugBuffer struct {
	mu         sync.Mutex
	events     []types.DebugEvent
	eventBytes []int
	max        int
	maxBytes   int
	totalBytes int
}

func newDebugBuffer(max int) *debugBuffer {
	return newDebugBufferWithPolicy(DebugRetentionPolicy{MaxEvents: max, MaxBytes: debugMaxBufferedBytes})
}

func newDebugBufferWithBytes(max, maxBytes int) *debugBuffer {
	return newDebugBufferWithPolicy(DebugRetentionPolicy{MaxEvents: max, MaxBytes: maxBytes})
}

func newDebugBufferWithPolicy(policy DebugRetentionPolicy) *debugBuffer {
	policy = policy.normalize()
	return &debugBuffer{
		events:     make([]types.DebugEvent, 0, policy.MaxEvents),
		eventBytes: make([]int, 0, policy.MaxEvents),
		max:        policy.MaxEvents,
		maxBytes:   policy.MaxBytes,
	}
}

func (b *debugBuffer) Append(event types.DebugEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	eventBytes := len(event.Chunk)
	b.events = append(b.events, event)
	b.eventBytes = append(b.eventBytes, eventBytes)
	b.totalBytes += eventBytes
	for {
		trimEvents := len(b.events) > b.max
		trimBytes := b.maxBytes > 0 && b.totalBytes > b.maxBytes
		if !trimEvents && !trimBytes {
			break
		}
		if len(b.events) == 0 {
			break
		}
		b.totalBytes -= b.eventBytes[0]
		b.events = b.events[1:]
		b.eventBytes = b.eventBytes[1:]
	}
}

func (b *debugBuffer) Snapshot(lines int) []types.DebugEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	if lines <= 0 || lines >= len(b.events) {
		out := make([]types.DebugEvent, len(b.events))
		copy(out, b.events)
		return out
	}
	start := len(b.events) - lines
	out := make([]types.DebugEvent, len(b.events[start:]))
	copy(out, b.events[start:])
	return out
}
