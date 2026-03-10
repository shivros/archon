package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const defaultCanonicalTranscriptHubIdleTTL = 45 * time.Second

type canonicalTranscriptHubLifecycle struct {
	hub             CanonicalTranscriptHub
	provider        string
	instanceID      string
	subscriberCount int
	lastDetachedAt  time.Time
	idleTimer       *time.Timer
}

type defaultCanonicalTranscriptHubRegistry struct {
	mu sync.Mutex

	ingressFactory   TranscriptIngressFactory
	mapper           TranscriptMapper
	projectorFactory TranscriptProjectorFactory
	reconnectPolicy  TranscriptReconnectPolicy
	idleTTL          time.Duration
	nextInstanceSeq  uint64

	hubs map[string]*canonicalTranscriptHubLifecycle
}

func NewDefaultCanonicalTranscriptHubRegistry(
	ingressFactory TranscriptIngressFactory,
	mapper TranscriptMapper,
	projectorFactory TranscriptProjectorFactory,
) CanonicalTranscriptHubRegistry {
	return newDefaultCanonicalTranscriptHubRegistryWithIdleTTL(
		ingressFactory,
		mapper,
		projectorFactory,
		defaultCanonicalTranscriptHubIdleTTL,
	)
}

func NewDefaultCanonicalTranscriptHubRegistryWithReconnectPolicy(
	ingressFactory TranscriptIngressFactory,
	mapper TranscriptMapper,
	projectorFactory TranscriptProjectorFactory,
	reconnectPolicy TranscriptReconnectPolicy,
) CanonicalTranscriptHubRegistry {
	return newDefaultCanonicalTranscriptHubRegistryWithPolicies(
		ingressFactory,
		mapper,
		projectorFactory,
		defaultCanonicalTranscriptHubIdleTTL,
		reconnectPolicy,
	)
}

func newDefaultCanonicalTranscriptHubRegistryWithIdleTTL(
	ingressFactory TranscriptIngressFactory,
	mapper TranscriptMapper,
	projectorFactory TranscriptProjectorFactory,
	idleTTL time.Duration,
) *defaultCanonicalTranscriptHubRegistry {
	return newDefaultCanonicalTranscriptHubRegistryWithPolicies(
		ingressFactory,
		mapper,
		projectorFactory,
		idleTTL,
		nil,
	)
}

func newDefaultCanonicalTranscriptHubRegistryWithPolicies(
	ingressFactory TranscriptIngressFactory,
	mapper TranscriptMapper,
	projectorFactory TranscriptProjectorFactory,
	idleTTL time.Duration,
	reconnectPolicy TranscriptReconnectPolicy,
) *defaultCanonicalTranscriptHubRegistry {
	if mapper == nil {
		mapper = NewDefaultTranscriptMapper(nil)
	}
	if projectorFactory == nil {
		projectorFactory = NewDefaultTranscriptProjectorFactory()
	}
	if idleTTL <= 0 {
		idleTTL = defaultCanonicalTranscriptHubIdleTTL
	}
	return &defaultCanonicalTranscriptHubRegistry{
		ingressFactory:   ingressFactory,
		mapper:           mapper,
		projectorFactory: projectorFactory,
		reconnectPolicy:  reconnectPolicy,
		idleTTL:          idleTTL,
		hubs:             map[string]*canonicalTranscriptHubLifecycle{},
	}
}

func (r *defaultCanonicalTranscriptHubRegistry) HubForSession(
	_ context.Context,
	sessionID, provider string,
) (CanonicalTranscriptHub, error) {
	if r == nil {
		return nil, unavailableError("canonical transcript hub registry unavailable", nil)
	}
	sessionID = strings.TrimSpace(sessionID)
	provider = normalizeTranscriptProvider(provider)
	if sessionID == "" {
		return nil, invalidError("session id is required", nil)
	}
	if provider == "" {
		return nil, invalidError("provider is required", nil)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if lifecycle, ok := r.hubs[sessionID]; ok && lifecycle != nil {
		hub := lifecycle.hub
		if hub != nil && !isCanonicalTranscriptHubExplicitlyClosed(hub) {
			if lifecycle.provider != provider {
				return nil, conflictError(
					"canonical transcript hub provider mismatch",
					fmt.Errorf("session %q bound to provider %q, requested %q", sessionID, lifecycle.provider, provider),
				)
			}
			return hub, nil
		}
		if lifecycle.idleTimer != nil {
			lifecycle.idleTimer.Stop()
			lifecycle.idleTimer = nil
		}
		delete(r.hubs, sessionID)
	}

	hub, err := newCanonicalTranscriptHub(sessionID, provider, r.ingressFactory, r.mapper, r.projectorFactory)
	if err != nil {
		return nil, err
	}
	if r.reconnectPolicy != nil {
		hub.setReconnectPolicy(r.reconnectPolicy)
	}
	r.nextInstanceSeq++
	instanceID := fmt.Sprintf("%s/%d", sessionID, r.nextInstanceSeq)
	hub.bindLifecycleObserver(r, instanceID)
	r.hubs[sessionID] = &canonicalTranscriptHubLifecycle{
		hub:        hub,
		provider:   provider,
		instanceID: instanceID,
	}
	return hub, nil
}

func isCanonicalTranscriptHubExplicitlyClosed(hub CanonicalTranscriptHub) bool {
	probe, ok := hub.(interface{ isExplicitlyClosed() bool })
	if !ok {
		return false
	}
	return probe.isExplicitlyClosed()
}

func (r *defaultCanonicalTranscriptHubRegistry) SubscriberAttached(sessionID, hubInstanceID string) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	hubInstanceID = strings.TrimSpace(hubInstanceID)
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	lifecycle, ok := r.hubs[sessionID]
	if !ok || lifecycle == nil {
		return
	}
	if hubInstanceID != "" && lifecycle.instanceID != hubInstanceID {
		return
	}
	lifecycle.subscriberCount++
	lifecycle.lastDetachedAt = time.Time{}
	if lifecycle.idleTimer != nil {
		lifecycle.idleTimer.Stop()
		lifecycle.idleTimer = nil
	}
}

