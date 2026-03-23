package daemon

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

const (
	providerNotificationScenarioCompleted   = "completed"
	providerNotificationScenarioFailed      = "failed"
	providerNotificationScenarioInterrupted = "interrupted"
)

type providerNotificationScenarioSupport struct {
	supported bool
	reason    string
}

type providerNotificationScenarioStatusAssertions struct {
	published  bool
	dispatched bool
}

type providerNotificationScenarioContext struct {
	repoDir     string
	runtimeOpts *types.SessionRuntimeOptions
	timeout     time.Duration
}

type providerNotificationScenarioResult struct {
	sessionID string
	turn      providerTurnCompletionResult
}

type providerNotificationScenarioInducer func(
	t *testing.T,
	profile ProviderNotificationProfile,
	env *notificationIntegrationEnvironment,
	ctx providerNotificationScenarioContext,
) providerNotificationScenarioResult

type notificationScenarioSupportResolver interface {
	ScenarioSupport(provider string, scenario string) providerNotificationScenarioSupport
}

type notificationScenarioAssertionResolver interface {
	ScenarioAssertions(provider string, scenario string) providerNotificationScenarioStatusAssertions
}

type defaultNotificationScenarioSupportResolver struct {
	defaults    map[string]providerNotificationScenarioSupport
	byProvider  map[string]map[string]providerNotificationScenarioSupport
	fallbackMsg string
}

func newDefaultNotificationScenarioSupportResolver() notificationScenarioSupportResolver {
	return defaultNotificationScenarioSupportResolver{
		defaults: map[string]providerNotificationScenarioSupport{
			scenarioKey(providerNotificationScenarioCompleted): {
				supported: true,
			},
			scenarioKey(providerNotificationScenarioFailed): {
				supported: false,
				reason:    "failed-turn scenario currently deterministic only for OpenCode-family integrations",
			},
			scenarioKey(providerNotificationScenarioInterrupted): {
				supported: false,
				reason:    "interrupted-turn status assertions are currently deterministic only for OpenCode-family integrations",
			},
		},
		byProvider: map[string]map[string]providerNotificationScenarioSupport{
			"opencode": {
				scenarioKey(providerNotificationScenarioFailed): {
					supported: true,
				},
				scenarioKey(providerNotificationScenarioInterrupted): {
					supported: true,
				},
			},
			"kilocode": {
				scenarioKey(providerNotificationScenarioFailed): {
					supported: true,
				},
				scenarioKey(providerNotificationScenarioInterrupted): {
					supported: true,
				},
			},
		},
		fallbackMsg: "provider scenario support not declared",
	}
}

func (r defaultNotificationScenarioSupportResolver) ScenarioSupport(provider string, scenario string) providerNotificationScenarioSupport {
	providerKey := providers.Normalize(provider)
	scenarioName := scenarioKey(scenario)
	if byScenario, ok := r.byProvider[providerKey]; ok {
		if support, ok := byScenario[scenarioName]; ok {
			if !support.supported && strings.TrimSpace(support.reason) == "" {
				support.reason = r.fallbackMsg
			}
			return support
		}
	}
	if support, ok := r.defaults[scenarioName]; ok {
		if !support.supported && strings.TrimSpace(support.reason) == "" {
			support.reason = r.fallbackMsg
		}
		return support
	}
	return providerNotificationScenarioSupport{
		supported: false,
		reason:    r.fallbackMsg,
	}
}

type defaultNotificationScenarioAssertionResolver struct {
	defaults   map[string]providerNotificationScenarioStatusAssertions
	byProvider map[string]map[string]providerNotificationScenarioStatusAssertions
}

func newDefaultNotificationScenarioAssertionResolver() notificationScenarioAssertionResolver {
	return defaultNotificationScenarioAssertionResolver{
		defaults: map[string]providerNotificationScenarioStatusAssertions{
			scenarioKey(providerNotificationScenarioCompleted): {},
			scenarioKey(providerNotificationScenarioFailed):    {},
			scenarioKey(providerNotificationScenarioInterrupted): {
				published: true,
			},
		},
		byProvider: map[string]map[string]providerNotificationScenarioStatusAssertions{
			"opencode": {
				scenarioKey(providerNotificationScenarioFailed): {
					published:  true,
					dispatched: true,
				},
				scenarioKey(providerNotificationScenarioInterrupted): {
					published: true,
				},
			},
			"kilocode": {
				scenarioKey(providerNotificationScenarioFailed): {
					published:  true,
					dispatched: true,
				},
				scenarioKey(providerNotificationScenarioInterrupted): {
					published: true,
				},
			},
		},
	}
}

