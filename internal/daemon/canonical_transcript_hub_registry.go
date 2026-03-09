package daemon

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type defaultCanonicalTranscriptHubRegistry struct {
	mu sync.Mutex

	ingressFactory   TranscriptIngressFactory
	mapper           TranscriptMapper
	projectorFactory TranscriptProjectorFactory

	hubs map[string]*canonicalTranscriptHub
}

func NewDefaultCanonicalTranscriptHubRegistry(
	ingressFactory TranscriptIngressFactory,
	mapper TranscriptMapper,
	projectorFactory TranscriptProjectorFactory,
) CanonicalTranscriptHubRegistry {
	if mapper == nil {
		mapper = NewDefaultTranscriptMapper(nil)
	}
	if projectorFactory == nil {
		projectorFactory = NewDefaultTranscriptProjectorFactory()
	}
	return &defaultCanonicalTranscriptHubRegistry{
		ingressFactory:   ingressFactory,
		mapper:           mapper,
		projectorFactory: projectorFactory,
		hubs:             map[string]*canonicalTranscriptHub{},
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
	if hub, ok := r.hubs[sessionID]; ok && hub != nil && !hub.isExplicitlyClosed() {
		if hub.provider != provider {
			return nil, conflictError(
				"canonical transcript hub provider mismatch",
				fmt.Errorf("session %q bound to provider %q, requested %q", sessionID, hub.provider, provider),
			)
		}
		return hub, nil
	}

	hub, err := newCanonicalTranscriptHub(sessionID, provider, r.ingressFactory, r.mapper, r.projectorFactory)
	if err != nil {
		return nil, err
	}
	r.hubs[sessionID] = hub
	return hub, nil
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
	hub := r.hubs[sessionID]
	delete(r.hubs, sessionID)
	r.mu.Unlock()
	if hub == nil {
		return nil
	}
	return hub.Close()
}

func (r *defaultCanonicalTranscriptHubRegistry) CloseAll() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	hubs := make([]*canonicalTranscriptHub, 0, len(r.hubs))
	for key, hub := range r.hubs {
		if hub != nil {
			hubs = append(hubs, hub)
		}
		delete(r.hubs, key)
	}
	r.mu.Unlock()
	for _, hub := range hubs {
		_ = hub.Close()
	}
	return nil
}
