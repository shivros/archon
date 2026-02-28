package app

import (
	"strings"
	"testing"
	"time"
)

func TestPresentRateLimitSystemTextWithRetryAt(t *testing.T) {
	now := time.Date(2026, 2, 28, 15, 0, 0, 0, time.UTC)
	retryAt := now.Add(20 * time.Minute)
	text, ok := presentRateLimitSystemText(map[string]any{
		"type":     "rateLimit",
		"provider": "claude",
		"retry_at": retryAt.Format(time.RFC3339Nano),
	}, now, DefaultProviderDisplayPolicy())
	if !ok {
		t.Fatalf("expected rate limit text to be present")
	}
	if !strings.Contains(text, "Claude is rate-limited.") {
		t.Fatalf("expected provider line, got %q", text)
	}
	if !strings.Contains(text, "Try again at") {
		t.Fatalf("expected retry line, got %q", text)
	}
	if !strings.Contains(text, "in 20 minutes") {
		t.Fatalf("expected relative wait text, got %q", text)
	}
}

func TestPresentRateLimitSystemTextWithoutRetryAt(t *testing.T) {
	text, ok := presentRateLimitSystemText(map[string]any{
		"type":     "rateLimit",
		"provider": "claude",
	}, time.Time{}, DefaultProviderDisplayPolicy())
	if !ok {
		t.Fatalf("expected rate limit text to be present")
	}
	if !strings.Contains(text, "Claude is rate-limited.") {
		t.Fatalf("expected provider line, got %q", text)
	}
	if !strings.Contains(text, "Try again later.") {
		t.Fatalf("expected fallback retry line, got %q", text)
	}
}

func TestPresentRateLimitSystemTextIgnoresNonRateLimitItem(t *testing.T) {
	if text, ok := presentRateLimitSystemText(map[string]any{"type": "assistant"}, time.Now(), nil); ok || text != "" {
		t.Fatalf("expected non-rate-limit item to be ignored, got ok=%v text=%q", ok, text)
	}
}

func TestPresentRateLimitSystemTextNilPolicyFallsBack(t *testing.T) {
	text, ok := presentRateLimitSystemText(map[string]any{
		"type":     "rateLimit",
		"provider": "claude",
	}, time.Time{}, nil)
	if !ok {
		t.Fatalf("expected rate limit text")
	}
	if !strings.Contains(text, "Claude is rate-limited.") {
		t.Fatalf("expected default provider policy to apply, got %q", text)
	}
}

func TestPresentRateLimitSystemTextPastDueRetryShowsNow(t *testing.T) {
	now := time.Date(2026, 2, 28, 15, 0, 0, 0, time.UTC)
	text, ok := presentRateLimitSystemText(map[string]any{
		"type":       "rateLimit",
		"provider":   "claude",
		"retry_unix": now.Add(-1 * time.Minute).Unix(),
	}, now, DefaultProviderDisplayPolicy())
	if !ok {
		t.Fatalf("expected rate limit text")
	}
	if !strings.Contains(text, "Try again now.") {
		t.Fatalf("expected now message, got %q", text)
	}
}

func TestFormatRateLimitWaitBranches(t *testing.T) {
	now := time.Date(2026, 2, 28, 15, 0, 0, 0, time.UTC)
	if got := formatRateLimitWait(now, now.Add(20*time.Second)); got != "in under a minute" {
		t.Fatalf("under-minute branch mismatch: %q", got)
	}
	if got := formatRateLimitWait(now, now.Add(61*time.Second)); got != "in 1 minute" {
		t.Fatalf("one-minute branch mismatch: %q", got)
	}
	if got := formatRateLimitWait(now, now.Add(3*time.Hour)); got != "in 3 hours" {
		t.Fatalf("multi-hour branch mismatch: %q", got)
	}
}

type testProviderDisplayPolicy struct{}

func (testProviderDisplayPolicy) DisplayName(string) string { return "Anthropic Claude" }

func TestPresentRateLimitSystemTextUsesProviderPolicy(t *testing.T) {
	text, ok := presentRateLimitSystemText(map[string]any{
		"type":     "rateLimit",
		"provider": "claude",
	}, time.Time{}, testProviderDisplayPolicy{})
	if !ok {
		t.Fatalf("expected rate limit text to be present")
	}
	if !strings.Contains(text, "Anthropic Claude is rate-limited.") {
		t.Fatalf("expected custom provider label, got %q", text)
	}
}
