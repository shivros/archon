package daemon

import "sync"

type itemSubscriber struct {
	id int
	ch chan map[string]any
}

type itemHub struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]*itemSubscriber
}

func newItemHub() *itemHub {
	return &itemHub{
		subs: make(map[int]*itemSubscriber),
	}
}

func (h *itemHub) Add() (<-chan map[string]any, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	id := h.nextID
	ch := make(chan map[string]any, 256)
	h.subs[id] = &itemSubscriber{id: id, ch: ch}
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

func (h *itemHub) Broadcast(item map[string]any) {
	if item == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sub := range h.subs {
		select {
		case sub.ch <- item:
		default:
		}
	}
}
