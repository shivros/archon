package daemon

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

// ProviderNotificationProfile encapsulates provider-specific notification
// integration behavior while allowing one shared test body.
type ProviderNotificationProfile struct {
	testCase                      providerTestCase
	waitForTurnCompletionFn       providerTurnCompletionWaiter
	normalizeExpectedTurnStatusFn func(string) string
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
	server *httptest.Server,
	manager *SessionManager,
	sessionID string,
	expectedTurnID string,
	timeout time.Duration,
) providerTurnCompletionResult {
	t.Helper()
	if p.waitForTurnCompletionFn == nil {
		t.Fatalf("provider %q has no turn-completion waiter", p.name())
	}
	return p.waitForTurnCompletionFn(t, server, manager, sessionID, expectedTurnID, timeout)
}

func (p ProviderNotificationProfile) normalizeExpectedTurnStatus(status string) string {
	if p.normalizeExpectedTurnStatusFn == nil {
		return normalizeProviderTurnStatus(status)
	}
	return p.normalizeExpectedTurnStatusFn(status)
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
	if resolver == nil {
		resolver = newProviderTurnCompletionWaitStrategyRegistry(defaultProviderCapabilitiesResolver{})
	}
	waiter := resolver.Waiter(tc.name)
	if waiter == nil {
		waiter = waitForProviderTurnCompletionFromHistory
	}
	return ProviderNotificationProfile{
		testCase:                      tc,
		waitForTurnCompletionFn:       waiter,
		normalizeExpectedTurnStatusFn: normalizeProviderTurnStatus,
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
		return fmt.Errorf("%s: missing provider notification profiles for %v", testName, missing)
	}
	return nil
}
