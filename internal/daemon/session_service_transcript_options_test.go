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

type testTranscriptIngressFactory struct{}

func (testTranscriptIngressFactory) Open(context.Context, string, string) (TranscriptIngressHandle, error) {
	return TranscriptIngressHandle{FollowAvailable: false, Close: func() {}}, nil
}

type testTranscriptProjectorFactory struct{}

func (testTranscriptProjectorFactory) New(string, string, transcriptdomain.RevisionToken) TranscriptProjector {
	return NewTranscriptProjector("s1", "codex", "")
}

type testCanonicalTranscriptHubRegistry struct{}

func (testCanonicalTranscriptHubRegistry) HubForSession(context.Context, string, string) (CanonicalTranscriptHub, error) {
	return nil, nil
}

func (testCanonicalTranscriptHubRegistry) CloseSession(string) error { return nil }
func (testCanonicalTranscriptHubRegistry) CloseAll() error           { return nil }

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

func TestWithTranscriptIngressFactoryOptionGuardsAndSets(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	WithTranscriptIngressFactory(nil)(svc)
	if svc.transcriptIngressOpen != nil {
		t.Fatalf("expected nil ingress factory to leave ingress factory unset")
	}

	factory := testTranscriptIngressFactory{}
	WithTranscriptIngressFactory(factory)(svc)
	if svc.transcriptIngressOpen == nil {
		t.Fatalf("expected ingress factory option to set factory")
	}

	var nilSvc *SessionService
	WithTranscriptIngressFactory(factory)(nilSvc)
}

func TestWithTranscriptProjectorFactoryOptionGuardsAndSets(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	WithTranscriptProjectorFactory(nil)(svc)
	if svc.transcriptProjectorCreate != nil {
		t.Fatalf("expected nil projector factory to leave projector factory unset")
	}

	factory := testTranscriptProjectorFactory{}
	WithTranscriptProjectorFactory(factory)(svc)
	if svc.transcriptProjectorCreate == nil {
		t.Fatalf("expected projector factory option to set factory")
	}

	var nilSvc *SessionService
	WithTranscriptProjectorFactory(factory)(nilSvc)
}

func TestWithCanonicalTranscriptHubRegistryOptionGuardsAndSets(t *testing.T) {
	t.Parallel()
	svc := &SessionService{}
	WithCanonicalTranscriptHubRegistry(nil)(svc)
	if svc.transcriptHubRegistry != nil {
		t.Fatalf("expected nil hub registry to leave hub registry unset")
	}

	registry := testCanonicalTranscriptHubRegistry{}
	WithCanonicalTranscriptHubRegistry(registry)(svc)
	if svc.transcriptHubRegistry == nil {
		t.Fatalf("expected hub registry option to set registry")
	}

	var nilSvc *SessionService
	WithCanonicalTranscriptHubRegistry(registry)(nilSvc)
}
