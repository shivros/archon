package app

import "testing"

func TestDebugStreamRetentionPolicyNormalize(t *testing.T) {
	policy := (DebugStreamRetentionPolicy{MaxLines: -2, MaxBytes: -5}).normalize()
	if policy.MaxLines != 0 {
		t.Fatalf("expected MaxLines to normalize to 0, got %d", policy.MaxLines)
	}
	if policy.MaxBytes != 0 {
		t.Fatalf("expected MaxBytes to normalize to 0, got %d", policy.MaxBytes)
	}
}
