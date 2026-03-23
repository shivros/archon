package daemon

import (
	"testing"
	"time"
)

type stubProviderTurnWaitStrategyResolver struct {
	waiter       providerTurnCompletionWaiter
	providerName string
	calls        int
}

func (s *stubProviderTurnWaitStrategyResolver) Waiter(provider string) providerTurnCompletionWaiter {
	s.calls++
	s.providerName = provider
	return s.waiter
}

func TestProviderNotificationProfileForTestCaseUsesResolverWaiter(t *testing.T) {
	t.Parallel()
	waitCalls := 0
	resolver := &stubProviderTurnWaitStrategyResolver{
		waiter: func(*testing.T, *notificationIntegrationEnvironment, string, string, time.Duration) providerTurnCompletionResult {
			waitCalls++
			return providerTurnCompletionResult{TurnID: "turn-1", Status: "completed"}
		},
	}
	tc := providerTestCase{name: "codex"}
	profile := providerNotificationProfileForTestCase(tc, resolver)
	if resolver.calls != 1 || resolver.providerName != "codex" {
		t.Fatalf("expected resolver to be used for provider codex, calls=%d provider=%q", resolver.calls, resolver.providerName)
	}
	result := profile.waitForTurnCompletion(t, nil, "sess", "turn-1", time.Second)
	if waitCalls != 1 {
		t.Fatalf("expected waiter to be invoked once, got %d", waitCalls)
	}
	if result.TurnID != "turn-1" {
		t.Fatalf("unexpected turn id %q", result.TurnID)
	}
}

func TestProviderNotificationProfileForTestCaseHandlesNilResolver(t *testing.T) {
	t.Parallel()
	profile := providerNotificationProfileForTestCase(providerTestCase{name: "codex"}, nil)
	if profile.waitForTurnCompletionFn == nil {
		t.Fatalf("expected non-nil waiter when resolver is nil")
	}
}

func TestProviderNotificationProfileForTestCaseFallsBackWhenResolverReturnsNil(t *testing.T) {
	t.Parallel()
	resolver := &stubProviderTurnWaitStrategyResolver{waiter: nil}
	profile := providerNotificationProfileForTestCase(providerTestCase{name: "claude"}, resolver)
	if profile.waitForTurnCompletionFn == nil {
		t.Fatalf("expected fallback waiter when resolver returns nil")
	}
}

func TestProviderNotificationTimeoutDefaults(t *testing.T) {
	t.Parallel()
	profile := ProviderNotificationProfile{
		testCase: providerTestCase{
			name: "default",
		},
	}
	if got := providerNotificationTimeout(profile); got != 2*time.Minute {
		t.Fatalf("expected default timeout 2m, got %s", got)
	}
}

func TestProviderNotificationTimeoutUsesProviderTimeout(t *testing.T) {
	t.Parallel()
	profile := ProviderNotificationProfile{
		testCase: providerTestCase{
			name:    "custom",
			timeout: func() time.Duration { return 42 * time.Second },
		},
	}
	if got := providerNotificationTimeout(profile); got != 42*time.Second {
		t.Fatalf("expected provider timeout 42s, got %s", got)
	}
}

func TestRequireProviderNotificationCoverageSuccess(t *testing.T) {
	t.Parallel()
	requireProviderNotificationCoverage(t, providerNotificationProfiles(), "coverage-success")
}

func TestRequireProviderNotificationCoverageFailsWhenProfilesMissing(t *testing.T) {
	t.Parallel()
	err := validateProviderNotificationCoverage(nil, "coverage-missing")
	if err == nil {
		t.Fatalf("expected coverage validation error for missing profiles")
	}
}

func TestNormalizeProviderTurnStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{in: "done", want: "completed"},
		{in: "Succeeded", want: "completed"},
		{in: "cancelled", want: "interrupted"},
		{in: "STOPPED", want: "interrupted"},
		{in: "completed", want: "completed"},
	}
	for _, tc := range cases {
		if got := normalizeProviderTurnStatus(tc.in); got != tc.want {
			t.Fatalf("normalizeProviderTurnStatus(%q)=%q want=%q", tc.in, got, tc.want)
		}
	}
}

func TestFindTurnCompletionInHistoryItems(t *testing.T) {
	t.Parallel()
	items := []map[string]any{
		{"type": "agentMessage", "text": "hello"},
		{"type": "turnCompletion", "turn_id": "turn-a", "turn_status": "completed"},
		{"type": "turnCompletion", "turn_id": "turn-b", "status": "done"},
	}

	got, ok := findTurnCompletionInHistoryItems(items, "turn-a")
	if !ok {
		t.Fatalf("expected match for turn-a")
	}
	if got.TurnID != "turn-a" || got.Status != "completed" {
		t.Fatalf("unexpected turn-a result: %#v", got)
	}

	got, ok = findTurnCompletionInHistoryItems(items, "turn-b")
	if !ok {
		t.Fatalf("expected match for turn-b")
	}
	if got.TurnID != "turn-b" || got.Status != "done" {
		t.Fatalf("unexpected turn-b result: %#v", got)
	}

	if _, ok := findTurnCompletionInHistoryItems(items, "turn-missing"); ok {
		t.Fatalf("expected no match for missing turn id")
	}
}

func TestFindTurnCompletionInHistoryItemsFallsBackToExpectedTurnID(t *testing.T) {
	t.Parallel()
	items := []map[string]any{
		{"type": "turnCompletion", "turn_status": "completed"},
	}
	got, ok := findTurnCompletionInHistoryItems(items, "turn-fallback")
	if !ok {
		t.Fatalf("expected match with fallback turn id")
	}
	if got.TurnID != "turn-fallback" {
		t.Fatalf("expected fallback turn id, got %q", got.TurnID)
	}
}
