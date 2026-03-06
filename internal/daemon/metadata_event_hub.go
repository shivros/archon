package daemon

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

const (
	metadataEventSubscriberBuffer = 128
	metadataEventReplayLimit      = 512
)

var errInvalidAfterRevision = errors.New("invalid after_revision")

type metadataEventSubscriber struct {
	id int
	ch chan types.MetadataEvent
}

type metadataEventHub struct {
	mu           sync.Mutex
	nextID       int
	nextRevision uint64
	subs         map[int]*metadataEventSubscriber
	replay       []types.MetadataEvent
	logger       logging.Logger
}

func newMetadataEventHub(logger logging.Logger) *metadataEventHub {
	if logger == nil {
		logger = logging.Nop()
	}
	return &metadataEventHub{
		subs:   make(map[int]*metadataEventSubscriber),
		replay: make([]types.MetadataEvent, 0, metadataEventReplayLimit),
		logger: logger,
	}
}

func parseMetadataAfterRevision(raw string) (uint64, bool, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return 0, false, nil
	}
	seq, err := strconv.ParseUint(token, 10, 64)
	if err != nil {
		return 0, false, errInvalidAfterRevision
	}
	return seq, true, nil
}

func (h *metadataEventHub) Subscribe(afterRevision string) (<-chan types.MetadataEvent, func(), error) {
	if h == nil {
		return nil, nil, nil
	}
	afterSeq, hasAfter, err := parseMetadataAfterRevision(afterRevision)
	if err != nil {
		return nil, nil, err
	}

	ch := make(chan types.MetadataEvent, metadataEventSubscriberBuffer)

	h.mu.Lock()
	h.nextID++
	id := h.nextID
	h.subs[id] = &metadataEventSubscriber{id: id, ch: ch}

	backlog := make([]types.MetadataEvent, 0, len(h.replay))
	for _, event := range h.replay {
		if !hasAfter {
			backlog = append(backlog, event)
			continue
		}
		seq, ok := metadataRevisionSequence(event.Revision)
		if !ok {
			continue
		}
		if seq > afterSeq {
			backlog = append(backlog, event)
		}
	}
	h.mu.Unlock()

	for _, event := range backlog {
		select {
		case ch <- event:
		default:
			if h.logger != nil {
				h.logger.Warn("metadata_event_subscriber_backlog_dropped",
					logging.F("subscriber_id", id),
					logging.F("after_revision", strings.TrimSpace(afterRevision)),
				)
			}
			return ch, func() {
				h.mu.Lock()
				sub, ok := h.subs[id]
				if ok {
					delete(h.subs, id)
				}
				h.mu.Unlock()
				if ok {
					close(sub.ch)
				}
			}, nil
		}
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

func (h *metadataEventHub) PublishMetadataEvent(event types.MetadataEvent) {
	if h == nil {
		return
	}
	normalized := normalizeMetadataEvent(event)
	if normalized.Type == "" {
		return
	}

	h.mu.Lock()
	h.nextRevision++
	normalized.Revision = strconv.FormatUint(h.nextRevision, 10)
	if normalized.OccurredAt.IsZero() {
		normalized.OccurredAt = time.Now().UTC()
	}
	normalized.Session = normalizeMetadataEntityUpdated(normalized.Session, normalized.Revision, normalized.OccurredAt)
	normalized.Workflow = normalizeMetadataEntityUpdated(normalized.Workflow, normalized.Revision, normalized.OccurredAt)

	if len(h.replay) >= metadataEventReplayLimit {
		h.replay = append(h.replay[1:], normalized)
	} else {
		h.replay = append(h.replay, normalized)
	}

	dropped := 0
	for _, sub := range h.subs {
		select {
		case sub.ch <- normalized:
		default:
			dropped++
		}
	}
	h.mu.Unlock()

	if dropped > 0 && h.logger != nil {
		h.logger.Warn("metadata_event_subscriber_drop",
			logging.F("event_type", normalized.Type),
			logging.F("revision", normalized.Revision),
			logging.F("dropped_subscribers", dropped),
		)
	}
}

func normalizeMetadataEvent(event types.MetadataEvent) types.MetadataEvent {
	out := event
	out.Version = strings.TrimSpace(out.Version)
	if out.Version == "" {
		out.Version = types.MetadataEventSchemaVersionV1
	}
	out.Type = strings.TrimSpace(out.Type)
	if !out.OccurredAt.IsZero() {
		out.OccurredAt = out.OccurredAt.UTC()
	}
	return out
}

func normalizeMetadataEntityUpdated(entity *types.MetadataEntityUpdated, revision string, now time.Time) *types.MetadataEntityUpdated {
	if entity == nil {
		return nil
	}
	copy := *entity
	copy.ID = strings.TrimSpace(copy.ID)
	copy.Title = strings.TrimSpace(copy.Title)
	if copy.Revision == "" {
		copy.Revision = revision
	}
	if copy.UpdatedAt.IsZero() {
		copy.UpdatedAt = now
	}
	copy.UpdatedAt = copy.UpdatedAt.UTC()
	return &copy
}

func metadataRevisionSequence(revision string) (uint64, bool) {
	seq, err := strconv.ParseUint(strings.TrimSpace(revision), 10, 64)
	if err != nil {
		return 0, false
	}
	return seq, true
}
