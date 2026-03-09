package daemon

import (
	"context"
	"reflect"
	"testing"
)

func TestTranscriptIngressFactoryOrDefaultNilService(t *testing.T) {
	var svc *SessionService
	if got := svc.transcriptIngressFactoryOrDefault(); got != nil {
		t.Fatalf("expected nil ingress factory for nil service")
	}
}

func TestTranscriptIngressFactoryOrDefaultCachesFactory(t *testing.T) {
	svc := &SessionService{}
	first := svc.transcriptIngressFactoryOrDefault()
	if first == nil {
		t.Fatalf("expected ingress factory")
	}
	second := svc.transcriptIngressFactoryOrDefault()
	if second == nil {
		t.Fatalf("expected ingress factory on second call")
	}
	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Fatalf("expected cached ingress factory instance")
	}
}

func TestTranscriptProjectorFactoryOrDefaultNilService(t *testing.T) {
	var svc *SessionService
	factory := svc.transcriptProjectorFactoryOrDefault()
	if factory == nil {
		t.Fatalf("expected default projector factory for nil service")
	}
	if projector := factory.New("s1", "codex", ""); projector == nil {
		t.Fatalf("expected projector from default factory")
	}
}

func TestTranscriptFollowOpenerOrDefaultNilService(t *testing.T) {
	var svc *SessionService
	opener := svc.transcriptFollowOpenerOrDefault()
	if opener == nil {
		t.Fatalf("expected follow opener for nil service")
	}
	if _, _, err := opener.OpenFollow(context.Background(), "s1", "codex", ""); err == nil {
		t.Fatalf("expected unavailable error from nil-service follow opener")
	}
}
