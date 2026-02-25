package daemon

import (
	"sync"

	"control/internal/types"
)

const debugMaxEvents = 2048

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
	mu     sync.Mutex
	events []types.DebugEvent
	max    int
}

func newDebugBuffer(max int) *debugBuffer {
	if max <= 0 {
		max = debugMaxEvents
	}
	return &debugBuffer{
		events: make([]types.DebugEvent, 0, max),
		max:    max,
	}
}

func (b *debugBuffer) Append(event types.DebugEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) == b.max {
		copy(b.events, b.events[1:])
		b.events = b.events[:b.max-1]
	}
	b.events = append(b.events, event)
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