func (r *defaultCanonicalTranscriptHubRegistry) SubscriberDetached(sessionID, hubInstanceID string) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	hubInstanceID = strings.TrimSpace(hubInstanceID)
	if sessionID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	lifecycle, ok := r.hubs[sessionID]
	if !ok || lifecycle == nil {
		return
	}
	if hubInstanceID != "" && lifecycle.instanceID != hubInstanceID {
		return
	}
	if lifecycle.subscriberCount > 0 {
		lifecycle.subscriberCount--
	}
	if lifecycle.subscriberCount > 0 {
		return
	}
	lifecycle.lastDetachedAt = time.Now().UTC()
	if lifecycle.idleTimer != nil {
		return
	}
	targetHubInstanceID := lifecycle.instanceID
	lifecycle.idleTimer = time.AfterFunc(r.idleTTL, func() {
		r.evictIdleHub(sessionID, targetHubInstanceID)
	})
}

func (r *defaultCanonicalTranscriptHubRegistry) evictIdleHub(sessionID, targetHubInstanceID string) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	var hub CanonicalTranscriptHub
	r.mu.Lock()
	lifecycle, ok := r.hubs[sessionID]
	if ok && lifecycle != nil && lifecycle.instanceID == targetHubInstanceID && lifecycle.subscriberCount == 0 {
		if lifecycle.idleTimer != nil {
			lifecycle.idleTimer.Stop()
			lifecycle.idleTimer = nil
		}
		hub = lifecycle.hub
		delete(r.hubs, sessionID)
	}
	r.mu.Unlock()
	if hub != nil {
		_ = hub.Close()
	}
}

func (r *defaultCanonicalTranscriptHubRegistry) HubClosed(sessionID, hubInstanceID string) {
	if r == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	hubInstanceID = strings.TrimSpace(hubInstanceID)
	if sessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	lifecycle, ok := r.hubs[sessionID]
	if !ok || lifecycle == nil {
		return
	}
	if hubInstanceID != "" && lifecycle.instanceID != hubInstanceID {
		return
	}
	if lifecycle.idleTimer != nil {
		lifecycle.idleTimer.Stop()
		lifecycle.idleTimer = nil
	}
	delete(r.hubs, sessionID)
}

func (r *defaultCanonicalTranscriptHubRegistry) CloseSession(sessionID string) error {
	if r == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}

	r.mu.Lock()
	lifecycle := r.hubs[sessionID]
	delete(r.hubs, sessionID)
	r.mu.Unlock()
	if lifecycle == nil {
		return nil
	}
	if lifecycle.idleTimer != nil {
		lifecycle.idleTimer.Stop()
	}
	if lifecycle.hub == nil {
		return nil
	}
	return lifecycle.hub.Close()
}

func (r *defaultCanonicalTranscriptHubRegistry) CloseAll() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	hubs := make([]CanonicalTranscriptHub, 0, len(r.hubs))
	for key, lifecycle := range r.hubs {
		if lifecycle == nil {
			delete(r.hubs, key)
			continue
		}
		if lifecycle.idleTimer != nil {
			lifecycle.idleTimer.Stop()
			lifecycle.idleTimer = nil
		}
		if lifecycle.hub != nil {
			hubs = append(hubs, lifecycle.hub)
		}
		delete(r.hubs, key)
	}
	r.mu.Unlock()
	for _, hub := range hubs {
		_ = hub.Close()
	}
	return nil
}
