package daemon

import (
	"context"
	"errors"
	"sync"

	"control/internal/types"
)

var errFileSearchHubNotFound = errors.New("file search not found")

type FileSearchHub interface {
	Register(searchID string, session *types.FileSearchSession, runtime FileSearchRuntime) error
	Lookup(searchID string) (*types.FileSearchSession, FileSearchRuntime, error)
	Publish(searchID string, session *types.FileSearchSession, event types.FileSearchEvent, terminal bool) error
	Subscribe(ctx context.Context, id string) (<-chan types.FileSearchEvent, func(), error)
}

type fileSearchHubEntry struct {
	session        *types.FileSearchSession
	runtime        FileSearchRuntime
	subscribers    map[int]chan types.FileSearchEvent
	nextSubscriber int
}

type memoryFileSearchHub struct {
	mu      sync.Mutex
	entries map[string]*fileSearchHubEntry
}

func NewMemoryFileSearchHub() FileSearchHub {
	return &memoryFileSearchHub{
		entries: map[string]*fileSearchHubEntry{},
	}
}

func (h *memoryFileSearchHub) Register(searchID string, session *types.FileSearchSession, runtime FileSearchRuntime) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries[searchID] = &fileSearchHubEntry{
		session:     cloneFileSearchSession(session),
		runtime:     runtime,
		subscribers: map[int]chan types.FileSearchEvent{},
	}
	return nil
}

func (h *memoryFileSearchHub) Lookup(searchID string) (*types.FileSearchSession, FileSearchRuntime, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.entries[searchID]
	if !ok || entry == nil {
		return nil, nil, errFileSearchHubNotFound
	}
	return cloneFileSearchSession(entry.session), entry.runtime, nil
}

func (h *memoryFileSearchHub) Publish(searchID string, session *types.FileSearchSession, event types.FileSearchEvent, terminal bool) error {
	h.mu.Lock()
	entry, ok := h.entries[searchID]
	if !ok || entry == nil {
		h.mu.Unlock()
		return errFileSearchHubNotFound
	}
	entry.session = cloneFileSearchSession(session)
	subscribers := entrySubscriberChannels(entry)
	if terminal {
		delete(h.entries, searchID)
	}
	h.mu.Unlock()

	broadcastFileSearchEvent(subscribers, event)
	if terminal {
		closeFileSearchSubscribers(subscribers)
	}
	return nil
}

func (h *memoryFileSearchHub) Subscribe(ctx context.Context, id string) (<-chan types.FileSearchEvent, func(), error) {
	h.mu.Lock()
	entry, ok := h.entries[id]
	if !ok || entry == nil {
		h.mu.Unlock()
		return nil, nil, errFileSearchHubNotFound
	}
	if entry.subscribers == nil {
		entry.subscribers = map[int]chan types.FileSearchEvent{}
	}
	entry.nextSubscriber++
	subID := entry.nextSubscriber
	ch := make(chan types.FileSearchEvent, 32)
	entry.subscribers[subID] = ch
	h.mu.Unlock()

	cancel := func() {
		h.unsubscribe(id, subID)
	}
	if ctx != nil {
		go func() {
			<-ctx.Done()
			cancel()
		}()
	}
	return ch, cancel, nil
}

func (h *memoryFileSearchHub) unsubscribe(searchID string, subscriberID int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.entries[searchID]
	if !ok || entry == nil || entry.subscribers == nil {
		return
	}
	ch, ok := entry.subscribers[subscriberID]
	if !ok {
		return
	}
	delete(entry.subscribers, subscriberID)
	close(ch)
}

func isFileSearchHubNotFound(err error) bool {
	return errors.Is(err, errFileSearchHubNotFound)
}

func entrySubscriberChannels(entry *fileSearchHubEntry) []chan types.FileSearchEvent {
	if entry == nil || len(entry.subscribers) == 0 {
		return nil
	}
	channels := make([]chan types.FileSearchEvent, 0, len(entry.subscribers))
	for _, ch := range entry.subscribers {
		if ch != nil {
			channels = append(channels, ch)
		}
	}
	return channels
}
