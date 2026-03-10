package daemon

import (
	"context"
	"strings"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
)

type canonicalTranscriptSnapshotService struct {
	mapper      TranscriptMapper
	readHistory transcriptHistoryReader
}

func NewCanonicalTranscriptSnapshotService(
	mapper TranscriptMapper,
	readHistory transcriptHistoryReader,
) TranscriptSnapshotReader {
	if mapper == nil {
		mapper = NewDefaultTranscriptMapper(nil)
	}
	return &canonicalTranscriptSnapshotService{
		mapper:      mapper,
		readHistory: readHistory,
	}
}

func (s *canonicalTranscriptSnapshotService) ReadSnapshot(
	ctx context.Context,
	sessionID, provider string,
	lines int,
) (transcriptdomain.TranscriptSnapshot, error) {
	sessionID = strings.TrimSpace(sessionID)
	provider = normalizeTranscriptProvider(provider)
	if sessionID == "" {
		return transcriptdomain.TranscriptSnapshot{}, invalidError("session id is required", nil)
	}
	if lines <= 0 {
		lines = defaultTranscriptSnapshotLines
	}
	if s == nil || s.readHistory == nil {
		return transcriptdomain.TranscriptSnapshot{}, unavailableError("transcript history reader unavailable", nil)
	}

	items, err := s.readHistory(ctx, sessionID, lines)
	if err != nil {
		return transcriptdomain.TranscriptSnapshot{}, err
	}

	projector := NewTranscriptProjector(sessionID, provider, "")
	for _, item := range items {
		mapped := s.mapper.MapItem(provider, transcriptadapters.MappingContext{
			SessionID:    sessionID,
			Revision:     transcriptdomain.MustParseRevisionToken("1"),
			ActiveTurnID: projector.ActiveTurnID(),
		}, item)
		for _, event := range mapped {
			event.Revision = projector.NextRevision()
			event.SessionID = sessionID
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