func (r defaultNotificationScenarioAssertionResolver) ScenarioAssertions(provider string, scenario string) providerNotificationScenarioStatusAssertions {
	providerKey := providers.Normalize(provider)
	scenarioName := scenarioKey(scenario)
	if byScenario, ok := r.byProvider[providerKey]; ok {
		if assertions, ok := byScenario[scenarioName]; ok {
			return assertions
		}
	}
	if assertions, ok := r.defaults[scenarioName]; ok {
		return assertions
	}
	return providerNotificationScenarioStatusAssertions{}
}

// NotificationTerminalScenario defines a provider-agnostic terminal notification
// integration scenario used by the shared provider-matrix tests.
type NotificationTerminalScenario struct {
	name           string
	expectedStatus string
	induceFn       providerNotificationScenarioInducer
}

func (s NotificationTerminalScenario) induce(
	t *testing.T,
	profile ProviderNotificationProfile,
	env *notificationIntegrationEnvironment,
	ctx providerNotificationScenarioContext,
) providerNotificationScenarioResult {
	t.Helper()
	if s.induceFn == nil {
		t.Fatalf("scenario %q has no inducer", s.name)
	}
	result := s.induceFn(t, profile, env, ctx)
	if strings.TrimSpace(result.sessionID) == "" {
		t.Fatalf("scenario %q returned empty session id", s.name)
	}
	if strings.TrimSpace(result.turn.TurnID) == "" {
		t.Fatalf("scenario %q returned empty turn id (session=%q)", s.name, result.sessionID)
	}
	return result
}

func providerTerminalNotificationScenarios() []NotificationTerminalScenario {
	return []NotificationTerminalScenario{
		{
			name:           providerNotificationScenarioCompleted,
			expectedStatus: "completed",
			induceFn:       induceProviderCompletedTurnScenario,
		},
		{
			name:           providerNotificationScenarioFailed,
			expectedStatus: "failed",
			induceFn:       induceProviderFailedTurnScenario,
		},
		{
			name:           providerNotificationScenarioInterrupted,
			expectedStatus: "interrupted",
			induceFn:       induceProviderInterruptedTurnScenario,
		},
	}
}

func providerTerminalNotificationScenarioByName(name string) (NotificationTerminalScenario, bool) {
	for _, scenario := range providerTerminalNotificationScenarios() {
		if strings.EqualFold(strings.TrimSpace(scenario.name), strings.TrimSpace(name)) {
			return scenario, true
		}
	}
	return NotificationTerminalScenario{}, false
}

// ProviderNotificationProfile encapsulates provider-specific notification
// integration behavior while allowing one shared test body.
type ProviderNotificationProfile struct {
	testCase                      providerTestCase
	waitForTurnCompletionFn       providerTurnCompletionWaiter
	normalizeExpectedTurnStatusFn func(string) string
	scenarioSupport               map[string]providerNotificationScenarioSupport
	scenarioExpectedStatus        map[string]string
	scenarioStatusAssertions      map[string]providerNotificationScenarioStatusAssertions
}

func (p ProviderNotificationProfile) name() string {
	return p.testCase.name
}

func (p ProviderNotificationProfile) require(t *testing.T) {
	t.Helper()
	if p.testCase.require != nil {
		p.testCase.require(t)
	}
}

func (p ProviderNotificationProfile) setup(t *testing.T) (repoDir string, runtimeOpts *types.SessionRuntimeOptions) {
	t.Helper()
	if p.testCase.setup == nil {
		t.Fatalf("provider %q has no integration setup function", p.name())
	}
	return p.testCase.setup(t)
}

func (p ProviderNotificationProfile) timeout() time.Duration {
	if p.testCase.timeout == nil {
		return 0
	}
	return p.testCase.timeout()
}

func (p ProviderNotificationProfile) waitForTurnCompletion(
	t *testing.T,
	env *notificationIntegrationEnvironment,
	sessionID string,
	expectedTurnID string,
	timeout time.Duration,
) providerTurnCompletionResult {
	t.Helper()
	if p.waitForTurnCompletionFn == nil {
		t.Fatalf("provider %q has no turn-completion waiter", p.name())
	}
	return p.waitForTurnCompletionFn(t, env, sessionID, expectedTurnID, timeout)
}

func (p ProviderNotificationProfile) normalizeExpectedTurnStatus(status string) string {
	if p.normalizeExpectedTurnStatusFn == nil {
		return normalizeProviderTurnStatus(status)
	}
	return p.normalizeExpectedTurnStatusFn(status)
}

