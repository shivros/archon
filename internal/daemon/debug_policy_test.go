package daemon

import (
	"testing"
	"time"
)

func TestDebugBatchPolicyNormalize(t *testing.T) {
	policy := (DebugBatchPolicy{
		FlushInterval:  -time.Second,
		MaxBatchBytes:  -10,
		FlushOnNewline: true,
	}).normalize()
	if policy.FlushInterval != 0 {
		t.Fatalf("expected FlushInterval to normalize to 0, got %s", policy.FlushInterval)
	}
	if policy.MaxBatchBytes != 0 {
		t.Fatalf("expected MaxBatchBytes to normalize to 0, got %d", policy.MaxBatchBytes)
	}
}

func TestDebugRetentionPolicyNormalize(t *testing.T) {
	policy := (DebugRetentionPolicy{MaxEvents: 0, MaxBytes: -1}).normalize()
	if policy.MaxEvents != debugMaxEvents {
		t.Fatalf("expected MaxEvents to normalize to default %d, got %d", debugMaxEvents, policy.MaxEvents)
	}
	if policy.MaxBytes != 0 {
		t.Fatalf("expected MaxBytes to normalize to 0, got %d", policy.MaxBytes)
	}
}
