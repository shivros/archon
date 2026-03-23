package daemon

import (
	"strings"
	"testing"
	"time"
)

type stubProviderTurnWaitStrategyResolver struct {
	waiter       providerTurnCompletionWaiter
	providerName string
	calls        int
}

type stubNotificationScenarioSupportResolver struct {
	entries map[string]map[string]providerNotificationScenarioSupport
}

func (s stubNotificationScenarioSupportResolver) ScenarioSupport(provider string, scenario string) providerNotificationScenarioSupport {
	if s.entries == nil {
		return providerNotificationScenarioSupport{}
	}
	if byScenario, ok := s.entries[strings.ToLower(strings.TrimSpace(provider))]; ok {
		if support, ok := byScenario[strings.ToLower(strings.TrimSpace(scenario))]; ok {
			return support
		}
	}
	return providerNotificationScenarioSupport{}
}

type stubNotificationScenarioAssertionResolver struct {
	entries map[string]map[string]providerNotificationScenarioStatusAssertions
}

func (s stubNotificationScenarioAssertionResolver) ScenarioAssertions(provider string, scenario string) providerNotificationScenarioStatusAssertions {
	if s.entries == nil {
		return providerNotificationScenarioStatusAssertions{}
	}
	if byScenario, ok := s.entries[strings.ToLower(strings.TrimSpace(provider))]; ok {
		if assertions, ok := byScenario[strings.ToLower(strings.TrimSpace(scenario))]; ok {
			return assertions
		}
	}
	return providerNotificationScenarioStatusAssertions{}
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

func TestProviderTerminalNotificationScenariosIncludeCoreVariants(t *testing.T) {
	t.Parallel()
	scenarios := providerTerminalNotificationScenarios()
	if len(scenarios) < 3 {
		t.Fatalf("expected at least 3 terminal scenarios, got %d", len(scenarios))
	}
	names := map[string]bool{}
	for _, scenario := range scenarios {
		names[strings.ToLower(strings.TrimSpace(scenario.name))] = true
	}
	for _, required := range []string{
		providerNotificationScenarioCompleted,
		providerNotificationScenarioFailed,
		providerNotificationScenarioInterrupted,
	} {
		if !names[required] {
			t.Fatalf("expected scenario %q in registry", required)
		}
	}
}

func TestProviderNotificationProfileScenarioSupport(t *testing.T) {
	t.Parallel()

	openCode := providerNotificationProfileForTestCase(providerTestCase{name: "opencode"}, nil)
	if !openCode.SupportsScenario(providerNotificationScenarioFailed) {
		t.Fatalf("expected opencode to support failed scenario")
	}
	if !openCode.SupportsScenario(providerNotificationScenarioInterrupted) {
		t.Fatalf("expected opencode to support interrupted scenario")
	}
	if openCode.ScenarioSkipReason(providerNotificationScenarioFailed) != "" {
		t.Fatalf("expected no skip reason for supported scenario")
	}

	codex := providerNotificationProfileForTestCase(providerTestCase{name: "codex"}, nil)
	if codex.SupportsScenario(providerNotificationScenarioFailed) {
		t.Fatalf("expected codex failed scenario unsupported")
	}
	if strings.TrimSpace(codex.ScenarioSkipReason(providerNotificationScenarioFailed)) == "" {
		t.Fatalf("expected codex failed scenario skip reason")
	}
}

func TestProviderNotificationProfileExpectedNotificationStatusFallsBackToScenario(t *testing.T) {
	t.Parallel()
	profile := providerNotificationProfileForTestCase(providerTestCase{name: "opencode"}, nil)
	if got := profile.ExpectedNotificationStatus(providerNotificationScenarioFailed, ""); got != "failed" {
		t.Fatalf("expected fallback failed status, got %q", got)
	}
	if got := profile.ExpectedNotificationStatus(providerNotificationScenarioCompleted, "done"); got != "completed" {
		t.Fatalf("expected normalized observed status, got %q", got)
	}
}

func TestProviderNotificationProfilePhaseSpecificAssertionCapabilities(t *testing.T) {
	t.Parallel()

	profile := providerNotificationProfileForTestCase(providerTestCase{name: "opencode"}, nil)
	if !profile.CanAssertPublishedTerminalStatus(providerNotificationScenarioFailed) {
		t.Fatalf("expected published failed assertion capability")
	}
	if !profile.CanAssertDispatchedTerminalStatus(providerNotificationScenarioFailed) {
		t.Fatalf("expected dispatched failed assertion capability")
	}
	if !profile.CanAssertPublishedTerminalStatus(providerNotificationScenarioInterrupted) {
		t.Fatalf("expected published interrupted assertion capability")
	}
	if profile.CanAssertDispatchedTerminalStatus(providerNotificationScenarioInterrupted) {
		t.Fatalf("expected dispatched interrupted assertion capability to be disabled")
	}
}

func TestProviderNotificationProfileForTestCaseSupportsResolverOverrides(t *testing.T) {
	t.Parallel()

	supportResolver := stubNotificationScenarioSupportResolver{
		entries: map[string]map[string]providerNotificationScenarioSupport{
			"codex": {
				providerNotificationScenarioFailed: {
					supported: true,
				},
			},
		},
	}
	assertionResolver := stubNotificationScenarioAssertionResolver{
		entries: map[string]map[string]providerNotificationScenarioStatusAssertions{
			"codex": {
				providerNotificationScenarioFailed: {
					published:  true,
					dispatched: true,
				},
			},
		},
	}

	profile := providerNotificationProfileForTestCaseWithResolvers(
		providerTestCase{name: "codex"},
		nil,
		supportResolver,
		assertionResolver,
	)
	if !profile.SupportsScenario(providerNotificationScenarioFailed) {
		t.Fatalf("expected custom resolver to enable codex failed scenario")
	}
	if !profile.CanAssertPublishedTerminalStatus(providerNotificationScenarioFailed) {
		t.Fatalf("expected custom resolver to enable published failed assertion")
	}
	if !profile.CanAssertDispatchedTerminalStatus(providerNotificationScenarioFailed) {
		t.Fatalf("expected custom resolver to enable dispatched failed assertion")
	}
}

func TestDefaultNotificationScenarioSupportResolverFallbacks(t *testing.T) {
	t.Parallel()
	resolver := newDefaultNotificationScenarioSupportResolver()

	unknownScenario := resolver.ScenarioSupport("codex", "unknown-scenario")
	if unknownScenario.supported {
		t.Fatalf("expected unknown scenario to be unsupported")
	}
	if strings.TrimSpace(unknownScenario.reason) == "" {
		t.Fatalf("expected unknown scenario to include fallback reason")
	}

	defaultUnsupported := resolver.ScenarioSupport("codex", providerNotificationScenarioFailed)
	if defaultUnsupported.supported {
		t.Fatalf("expected codex failed scenario to be unsupported by default resolver")
	}
	if strings.TrimSpace(defaultUnsupported.reason) == "" {
		t.Fatalf("expected codex failed scenario to include reason")
	}
}

func TestDefaultNotificationScenarioAssertionResolverFallbacks(t *testing.T) {
	t.Parallel()
	resolver := newDefaultNotificationScenarioAssertionResolver()

	unknown := resolver.ScenarioAssertions("unknown-provider", "unknown-scenario")
	if unknown.published || unknown.dispatched {
		t.Fatalf("expected zero-value assertions for unknown scenario, got %#v", unknown)
	}

	opencodeFailed := resolver.ScenarioAssertions("opencode", providerNotificationScenarioFailed)
	if !opencodeFailed.published || !opencodeFailed.dispatched {
		t.Fatalf("expected opencode failed assertions enabled, got %#v", opencodeFailed)
	}
}