func (p ProviderNotificationProfile) SupportsScenario(name string) bool {
	support, ok := p.scenarioSupport[scenarioKey(name)]
	if !ok {
		return false
	}
	return support.supported
}

func (p ProviderNotificationProfile) ScenarioSkipReason(name string) string {
	support, ok := p.scenarioSupport[scenarioKey(name)]
	if !ok {
		return fmt.Sprintf("provider %q has no support declaration for scenario %q", p.name(), strings.TrimSpace(name))
	}
	if support.supported {
		return ""
	}
	reason := strings.TrimSpace(support.reason)
	if reason == "" {
		reason = "provider scenario not supported"
	}
	return reason
}

func (p ProviderNotificationProfile) ExpectedNotificationStatus(scenarioName string, observedStatus string) string {
	if normalized := p.normalizeExpectedTurnStatus(observedStatus); normalized != "" {
		return normalized
	}
	if fallback := strings.TrimSpace(p.scenarioExpectedStatus[scenarioKey(scenarioName)]); fallback != "" {
		return p.normalizeExpectedTurnStatus(fallback)
	}
	return ""
}

func (p ProviderNotificationProfile) CanAssertPublishedTerminalStatus(scenarioName string) bool {
	return p.scenarioStatusAssertions[scenarioKey(scenarioName)].published
}

func (p ProviderNotificationProfile) CanAssertDispatchedTerminalStatus(scenarioName string) bool {
	return p.scenarioStatusAssertions[scenarioKey(scenarioName)].dispatched
}

func providerNotificationProfiles() []ProviderNotificationProfile {
	cases := allProviderTestCases()
	resolver := newProviderTurnCompletionWaitStrategyRegistry(defaultProviderCapabilitiesResolver{})
	profiles := make([]ProviderNotificationProfile, 0, len(cases))
	for _, tc := range cases {
		profiles = append(profiles, providerNotificationProfileForTestCase(tc, resolver))
	}
	return profiles
}

func providerNotificationProfileForTestCase(
	tc providerTestCase,
	resolver providerTurnCompletionWaitStrategyResolver,
) ProviderNotificationProfile {
	return providerNotificationProfileForTestCaseWithResolvers(tc, resolver, nil, nil)
}

func providerNotificationProfileForTestCaseWithResolvers(
	tc providerTestCase,
	resolver providerTurnCompletionWaitStrategyResolver,
	supportResolver notificationScenarioSupportResolver,
	assertionResolver notificationScenarioAssertionResolver,
) ProviderNotificationProfile {
	if resolver == nil {
		resolver = newProviderTurnCompletionWaitStrategyRegistry(defaultProviderCapabilitiesResolver{})
	}
	if supportResolver == nil {
		supportResolver = newDefaultNotificationScenarioSupportResolver()
	}
	if assertionResolver == nil {
		assertionResolver = newDefaultNotificationScenarioAssertionResolver()
	}
	waiter := resolver.Waiter(tc.name)
	if waiter == nil {
		waiter = waitForProviderTurnCompletionFromHistory
	}
	scenarioSupport := map[string]providerNotificationScenarioSupport{}
	scenarioExpectedStatus := map[string]string{}
	scenarioStatusAssertions := map[string]providerNotificationScenarioStatusAssertions{}
	for _, scenario := range providerTerminalNotificationScenarios() {
		key := scenarioKey(scenario.name)
		scenarioSupport[key] = supportResolver.ScenarioSupport(tc.name, scenario.name)
		scenarioExpectedStatus[key] = strings.TrimSpace(scenario.expectedStatus)
		scenarioStatusAssertions[key] = assertionResolver.ScenarioAssertions(tc.name, scenario.name)
	}

	return ProviderNotificationProfile{
		testCase:                      tc,
		waitForTurnCompletionFn:       waiter,
		normalizeExpectedTurnStatusFn: normalizeProviderTurnStatus,
		scenarioSupport:               scenarioSupport,
		scenarioExpectedStatus:        scenarioExpectedStatus,
		scenarioStatusAssertions:      scenarioStatusAssertions,
	}
}

func providerNotificationTimeout(profile ProviderNotificationProfile) time.Duration {
	timeout := profile.timeout()
	if timeout <= 0 {
		return 2 * time.Minute
	}
	return timeout
}

func requireProviderNotificationCoverage(t *testing.T, profiles []ProviderNotificationProfile, testName string) {
	t.Helper()
	if err := validateProviderNotificationCoverage(profiles, testName); err != nil {
		t.Fatalf("%v", err)
	}
}

