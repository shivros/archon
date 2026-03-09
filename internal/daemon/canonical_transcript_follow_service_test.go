package daemon

import (
	"context"
	"errors"
	"testing"

	"control/internal/daemon/transcriptdomain"
)

type followServiceHubStub struct {
	ch          <-chan transcriptdomain.TranscriptEvent
	cancel      func()
	err         error
	subscribes  int
	lastAfter   transcriptdomain.RevisionToken
	lastContext context.Context
}

func (h *followServiceHubStub) Subscribe(
	ctx context.Context,
	after transcriptdomain.RevisionToken,
) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	h.subscribes++
	h.lastAfter = after
	h.lastContext = ctx
	if h.err != nil {
		return nil, nil, h.err
	}
	if h.cancel == nil {
		h.cancel = func() {}
	}
	return h.ch, h.cancel, nil
}

func (h *followServiceHubStub) Snapshot() transcriptdomain.TranscriptSnapshot {
	return transcriptdomain.TranscriptSnapshot{}
}

func (h *followServiceHubStub) Close() error { return nil }

type followServiceRegistryStub struct {
	hub          CanonicalTranscriptHub
	err          error
	calls        int
	lastSession  string
	lastProvider string
}

func (r *followServiceRegistryStub) HubForSession(_ context.Context, sessionID, provider string) (CanonicalTranscriptHub, error) {
	r.calls++
	r.lastSession = sessionID
	r.lastProvider = provider
	if r.err != nil {
		return nil, r.err
	}
	return r.hub, nil
}

func (r *followServiceRegistryStub) CloseSession(string) error { return nil }
func (r *followServiceRegistryStub) CloseAll() error           { return nil }

func TestCanonicalTranscriptFollowServiceOpenFollowRequiresSessionID(t *testing.T) {
	svc := NewCanonicalTranscriptFollowService(&followServiceRegistryStub{})
	if _, _, err := svc.OpenFollow(context.Background(), " ", "codex", ""); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestCanonicalTranscriptFollowServiceOpenFollowRequiresRegistry(t *testing.T) {
	svc := NewCanonicalTranscriptFollowService(nil)
	if _, _, err := svc.OpenFollow(context.Background(), "s1", "codex", ""); err == nil {
		t.Fatalf("expected unavailable error")
	}
}

func TestCanonicalTranscriptFollowServiceOpenFollowPropagatesRegistryError(t *testing.T) {
	boom := errors.New("boom")
	svc := NewCanonicalTranscriptFollowService(&followServiceRegistryStub{err: boom})
	if _, _, err := svc.OpenFollow(context.Background(), "s1", "codex", ""); !errors.Is(err, boom) {
		t.Fatalf("expected registry error, got %v", err)
	}
}

func TestCanonicalTranscriptFollowServiceOpenFollowRequiresHub(t *testing.T) {
	svc := NewCanonicalTranscriptFollowService(&followServiceRegistryStub{})
	if _, _, err := svc.OpenFollow(context.Background(), "s1", "codex", ""); err == nil {
		t.Fatalf("expected unavailable error")
	}
}

func TestCanonicalTranscriptFollowServiceOpenFollowPropagatesSubscribeError(t *testing.T) {
	boom := errors.New("subscribe failed")
	hub := &followServiceHubStub{err: boom}
	svc := NewCanonicalTranscriptFollowService(&followServiceRegistryStub{hub: hub})
	if _, _, err := svc.OpenFollow(context.Background(), "s1", "codex", ""); !errors.Is(err, boom) {
		t.Fatalf("expected subscribe error, got %v", err)
	}
}

func TestCanonicalTranscriptFollowServiceOpenFollowDelegatesWithNormalizedProvider(t *testing.T) {
	ch := make(chan transcriptdomain.TranscriptEvent)
	defer close(ch)
	hub := &followServiceHubStub{ch: ch}
	registry := &followServiceRegistryStub{hub: hub}
	svc := NewCanonicalTranscriptFollowService(registry)
	after := transcriptdomain.MustParseRevisionToken("42")
	stream, cancel, err := svc.OpenFollow(context.Background(), " s1 ", " CoDeX ", after)
	if err != nil {
		t.Fatalf("OpenFollow: %v", err)
	}
	if stream == nil || cancel == nil {
		t.Fatalf("expected stream and cancel")
	}
	if registry.calls != 1 {
		t.Fatalf("expected registry call count 1, got %d", registry.calls)
	}
	if registry.lastSession != "s1" {
		t.Fatalf("expected trimmed session id, got %q", registry.lastSession)
	}
	if registry.lastProvider != "codex" {
		t.Fatalf("expected normalized provider codex, got %q", registry.lastProvider)
	}
	if hub.subscribes != 1 {
		t.Fatalf("expected single subscribe call, got %d", hub.subscribes)
	}
	if hub.lastAfter != after {
		t.Fatalf("expected after %q, got %q", after, hub.lastAfter)
	}
}
