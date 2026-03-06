package app

import (
	"testing"
	"time"
)

func TestDefaultMetadataStreamRecoveryPolicyOnError(t *testing.T) {
	policy := newDefaultMetadataStreamRecoveryPolicy()
	decision := policy.OnError(0)
	if !decision.RefreshLists {
		t.Fatalf("expected refresh on stream error")
	}
	if decision.NextAttempts != 1 {
		t.Fatalf("expected attempts=1, got %d", decision.NextAttempts)
	}
	if decision.ReconnectDelay != metadataStreamRetryBase {
		t.Fatalf("expected base reconnect delay, got %s", decision.ReconnectDelay)
	}
}

func TestDefaultMetadataStreamRecoveryPolicyOnClosedBackoff(t *testing.T) {
	policy := newDefaultMetadataStreamRecoveryPolicy()
	decision := policy.OnClosed(2)
	if decision.RefreshLists {
		t.Fatalf("did not expect refresh on passive close")
	}
	if decision.NextAttempts != 3 {
		t.Fatalf("expected attempts=3, got %d", decision.NextAttempts)
	}
	if decision.ReconnectDelay <= metadataStreamRetryBase {
		t.Fatalf("expected exponential backoff delay, got %s", decision.ReconnectDelay)
	}
}

func TestDefaultMetadataStreamRecoveryPolicyCapsDelay(t *testing.T) {
	policy := newDefaultMetadataStreamRecoveryPolicy()
	decision := policy.OnError(20)
	if decision.ReconnectDelay != metadataStreamRetryMax {
		t.Fatalf("expected capped reconnect delay %s, got %s", metadataStreamRetryMax, decision.ReconnectDelay)
	}
}

func TestDefaultMetadataStreamRecoveryPolicyOnConnected(t *testing.T) {
	policy := newDefaultMetadataStreamRecoveryPolicy()
	decision := policy.OnConnected()
	if decision.NextAttempts != 0 {
		t.Fatalf("expected reconnect attempts reset")
	}
	if decision.ReconnectDelay != 0*time.Second {
		t.Fatalf("expected zero reconnect delay after connect")
	}
}