func validateProviderNotificationCoverage(profiles []ProviderNotificationProfile, testName string) error {
	expected := allProviderTestCases()
	if len(expected) == 0 {
		return fmt.Errorf("%s: no provider integration cases discovered", testName)
	}
	if len(profiles) != len(expected) {
		return fmt.Errorf("%s: provider profile count mismatch (profiles=%d expected=%d)", testName, len(profiles), len(expected))
	}
	expectedByName := make(map[string]struct{}, len(expected))
	for _, tc := range expected {
		expectedByName[providers.Normalize(tc.name)] = struct{}{}
	}
	for _, profile := range profiles {
		name := providers.Normalize(profile.name())
		if _, ok := expectedByName[name]; !ok {
			return fmt.Errorf("%s: unexpected provider notification profile %q", testName, profile.name())
		}
		delete(expectedByName, name)
	}
	if len(expectedByName) > 0 {
		missing := make([]string, 0, len(expectedByName))
		for name := range expectedByName {
			missing = append(missing, name)
		}
		sort.Strings(missing)
		return fmt.Errorf("%s: missing provider notification profiles for %v", testName, missing)
	}
	return nil
}

func induceProviderCompletedTurnScenario(
	t *testing.T,
	profile ProviderNotificationProfile,
	env *notificationIntegrationEnvironment,
	ctx providerNotificationScenarioContext,
) providerNotificationScenarioResult {
	t.Helper()

	ws := createWorkspace(t, env.server, ctx.repoDir)
	session := startSession(t, env.server, StartSessionRequest{
		Provider:       profile.name(),
		WorkspaceID:    ws.ID,
		RuntimeOptions: ctx.runtimeOpts,
	})
	if strings.TrimSpace(session.ID) == "" {
		t.Fatalf("session id missing")
	}

	turnID := sendMessageWithRetry(t, env.server, session.ID, `Say "ok" and nothing else.`, ctx.timeout)
	if strings.TrimSpace(turnID) == "" {
		t.Fatalf("turn id missing from send")
	}

	completion := profile.waitForTurnCompletion(t, env, session.ID, turnID, ctx.timeout)
	completion.TurnID = normalizeTurnIDWithExpected(completion.TurnID, turnID)
	if completion.TurnID != strings.TrimSpace(turnID) {
		t.Fatalf("provider completion turn id mismatch got=%q want=%q\n%s",
			completion.TurnID, turnID, sessionDiagnostics(env.manager, session.ID))
	}

	return providerNotificationScenarioResult{
		sessionID: session.ID,
		turn:      completion,
	}
}

func induceProviderFailedTurnScenario(
	t *testing.T,
	profile ProviderNotificationProfile,
	env *notificationIntegrationEnvironment,
	ctx providerNotificationScenarioContext,
) providerNotificationScenarioResult {
	t.Helper()

	ws := createWorkspace(t, env.server, ctx.repoDir)
	session := startSession(t, env.server, StartSessionRequest{
		Provider:    profile.name(),
		WorkspaceID: ws.ID,
		Text:        `Say "ok" and nothing else.`,
		// Intentionally omit RuntimeOptions to trigger deterministic model-required fail-fast
		// behavior for providers that support this scenario.
		RuntimeOptions: nil,
	})
	if strings.TrimSpace(session.ID) == "" {
		t.Fatalf("session id missing")
	}

	completion := waitForTerminalTurnCompletionInHistory(t, env, session.ID, "", []string{"failed", "error", "abandoned"}, ctx.timeout)
	return providerNotificationScenarioResult{
		sessionID: session.ID,
		turn:      completion,
	}
}

