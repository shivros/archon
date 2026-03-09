package daemon

import (
	"context"
	"strings"
	"sync"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

const (
	defaultTranscriptHubSubscriberBuffer = 256
)

var transcriptMappingBaseRevision = transcriptdomain.MustParseRevisionToken("1")

type defaultTranscriptProjectorFactory struct{}

func NewDefaultTranscriptProjectorFactory() TranscriptProjectorFactory {
	return defaultTranscriptProjectorFactory{}
}

func (defaultTranscriptProjectorFactory) New(
	sessionID, provider string,
	base transcriptdomain.RevisionToken,
) TranscriptProjector {
	return NewTranscriptProjector(sessionID, provider, base)
}

type canonicalTranscriptHub struct {
	sessionID string
	provider  string

	ingressFactory   TranscriptIngressFactory
	mapper           TranscriptMapper
	projectorFactory TranscriptProjectorFactory

	startMu          sync.Mutex
	started          bool
	explicitlyClosed bool
	runCancel        context.CancelFunc
	done             chan struct{}

	stateMu  sync.RWMutex
	snapshot transcriptdomain.TranscriptSnapshot
	revision transcriptdomain.RevisionToken

	fanoutMu       sync.Mutex
	subscribers    map[int]chan transcriptdomain.TranscriptEvent
	nextSubscriber int
}

func newCanonicalTranscriptHub(
	sessionID, provider string,
	ingressFactory TranscriptIngressFactory,
	mapper TranscriptMapper,
	projectorFactory TranscriptProjectorFactory,
) (*canonicalTranscriptHub, error) {
	sessionID = strings.TrimSpace(sessionID)
	provider = normalizeTranscriptProvider(provider)
	if sessionID == "" {
		return nil, invalidError("session id is required", nil)
	}
	if provider == "" {
		return nil, invalidError("provider is required", nil)
	}
	if mapper == nil {
		mapper = NewDefaultTranscriptMapper(nil)
	}
	if projectorFactory == nil {
		projectorFactory = NewDefaultTranscriptProjectorFactory()
	}
	return &canonicalTranscriptHub{
		sessionID:        sessionID,
		provider:         provider,
		ingressFactory:   ingressFactory,
		mapper:           mapper,
		projectorFactory: projectorFactory,
		subscribers:      map[int]chan transcriptdomain.TranscriptEvent{},
		nextSubscriber:   1,
	}, nil
}

func (h *canonicalTranscriptHub) Subscribe(
	ctx context.Context,
	after transcriptdomain.RevisionToken,
) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	if h == nil {
		return nil, nil, unavailableError("canonical transcript hub unavailable", nil)
	}
	ch := make(chan transcriptdomain.TranscriptEvent, defaultTranscriptHubSubscriberBuffer)
	h.fanoutMu.Lock()
	subscriberID := h.nextSubscriber
	h.nextSubscriber++

	snapshot, revision := h.snapshotAndRevision()
	requiresReplace := shouldReplaySnapshot(after, revision)
	if requiresReplace && !revision.IsZero() {
		replace := transcriptdomain.TranscriptEvent{
			Kind:      transcriptdomain.TranscriptEventReplace,
			SessionID: h.sessionID,
			Provider:  h.provider,
			Revision:  revision,
			Replace:   &snapshot,
		}
		if err := transcriptdomain.ValidateEvent(replace); err == nil {
			select {
			case ch <- replace:
			default:
				close(ch)
				h.fanoutMu.Unlock()
				return nil, nil, unavailableError("subscriber buffer saturated during replay", nil)
			}
		}
	}

	h.subscribers[subscriberID] = ch
	h.fanoutMu.Unlock()
	if err := h.ensureStarted(ctx); err != nil {
		h.removeSubscriber(subscriberID)
		return nil, nil, err
	}

	cancel := func() {
		h.removeSubscriber(subscriberID)
	}
	return ch, cancel, nil
}

func (h *canonicalTranscriptHub) Snapshot() transcriptdomain.TranscriptSnapshot {
	if h == nil {
		return transcriptdomain.TranscriptSnapshot{}
	}
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	return cloneTranscriptSnapshot(h.snapshot)
}

func (h *canonicalTranscriptHub) Close() error {
	if h == nil {
		return nil
	}
	h.startMu.Lock()
	if h.explicitlyClosed {
		h.startMu.Unlock()
		return nil
	}
	h.explicitlyClosed = true
	cancel := h.runCancel
	done := h.done
	h.startMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	h.closeSubscribers()
	return nil
}

func (h *canonicalTranscriptHub) isExplicitlyClosed() bool {
	if h == nil {
		return true
	}
	h.startMu.Lock()
	defer h.startMu.Unlock()
	return h.explicitlyClosed
}

