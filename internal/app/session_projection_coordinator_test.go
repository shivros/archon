package app

import (
	"context"
	"testing"
	"time"
)

func TestDefaultSessionProjectionCoordinatorScheduleCancelsSupersededToken(t *testing.T) {
	coordinator := NewDefaultSessionProjectionCoordinator(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 4}, nil)
	token := "key:sess:s1"

	ctx1, seq1 := coordinator.Schedule(token, context.Background())
	if seq1 <= 0 {
		t.Fatalf("expected positive projection seq, got %d", seq1)
	}
	ctx2, seq2 := coordinator.Schedule(token, context.Background())
	if seq2 <= seq1 {
		t.Fatalf("expected second seq to advance, got first=%d second=%d", seq1, seq2)
	}
	select {
	case <-ctx1.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("expected superseded projection context to be canceled")
	}
	select {
	case <-ctx2.Done():
		t.Fatalf("expected latest projection context to remain active")
	default:
	}
}

func TestDefaultSessionProjectionCoordinatorConsumeClearsPendingOnlyForMatchingSeq(t *testing.T) {
	coordinator := NewDefaultSessionProjectionCoordinator(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 4}, nil)
	token := "key:sess:s1"

	_, seq := coordinator.Schedule(token, context.Background())
	if !coordinator.HasPending(token) {
		t.Fatalf("expected pending projection after schedule")
	}
	coordinator.Consume(token, seq-1)
	if !coordinator.HasPending(token) {
		t.Fatalf("expected mismatched consume to leave token pending")
	}
	coordinator.Consume(token, seq)
	if coordinator.HasPending(token) {
		t.Fatalf("expected matching consume to clear pending token")
	}
}

func TestDefaultSessionProjectionCoordinatorPrunesOldestToken(t *testing.T) {
	coordinator := NewDefaultSessionProjectionCoordinator(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 2}, nil)

	_, _ = coordinator.Schedule("key:a", context.Background())
	_, _ = coordinator.Schedule("key:b", context.Background())
	_, _ = coordinator.Schedule("key:c", context.Background())

	latest := coordinator.LatestByToken()
	if len(latest) != 2 {
		t.Fatalf("expected coordinator to keep 2 tracked tokens, got %d", len(latest))
	}
	if _, ok := latest["key:a"]; ok {
		t.Fatalf("expected oldest token to be pruned")
	}
}

func TestDefaultSessionProjectionCoordinatorNilSafety(t *testing.T) {
	var coordinator *defaultSessionProjectionCoordinator
	if ctx, seq := coordinator.Schedule("key:s1", context.Background()); ctx == nil || seq != 0 {
		t.Fatalf("expected nil coordinator schedule to return parent context and seq 0")
	}
	if !coordinator.IsCurrent("key:s1", 0) {
		t.Fatalf("expected nil coordinator to treat non-positive seq as current")
	}
	if coordinator.HasPending("key:s1") {
		t.Fatalf("expected nil coordinator to report no pending work")
	}
	coordinator.Consume("key:s1", 1)
	if latest := coordinator.LatestByToken(); latest != nil {
		t.Fatalf("expected nil coordinator latest snapshot to be nil")
	}
}

func TestDefaultSessionProjectionCoordinatorScheduleIgnoresBlankToken(t *testing.T) {
	coordinator := NewDefaultSessionProjectionCoordinator(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 2}, nil)
	ctx, seq := coordinator.Schedule("   ", context.Background())
	if ctx == nil {
		t.Fatalf("expected parent context for blank token schedule")
	}
	if seq != 0 {
		t.Fatalf("expected blank token to skip scheduling, got seq %d", seq)
	}
}

func TestDefaultSessionProjectionCoordinatorLatestByTokenReturnsSnapshot(t *testing.T) {
	coordinator := NewDefaultSessionProjectionCoordinator(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 2}, nil)
	_, _ = coordinator.Schedule("key:a", context.Background())
	latest := coordinator.LatestByToken()
	latest["key:a"] = 99

	refreshed := coordinator.LatestByToken()
	if refreshed["key:a"] == 99 {
		t.Fatalf("expected LatestByToken to return a copy")
	}
}

func TestDefaultSessionProjectionTrackerTreatsNonPositiveMaxAsOne(t *testing.T) {
	tracker := newDefaultSessionProjectionTracker()
	_ = tracker.Next("key:a", 0)
	seq := tracker.Next("key:b", -1)
	latest := tracker.LatestByToken()
	if len(latest) != 1 {
		t.Fatalf("expected non-positive max tracked to retain one token, got %d", len(latest))
	}
	if latest["key:b"] != seq {
		t.Fatalf("expected newest token to remain tracked, got %#v", latest)
	}
}