func induceProviderInterruptedTurnScenario(
	t *testing.T,
	profile ProviderNotificationProfile,
	env *notificationIntegrationEnvironment,
	ctx providerNotificationScenarioContext,
) providerNotificationScenarioResult {
	t.Helper()

	ws := createWorkspace(t, env.server, ctx.repoDir)
	session := startSession(t, env.server, StartSessionRequest{
		Provider:       profile.name(),
		WorkspaceID:    ws.ID,
		RuntimeOptions: ctx.runtimeOpts,
	})
	if strings.TrimSpace(session.ID) == "" {
		t.Fatalf("session id missing")
	}

	stream, closeFn := openSSE(t, env.server, "/v1/sessions/"+session.ID+"/transcript/stream?follow=1")
	defer closeFn()

	finalToken := fmt.Sprintf("notification-interrupt-final-%s", providers.Normalize(profile.name()))
	turnID := sendMessageWithRetry(t, env.server, session.ID, providerInterruptPrompt(finalToken), ctx.timeout)
	if strings.TrimSpace(turnID) == "" {
		t.Fatalf("turn id missing from send")
	}

	readinessTimeout := 8 * time.Second
	if ctx.timeout > 0 && ctx.timeout < readinessTimeout {
		readinessTimeout = ctx.timeout
	}
	if !waitForProviderInterruptReadiness(stream, readinessTimeout) {
		time.Sleep(1500 * time.Millisecond)
	}

	status, body := interruptSession(env.server, session.ID)
	if status != 200 {
		t.Fatalf("interrupt failed status=%d body=%s", status, body)
	}

	completion := waitForTerminalTurnCompletionInHistory(t, env, session.ID, turnID, []string{"interrupted", "aborted", "cancelled", "canceled", "stopped"}, ctx.timeout)
	if normalized := normalizeProviderTurnStatus(completion.Status); normalized == "" || normalized == "completed" {
		t.Fatalf("expected interrupted terminal status, got raw=%q normalized=%q\n%s",
			completion.Status, normalized, sessionDiagnostics(env.manager, session.ID))
	}

	return providerNotificationScenarioResult{
		sessionID: session.ID,
		turn:      completion,
	}
}

func waitForTerminalTurnCompletionInHistory(
	t *testing.T,
	env *notificationIntegrationEnvironment,
	sessionID string,
	expectedTurnID string,
	expectedStatuses []string,
	timeout time.Duration,
) providerTurnCompletionResult {
	t.Helper()
	if env == nil {
		t.Fatalf("notification integration environment is required")
	}
	normalizedExpectedTurnID := strings.TrimSpace(expectedTurnID)
	normalizedExpectedStatuses := make([]string, 0, len(expectedStatuses))
	for _, candidate := range expectedStatuses {
		if normalized := normalizeProviderTurnStatus(candidate); normalized != "" {
			normalizedExpectedStatuses = append(normalizedExpectedStatuses, normalized)
		}
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		history := historySession(t, env.server, sessionID)
		if completion, ok := findTerminalTurnCompletionInHistoryItems(history.Items, normalizedExpectedTurnID, normalizedExpectedStatuses); ok {
			return completion
		}
		time.Sleep(providerNotificationPollInterval)
	}

	t.Fatalf("timeout waiting for terminal turn completion (session=%q expected_turn_id=%q expected_statuses=%v)\n%s",
		sessionID,
		normalizedExpectedTurnID,
		normalizedExpectedStatuses,
		sessionDiagnostics(env.manager, sessionID),
	)
	return providerTurnCompletionResult{}
}

func findTerminalTurnCompletionInHistoryItems(
	items []map[string]any,
	expectedTurnID string,
	expectedStatuses []string,
) (providerTurnCompletionResult, bool) {
	if len(items) == 0 {
		return providerTurnCompletionResult{}, false
	}
	expectedTurnID = strings.TrimSpace(expectedTurnID)
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if !strings.EqualFold(strings.TrimSpace(asString(item["type"])), "turnCompletion") {
			continue
		}
		turnID := normalizeTurnIDWithExpected(asString(item["turn_id"]), expectedTurnID)
		if expectedTurnID != "" && turnID != expectedTurnID {
			continue
		}
		status := strings.TrimSpace(asString(item["turn_status"]))
		if status == "" {
			status = strings.TrimSpace(asString(item["status"]))
		}
		normalizedStatus := normalizeProviderTurnStatus(status)

		if len(expectedStatuses) > 0 {
			matchedStatus := false
			for _, candidate := range expectedStatuses {
				if normalizedStatus == strings.TrimSpace(candidate) {
					matchedStatus = true
					break
				}
			}
			if !matchedStatus {
				if normalizeProviderTurnStatus(strings.TrimSpace(asString(item["turn_error"]))) != "" {
					// keep strict status matching; error text is handled by scenario-specific inducer
				}
				continue
			}
		}

		if strings.TrimSpace(status) == "" && len(expectedStatuses) > 0 {
			status = expectedStatuses[0]
		}
		return providerTurnCompletionResult{
			TurnID: turnID,
			Status: status,
		}, true
	}
	return providerTurnCompletionResult{}, false
}

func normalizeTurnIDWithExpected(rawTurnID string, expectedTurnID string) string {
	turnID := strings.TrimSpace(rawTurnID)
	expectedTurnID = strings.TrimSpace(expectedTurnID)
	if turnID == "" && expectedTurnID != "" {
		turnID = expectedTurnID
	}
	return turnID
}

func scenarioKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