func (h *canonicalTranscriptHub) ensureStarted(ctx context.Context) error {
	h.startMu.Lock()
	defer h.startMu.Unlock()
	if h.explicitlyClosed {
		return unavailableError("canonical transcript hub closed", nil)
	}
	if h.started {
		return nil
	}
	if h.ingressFactory == nil {
		return unavailableError("transcript ingress factory unavailable", nil)
	}

	handle, err := h.ingressFactory.Open(ctx, h.sessionID, h.provider)
	if err != nil {
		return err
	}
	if handle.Close == nil {
		handle.Close = func() {}
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	h.runCancel = runCancel
	h.done = make(chan struct{})
	h.started = true
	base := h.currentRevision()
	go h.runLoop(runCtx, handle, base)
	return nil
}

func (h *canonicalTranscriptHub) runLoop(
	runCtx context.Context,
	handle TranscriptIngressHandle,
	base transcriptdomain.RevisionToken,
) {
	done := h.done
	defer func() {
		h.startMu.Lock()
		if h.runCancel != nil {
			h.runCancel = nil
		}
		h.done = nil
		h.started = false
		h.startMu.Unlock()
		h.closeSubscribers()
		if done != nil {
			close(done)
		}
	}()
	defer handle.Close()

	projector := h.projectorFactory.New(h.sessionID, h.provider, base)

	emit := func(event transcriptdomain.TranscriptEvent) bool {
		event.SessionID = h.sessionID
		event.Provider = h.provider
		if event.Replace != nil {
			event.Replace.SessionID = h.sessionID
			event.Replace.Provider = h.provider
			event.Replace.Revision = event.Revision
		}
		if !projector.Apply(event) {
			return false
		}
		if err := transcriptdomain.ValidateEvent(event); err != nil {
			return false
		}
		h.updateState(projector.Snapshot(), event.Revision)
		h.broadcast(event)
		return true
	}

	emitStatus := func(status transcriptdomain.StreamStatus) {
		statusEvent := transcriptdomain.TranscriptEvent{
			Kind:         transcriptdomain.TranscriptEventStreamStatus,
			SessionID:    h.sessionID,
			Provider:     h.provider,
			Revision:     projector.NextRevision(),
			StreamStatus: status,
		}
		_ = emit(statusEvent)
	}

	emitStatus(transcriptdomain.StreamStatusReady)
	if !handle.FollowAvailable || (handle.Events == nil && handle.Items == nil) {
		emitStatus(transcriptdomain.StreamStatusClosed)
		return
	}

	events := handle.Events
	items := handle.Items
	for {
		if events == nil && items == nil {
			emitStatus(transcriptdomain.StreamStatusClosed)
			return
		}
		select {
		case <-runCtx.Done():
			emitStatus(transcriptdomain.StreamStatusClosed)
			return
		case native, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			h.emitMappedEvent(projector, emit, native)
		case item, ok := <-items:
			if !ok {
				items = nil
				continue
			}
			h.emitMappedItem(projector, emit, item)
		}
	}
}

func (h *canonicalTranscriptHub) emitMappedEvent(
	projector TranscriptProjector,
	emit func(transcriptdomain.TranscriptEvent) bool,
	native types.CodexEvent,
) {
	mapped := h.mapper.MapEvent(h.provider, transcriptadapters.MappingContext{
		SessionID:    h.sessionID,
		Revision:     transcriptMappingBaseRevision,
		ActiveTurnID: projector.ActiveTurnID(),
	}, native)
	for _, event := range mapped {
		event.Revision = projector.NextRevision()
		event.SessionID = h.sessionID
		event.Provider = h.provider
		_ = emit(event)
	}
}

func (h *canonicalTranscriptHub) emitMappedItem(
	projector TranscriptProjector,
	emit func(transcriptdomain.TranscriptEvent) bool,
	item map[string]any,
) {
	mapped := h.mapper.MapItem(h.provider, transcriptadapters.MappingContext{
		SessionID:    h.sessionID,
		Revision:     transcriptMappingBaseRevision,
		ActiveTurnID: projector.ActiveTurnID(),
	}, item)
	for _, event := range mapped {
		event.Revision = projector.NextRevision()
		event.SessionID = h.sessionID
		event.Provider = h.provider
		_ = emit(event)
	}
}

func (h *canonicalTranscriptHub) updateState(snapshot transcriptdomain.TranscriptSnapshot, revision transcriptdomain.RevisionToken) {
	h.stateMu.Lock()
	h.snapshot = cloneTranscriptSnapshot(snapshot)
	h.revision = revision
	h.stateMu.Unlock()
}

func (h *canonicalTranscriptHub) snapshotAndRevision() (transcriptdomain.TranscriptSnapshot, transcriptdomain.RevisionToken) {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	return cloneTranscriptSnapshot(h.snapshot), h.revision
}

func (h *canonicalTranscriptHub) currentRevision() transcriptdomain.RevisionToken {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	return h.revision
}

func (h *canonicalTranscriptHub) broadcast(event transcriptdomain.TranscriptEvent) {
	h.fanoutMu.Lock()
	defer h.fanoutMu.Unlock()
	for id, subscriber := range h.subscribers {
		select {
		case subscriber <- event:
		default:
			close(subscriber)
			delete(h.subscribers, id)
		}
	}
}

func (h *canonicalTranscriptHub) removeSubscriber(id int) {
	h.fanoutMu.Lock()
	defer h.fanoutMu.Unlock()
	subscriber, ok := h.subscribers[id]
	if !ok {
		return
	}
	delete(h.subscribers, id)
	close(subscriber)
}

func (h *canonicalTranscriptHub) closeSubscribers() {
	h.fanoutMu.Lock()
	defer h.fanoutMu.Unlock()
	for id, subscriber := range h.subscribers {
		close(subscriber)
		delete(h.subscribers, id)
	}
}

func shouldReplaySnapshot(after, current transcriptdomain.RevisionToken) bool {
	if current.IsZero() {
		return false
	}
	if after.IsZero() {
		return true
	}
	newer, err := transcriptdomain.IsRevisionNewer(current, after)
	if err != nil {
		return true
	}
	return newer
}

func cloneTranscriptSnapshot(snapshot transcriptdomain.TranscriptSnapshot) transcriptdomain.TranscriptSnapshot {
	copySnapshot := snapshot
	if len(snapshot.Blocks) == 0 {
		copySnapshot.Blocks = nil
		return copySnapshot
	}
	copySnapshot.Blocks = append([]transcriptdomain.Block(nil), snapshot.Blocks...)
	return copySnapshot
}
