package daemon

import (
	"context"
	"strings"

	"control/internal/daemon/transcriptdomain"
)

type canonicalTranscriptFollowService struct {
	hubRegistry CanonicalTranscriptHubRegistry
}

func NewCanonicalTranscriptFollowService(
	registry CanonicalTranscriptHubRegistry,
) TranscriptFollowOpener {
	return &canonicalTranscriptFollowService{hubRegistry: registry}
}

func (s *canonicalTranscriptFollowService) OpenFollow(
	ctx context.Context,
	sessionID, provider string,
	after transcriptdomain.RevisionToken,
) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	sessionID = strings.TrimSpace(sessionID)
	provider = normalizeTranscriptProvider(provider)
	if sessionID == "" {
		return nil, nil, invalidError("session id is required", nil)
	}
	if s == nil || s.hubRegistry == nil {
		return nil, nil, unavailableError("canonical transcript hub registry unavailable", nil)
	}

	hub, err := s.hubRegistry.HubForSession(ctx, sessionID, provider)
	if err != nil {
		return nil, nil, err
	}
	if hub == nil {
		return nil, nil, unavailableError("canonical transcript hub unavailable", nil)
	}
	return hub.Subscribe(ctx, after)
}
