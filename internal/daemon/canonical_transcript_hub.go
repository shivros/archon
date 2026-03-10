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
	defaultTranscriptHubMaxReconnects    = 4
)

var transcriptMappingBaseRevision = transcriptdomain.MustParseRevisionToken("1")

type hubRuntimeState string

const (
	hubStateStarting     hubRuntimeState = "starting"
	hubStateReady        hubRuntimeState = "ready"
	hubStateReconnecting hubRuntimeState = "reconnecting"
	hubStateClosed       hubRuntimeState = "closed"
	hubStateError        hubRuntimeState = "error"
)

type ingressTerminationKind int

const (
	ingressTerminatedContextCanceled ingressTerminationKind = iota
	ingressTerminatedEOF
)

type defaultTranscriptReconnectPolicy struct {
	maxReconnects int
}

func NewDefaultTranscriptReconnectPolicy(maxReconnects int) TranscriptReconnectPolicy {
	if maxReconnects <= 0 {
		maxReconnects = defaultTranscriptHubMaxReconnects
	}
	return defaultTranscriptReconnectPolicy{maxReconnects: maxReconnects}
}

func (p defaultTranscriptReconnectPolicy) NextAttempt(current int, hadTraffic bool) (int, bool) {
	if hadTraffic {
		return 0, true
	}
	next := current + 1
	return next, next <= p.maxReconnects
}

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

	stateMu      sync.RWMutex
	snapshot     transcriptdomain.TranscriptSnapshot
	revision     transcriptdomain.RevisionToken
	runtimeState hubRuntimeState

	fanoutMu       sync.Mutex
	subscribers    map[int]chan transcriptdomain.TranscriptEvent
	nextSubscriber int

	reconnectPolicy TranscriptReconnectPolicy

	lifecycleObserver CanonicalTranscriptHubLifecycleObserver
	hubInstanceID     string
	closeNotifyOnce   sync.Once
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
		runtimeState:     hubStateStarting,
		subscribers:      map[int]chan transcriptdomain.TranscriptEvent{},
		nextSubscriber:   1,
		reconnectPolicy:  NewDefaultTranscriptReconnectPolicy(defaultTranscriptHubMaxReconnects),
	}, nil
}

func (h *canonicalTranscriptHub) bindLifecycleObserver(
	observer CanonicalTranscriptHubLifecycleObserver,
	hubInstanceID string,
) {
	if h == nil {
		return
	}
	h.lifecycleObserver = observer
	h.hubInstanceID = strings.TrimSpace(hubInstanceID)
}

func (h *canonicalTranscriptHub) setReconnectPolicy(policy TranscriptReconnectPolicy) {
	if h == nil || policy == nil {
		return
	}
	h.reconnectPolicy = policy
}

func (h *canonicalTranscriptHub) reconnectPolicyOrDefault() TranscriptReconnectPolicy {
	if h == nil || h.reconnectPolicy == nil {
		return NewDefaultTranscriptReconnectPolicy(defaultTranscriptHubMaxReconnects)
	}
	return h.reconnectPolicy
}

func (h *canonicalTranscriptHub) notifySubscriberAttached() {
	if h == nil || h.lifecycleObserver == nil {
		return
	}
	h.lifecycleObserver.SubscriberAttached(h.sessionID, h.hubInstanceID)
}

func (h *canonicalTranscriptHub) notifySubscriberDetached(count int) {
	if h == nil || h.lifecycleObserver == nil || count <= 0 {
		return
	}
	for i := 0; i < count; i++ {
		h.lifecycleObserver.SubscriberDetached(h.sessionID, h.hubInstanceID)
	}
}

func (h *canonicalTranscriptHub) notifyHubClosed() {
	if h == nil {
		return
	}
	h.closeNotifyOnce.Do(func() {
		if h.lifecycleObserver != nil {
			h.lifecycleObserver.HubClosed(h.sessionID, h.hubInstanceID)
		}
	})
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
	h.notifySubscriberAttached()
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
		return nil
	}
	h.closeSubscribers()
	h.transitionRuntimeState(hubStateClosed)
	h.notifyHubClosed()
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
	handle = normalizeTranscriptIngressHandle(handle)

	runCtx, runCancel := context.WithCancel(context.Background())
	h.runCancel = runCancel
	h.done = make(chan struct{})
	h.started = true
	h.transitionRuntimeState(hubStateStarting)
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
		h.transitionRuntimeState(hubStateClosed)
		h.notifyHubClosed()
		h.closeSubscribers()
		if done != nil {
			close(done)
		}
	}()

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

	currentHandle := handle
	reconnectAttempts := 0
	if !h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusReady) {
		currentHandle.Close()
		return
	}
	for {
		if !currentHandle.FollowAvailable || (currentHandle.Events == nil && currentHandle.Items == nil) {
			currentHandle.Close()
			_ = h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusClosed)
			return
		}
		termination, hadTraffic := h.consumeIngress(runCtx, currentHandle, projector, emit)
		currentHandle.Close()
		switch termination {
		case ingressTerminatedContextCanceled:
			_ = h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusClosed)
			return
		case ingressTerminatedEOF:
			nextAttempt, shouldReconnect := h.reconnectPolicyOrDefault().NextAttempt(reconnectAttempts, hadTraffic)
			reconnectAttempts = nextAttempt
			if !currentHandle.Reconnectable || h.isExplicitlyClosed() {
				_ = h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusClosed)
				return
			}
			if !shouldReconnect {
				_ = h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusError)
				_ = h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusClosed)
				return
			}
			if !h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusReconnecting) {
				_ = h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusClosed)
				return
			}
			nextHandle, err := h.ingressFactory.Open(runCtx, h.sessionID, h.provider)
			if err != nil {
				_ = h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusError)
				_ = h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusClosed)
				return
			}
			currentHandle = normalizeTranscriptIngressHandle(nextHandle)
			_ = h.emitHubStatus(projector, emit, transcriptdomain.StreamStatusReady)
		}
	}
}

