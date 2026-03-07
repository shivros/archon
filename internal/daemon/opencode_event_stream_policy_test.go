package daemon

import (
	"testing"
	"time"
)

func TestDefaultOpenCodeEventReconnectPolicyRetryDelay(t *testing.T) {
	policy := defaultOpenCodeEventReconnectPolicy{}
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 300 * time.Millisecond},
		{attempt: 1, want: 300 * time.Millisecond},
		{attempt: 2, want: 600 * time.Millisecond},
		{attempt: 3, want: 1200 * time.Millisecond},
		{attempt: 4, want: 2400 * time.Millisecond},
		{attempt: 5, want: 3 * time.Second},
		{attempt: 6, want: 3 * time.Second},
	}
	for _, tc := range tests {
		if got := policy.RetryDelay(tc.attempt); got != tc.want {
			t.Fatalf("attempt=%d: expected %s, got %s", tc.attempt, tc.want, got)
		}
	}
}

func TestDefaultOpenCodeEventReconnectPolicyShouldLogFailure(t *testing.T) {
	policy := defaultOpenCodeEventReconnectPolicy{}
	tests := []struct {
		attempt int
		want    bool
	}{
		{attempt: -1, want: false},
		{attempt: 0, want: false},
		{attempt: 1, want: true},
		{attempt: 2, want: true},
		{attempt: 3, want: true},
		{attempt: 4, want: false},
		{attempt: 9, want: false},
		{attempt: 10, want: true},
		{attempt: 20, want: true},
	}
	for _, tc := range tests {
		if got := policy.ShouldLogFailure(tc.attempt); got != tc.want {
			t.Fatalf("attempt=%d: expected %t, got %t", tc.attempt, tc.want, got)
		}
	}
}
