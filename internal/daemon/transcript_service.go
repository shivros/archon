package daemon

import (
	"context"
	"strings"

	"control/internal/daemon/transcriptdomain"
	"control/internal/providers"
)

const defaultTranscriptSnapshotLines = 200

func (s *SessionService) transcriptMapperOrDefault() TranscriptMapper {
	if s == nil || s.transcriptMapper == nil {
		return NewDefaultTranscriptMapper(nil)
	}
	return s.transcriptMapper
}

func (s *SessionService) transcriptTransportSelectorOrDefault() TranscriptTransportSelector {
	if s == nil || s.transcriptTransportSelect == nil {
		return NewDefaultTranscriptTransportSelector(s.SubscribeEvents, s.SubscribeItems)
	}
	return s.transcriptTransportSelect
}

func (s *SessionService) transcriptIngressFactoryOrDefault() TranscriptIngressFactory {
	if s == nil {
		return nil
	}
	if s.transcriptIngressOpen == nil {
		s.transcriptIngressOpen = NewSelectorTranscriptIngressFactory(s.transcriptTransportSelectorOrDefault())
	}
	return s.transcriptIngressOpen
}

func (s *SessionService) transcriptProjectorFactoryOrDefault() TranscriptProjectorFactory {
	if s == nil {
		return NewDefaultTranscriptProjectorFactory()
	}
	if s.transcriptProjectorCreate == nil {
		s.transcriptProjectorCreate = NewDefaultTranscriptProjectorFactory()
	}
	return s.transcriptProjectorCreate
}

func (s *SessionService) transcriptSnapshotReaderOrDefault() TranscriptSnapshotReader {
	if s == nil || s.transcriptSnapshotRead == nil {
		return NewCanonicalTranscriptSnapshotService(s.transcriptMapperOrDefault(), func(ctx context.Context, sessionID string, lines int) ([]map[string]any, error) {
			if s == nil {
				return nil, unavailableError("session service unavailable", nil)
			}
			return s.History(ctx, sessionID, lines)
		})
	}
	return s.transcriptSnapshotRead
}

func (s *SessionService) transcriptFollowOpenerOrDefault() TranscriptFollowOpener {
	if s == nil {
		return NewCanonicalTranscriptFollowService(nil)
	}
	s.transcriptMu.Lock()
	defer s.transcriptMu.Unlock()
	if s.transcriptFollowOpen == nil {
		if s.transcriptHubRegistry == nil {
			s.transcriptHubRegistry = NewDefaultCanonicalTranscriptHubRegistry(
				s.transcriptIngressFactoryOrDefault(),
				s.transcriptMapperOrDefault(),
				s.transcriptProjectorFactoryOrDefault(),
			)
		}
		s.transcriptFollowOpen = NewCanonicalTranscriptFollowService(s.transcriptHubRegistry)
	}
	return s.transcriptFollowOpen
}

func (s *SessionService) GetTranscriptSnapshot(ctx context.Context, id string, lines int) (transcriptdomain.TranscriptSnapshot, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return transcriptdomain.TranscriptSnapshot{}, invalidError("session id is required", nil)
	}
	session, _, err := s.getSessionRecord(ctx, id)
	if err != nil {
		return transcriptdomain.TranscriptSnapshot{}, err
	}
	if session == nil {
		return transcriptdomain.TranscriptSnapshot{}, notFoundError("session not found", ErrSessionNotFound)
	}
	provider := normalizeTranscriptProvider(session.Provider)
	if lines <= 0 {
		lines = defaultTranscriptSnapshotLines
	}
	return s.transcriptSnapshotReaderOrDefault().ReadSnapshot(ctx, id, provider, lines)
}

func (s *SessionService) SubscribeTranscript(
	ctx context.Context,
	id string,
	after transcriptdomain.RevisionToken,
) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil, invalidError("session id is required", nil)
	}
	session, _, err := s.getSessionRecord(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if session == nil {
		return nil, nil, notFoundError("session not found", ErrSessionNotFound)
	}
	provider := normalizeTranscriptProvider(session.Provider)

	baseRevision := transcriptdomain.RevisionToken("")
	if !after.IsZero() {
		parsed, parseErr := transcriptdomain.ParseRevisionToken(after.String())
		if parseErr != nil {
			return nil, nil, invalidError("invalid after_revision", parseErr)
		}
		baseRevision = parsed
	}
	return s.transcriptFollowOpenerOrDefault().OpenFollow(ctx, id, provider, baseRevision)
}

func (s *SessionService) providerSupportsTranscriptStreaming(provider string) bool {
	caps := providers.CapabilitiesFor(provider)
	return caps.SupportsEvents || caps.UsesItems
}