func (h *canonicalTranscriptHub) consumeIngress(
	runCtx context.Context,
	handle TranscriptIngressHandle,
	projector TranscriptProjector,
	emit func(transcriptdomain.TranscriptEvent) bool,
) (ingressTerminationKind, bool) {
	events := handle.Events
	items := handle.Items
	hadTraffic := false
	for {
		if events == nil && items == nil {
			return ingressTerminatedEOF, hadTraffic
		}
		select {
		case <-runCtx.Done():
			return ingressTerminatedContextCanceled, hadTraffic
		case native, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			hadTraffic = true
			h.emitMappedEvent(projector, emit, native)
		case item, ok := <-items:
			if !ok {
				items = nil
				continue
			}
			hadTraffic = true
			h.emitMappedItem(projector, emit, item)
		}
	}
}

func normalizeTranscriptIngressHandle(handle TranscriptIngressHandle) TranscriptIngressHandle {
	if handle.Close == nil {
		handle.Close = func() {}
	}
	return handle
}

func hubStateFromStreamStatus(status transcriptdomain.StreamStatus) hubRuntimeState {
	switch status {
	case transcriptdomain.StreamStatusReady:
		return hubStateReady
	case transcriptdomain.StreamStatusReconnecting:
		return hubStateReconnecting
	case transcriptdomain.StreamStatusError:
		return hubStateError
	case transcriptdomain.StreamStatusClosed:
		return hubStateClosed
	default:
		return ""
	}
}

func (h *canonicalTranscriptHub) emitHubStatus(
	projector TranscriptProjector,
	emit func(transcriptdomain.TranscriptEvent) bool,
	status transcriptdomain.StreamStatus,
) bool {
	next := hubStateFromStreamStatus(status)
	if next == "" {
		return false
	}
	if !h.transitionRuntimeState(next) {
		return false
	}
	statusEvent := transcriptdomain.TranscriptEvent{
		Kind:         transcriptdomain.TranscriptEventStreamStatus,
		SessionID:    h.sessionID,
		Provider:     h.provider,
		Revision:     projector.NextRevision(),
		StreamStatus: status,
	}
	if !emit(statusEvent) {
		if status == transcriptdomain.StreamStatusClosed {
			h.transitionRuntimeState(hubStateClosed)
		}
		return false
	}
	return true
}

func (h *canonicalTranscriptHub) transitionRuntimeState(next hubRuntimeState) bool {
	if h == nil {
		return false
	}
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	current := h.runtimeState
	if current == "" {
		current = hubStateStarting
	}
	if current == next {
		h.runtimeState = next
		return true
	}
	allowed := false
	switch current {
	case hubStateStarting:
		allowed = next == hubStateReady || next == hubStateError || next == hubStateClosed
	case hubStateReady:
		allowed = next == hubStateReconnecting || next == hubStateError || next == hubStateClosed
	case hubStateReconnecting:
		allowed = next == hubStateReady || next == hubStateError || next == hubStateClosed
	case hubStateError:
		allowed = next == hubStateClosed
	case hubStateClosed:
		allowed = false
	}
	if !allowed {
		return false
	}
	h.runtimeState = next
	return true
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
	detached := 0
	for id, subscriber := range h.subscribers {
		select {
		case subscriber <- event:
		default:
			close(subscriber)
			delete(h.subscribers, id)
			detached++
		}
	}
	h.fanoutMu.Unlock()
	h.notifySubscriberDetached(detached)
}

func (h *canonicalTranscriptHub) removeSubscriber(id int) {
	h.fanoutMu.Lock()
	subscriber, ok := h.subscribers[id]
	if !ok {
		h.fanoutMu.Unlock()
		return
	}
	delete(h.subscribers, id)
	close(subscriber)
	h.fanoutMu.Unlock()
	h.notifySubscriberDetached(1)
}

func (h *canonicalTranscriptHub) closeSubscribers() {
	h.fanoutMu.Lock()
	detached := 0
	for id, subscriber := range h.subscribers {
		close(subscriber)
		delete(h.subscribers, id)
		detached++
	}
	h.fanoutMu.Unlock()
	h.notifySubscriberDetached(detached)
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
