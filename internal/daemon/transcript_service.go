package daemon

import (
	"context"
	"strings"

	"control/internal/daemon/transcriptadapters"
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

	items, err := s.History(ctx, id, lines)
	if err != nil {
		return transcriptdomain.TranscriptSnapshot{}, err
	}

	mapper := s.transcriptMapperOrDefault()
	projector := NewTranscriptProjector(id, provider, "")
	for _, item := range items {
		mapped := mapper.MapItem(provider, transcriptadapters.MappingContext{
			SessionID:    id,
			Revision:     transcriptdomain.MustParseRevisionToken("1"),
			ActiveTurnID: projector.ActiveTurnID(),
		}, item)
		for _, event := range mapped {
			event.Revision = projector.NextRevision()
			event.SessionID = id
			event.Provider = provider
			_ = projector.Apply(event)
		}
	}

	snapshot := projector.Snapshot()
	if err := transcriptdomain.ValidateSnapshot(snapshot); err != nil {
		return transcriptdomain.TranscriptSnapshot{}, unavailableError("transcript snapshot validation failed", err)
	}
	return snapshot, nil
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

	streamCtx, streamCancel := context.WithCancel(ctx)
	out := make(chan transcriptdomain.TranscriptEvent, 256)
	transport, err := s.transcriptTransportSelectorOrDefault().Select(streamCtx, id, provider)
	if err != nil {
		streamCancel()
		return nil, nil, err
	}

	cancel := func() {
		if transport.eventsCancel != nil {
			transport.eventsCancel()
		}
		if transport.itemsCancel != nil {
			transport.itemsCancel()
		}
		streamCancel()
	}

	mapper := s.transcriptMapperOrDefault()
	projector := NewTranscriptProjector(id, provider, baseRevision)

	go func() {
		defer close(out)
		defer cancel()

		emit := func(event transcriptdomain.TranscriptEvent) bool {
			if !projector.Apply(event) {
				return false
			}
			if err := transcriptdomain.ValidateEvent(event); err != nil {
				return false
			}
			select {
			case <-streamCtx.Done():
				return false
			case out <- event:
				return true
			}
		}

		ready := transcriptdomain.TranscriptEvent{
			Kind:         transcriptdomain.TranscriptEventStreamStatus,
			SessionID:    id,
			Provider:     provider,
			Revision:     projector.NextRevision(),
			StreamStatus: transcriptdomain.StreamStatusReady,
		}
		_ = emit(ready)

		eventsCh := transport.eventsCh
		itemsCh := transport.itemsCh
		for {
			if eventsCh == nil && itemsCh == nil {
				closed := transcriptdomain.TranscriptEvent{
					Kind:         transcriptdomain.TranscriptEventStreamStatus,
					SessionID:    id,
					Provider:     provider,
					Revision:     projector.NextRevision(),
					StreamStatus: transcriptdomain.StreamStatusClosed,
				}
				_ = emit(closed)
				return
			}
			select {
			case <-streamCtx.Done():
				return
			case native, ok := <-eventsCh:
				if !ok {
					eventsCh = nil
					continue
				}
				mapped := mapper.MapEvent(provider, transcriptadapters.MappingContext{
					SessionID:    id,
					Revision:     transcriptdomain.MustParseRevisionToken("1"),
					ActiveTurnID: projector.ActiveTurnID(),
				}, native)
				for _, event := range mapped {
					event.Revision = projector.NextRevision()
					event.SessionID = id
					event.Provider = provider
					_ = emit(event)
				}
			case item, ok := <-itemsCh:
				if !ok {
					itemsCh = nil
					continue
				}
				mapped := mapper.MapItem(provider, transcriptadapters.MappingContext{
					SessionID:    id,
					Revision:     transcriptdomain.MustParseRevisionToken("1"),
					ActiveTurnID: projector.ActiveTurnID(),
				}, item)
				for _, event := range mapped {
					event.Revision = projector.NextRevision()
					event.SessionID = id
					event.Provider = provider
					_ = emit(event)
				}
			}
		}
	}()

	return out, cancel, nil
}

func (s *SessionService) providerSupportsTranscriptStreaming(provider string) bool {
	caps := providers.CapabilitiesFor(provider)
	return caps.SupportsEvents || caps.UsesItems
}
