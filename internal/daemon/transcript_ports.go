package daemon

import (
	"context"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

// TranscriptSnapshotReader builds a canonical transcript snapshot for a
// persisted session/provider pair.
type TranscriptSnapshotReader interface {
	ReadSnapshot(ctx context.Context, sessionID, provider string, lines int) (transcriptdomain.TranscriptSnapshot, error)
}

// TranscriptFollowOpener opens canonical transcript follow events for a
// session/provider pair.
type TranscriptFollowOpener interface {
	OpenFollow(
		ctx context.Context,
		sessionID, provider string,
		after transcriptdomain.RevisionToken,
	) (<-chan transcriptdomain.TranscriptEvent, func(), error)
}

// CanonicalTranscriptHub owns one session's live canonical transcript runtime.
type CanonicalTranscriptHub interface {
	Subscribe(
		ctx context.Context,
		after transcriptdomain.RevisionToken,
	) (<-chan transcriptdomain.TranscriptEvent, func(), error)
	Snapshot() transcriptdomain.TranscriptSnapshot
	Close() error
}

// CanonicalTranscriptHubRegistry owns lifecycle of session-scoped hubs.
type CanonicalTranscriptHubRegistry interface {
	HubForSession(ctx context.Context, sessionID, provider string) (CanonicalTranscriptHub, error)
	CloseSession(sessionID string) error
	CloseAll() error
}

// TranscriptIngressFactory opens provider-native follow transport.
type TranscriptIngressFactory interface {
	Open(ctx context.Context, sessionID, provider string) (TranscriptIngressHandle, error)
}

type TranscriptIngressHandle struct {
	Events          <-chan types.CodexEvent
	Items           <-chan map[string]any
	FollowAvailable bool
	Close           func()
}

type TranscriptProjectorFactory interface {
	New(
		sessionID, provider string,
		base transcriptdomain.RevisionToken,
	) TranscriptProjector
}

type transcriptHistoryReader func(ctx context.Context, sessionID string, lines int) ([]map[string]any, error)
