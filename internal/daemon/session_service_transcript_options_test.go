package daemon

import (
	"context"
	"testing"

	"control/internal/daemon/transcriptdomain"
)

type testTranscriptSnapshotReader struct{}

func (testTranscriptSnapshotReader) ReadSnapshot(context.Context, string, string, int) (transcriptdomain.TranscriptSnapshot, error) {
	return transcriptdomain.TranscriptSnapshot{}, nil
}

type testTranscriptFollowOpener struct{}

func (testTranscriptFollowOpener) OpenFollow(context.Context, string, string, transcriptdomain.RevisionToken) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	ch := make(chan transcriptdomain.TranscriptEvent)
	close(ch)
	return ch, func() {}, nil
}

func TestWithTranscriptSnapshotReaderOptionGuardsAndSets(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	WithTranscriptSnapshotReader(nil)(svc)
	if svc.transcriptSnapshotRead != nil {
		t.Fatalf("expected nil reader to leave snapshot reader unset")
	}

	reader := testTranscriptSnapshotReader{}
	WithTranscriptSnapshotReader(reader)(svc)
	if svc.transcriptSnapshotRead == nil {
		t.Fatalf("expected snapshot reader option to set reader")
	}

	var nilSvc *SessionService
	WithTranscriptSnapshotReader(reader)(nilSvc)
}

func TestWithTranscriptFollowOpenerOptionGuardsAndSets(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	WithTranscriptFollowOpener(nil)(svc)
	if svc.transcriptFollowOpen != nil {
		t.Fatalf("expected nil opener to leave follow opener unset")
	}

	opener := testTranscriptFollowOpener{}
	WithTranscriptFollowOpener(opener)(svc)
	if svc.transcriptFollowOpen == nil {
		t.Fatalf("expected follow opener option to set opener")
	}

	var nilSvc *SessionService
	WithTranscriptFollowOpener(opener)(nilSvc)
}
