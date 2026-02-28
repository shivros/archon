package app

import (
	"testing"
	"time"
)

func TestNewDefaultTranscriptItemPresenterNilPolicyFallback(t *testing.T) {
	p := NewDefaultTranscriptItemPresenter(nil)
	if p == nil {
		t.Fatalf("expected presenter")
	}
}

func TestDefaultTranscriptItemPresenterPresentNilItem(t *testing.T) {
	p := NewDefaultTranscriptItemPresenter(nil)
	if _, ok := p.Present(nil, time.Time{}, time.Time{}); ok {
		t.Fatalf("expected nil item to be ignored")
	}
}

func TestDefaultTranscriptItemPresenterPresentNonRateLimitItem(t *testing.T) {
	p := NewDefaultTranscriptItemPresenter(nil)
	if _, ok := p.Present(map[string]any{"type": "assistant"}, time.Time{}, time.Now()); ok {
		t.Fatalf("expected non-rate-limit item to be ignored")
	}
}

func TestDefaultTranscriptItemPresenterPresentRateLimitItem(t *testing.T) {
	p := NewDefaultTranscriptItemPresenter(nil)
	block, ok := p.Present(map[string]any{
		"type":       "rateLimit",
		"provider":   "claude",
		"retry_unix": time.Now().Add(2 * time.Minute).Unix(),
	}, time.Now(), time.Now())
	if !ok {
		t.Fatalf("expected rate-limit item to be presented")
	}
	if block.Role != ChatRoleSystem {
		t.Fatalf("expected system role block, got %s", block.Role)
	}
}
