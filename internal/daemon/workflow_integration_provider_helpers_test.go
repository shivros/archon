package daemon

import (
	"fmt"
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/providers"
	"control/internal/types"
)

type guidedWorkflowReplyTransport string

const (
	guidedWorkflowReplyTransportHistory guidedWorkflowReplyTransport = "history"
	guidedWorkflowReplyTransportItems   guidedWorkflowReplyTransport = "items"
)

type guidedWorkflowProviderProfile struct {
	testCase             providerTestCase
	replyTransport       guidedWorkflowReplyTransport
	supportsDispatch     bool
	invalidModelRelevant bool
}

func (p guidedWorkflowProviderProfile) name() string {
	return p.testCase.name
}

func (p guidedWorkflowProviderProfile) require(t *testing.T) {
	t.Helper()
	if p.testCase.require != nil {
		p.testCase.require(t)
	}
}

func (p guidedWorkflowProviderProfile) setup(t *testing.T) (repoDir string, runtimeOpts *types.SessionRuntimeOptions) {
	t.Helper()
	if p.testCase.setup == nil {
		t.Fatalf("provider %q has no integration setup function", p.name())
	}
	return p.testCase.setup(t)
}

func (p guidedWorkflowProviderProfile) baseTimeout() time.Duration {
	if p.testCase.timeout == nil {
		return 0
	}
	return p.testCase.timeout()
}

// guidedWorkflowProviderProfiles centralizes guided workflow integration provider
// policy and capabilities in one place. Env gating remains in each profile's
// require() function:
// - codex: ARCHON_CODEX_INTEGRATION
// - claude: ARCHON_CLAUDE_INTEGRATION
// - opencode: ARCHON_OPENCODE_INTEGRATION
// - kilocode: ARCHON_KILOCODE_INTEGRATION
func guidedWorkflowProviderProfiles() []guidedWorkflowProviderProfile {
	cases := allProviderTestCases()
	profiles := make([]guidedWorkflowProviderProfile, 0, len(cases))
	for _, tc := range cases {
		profiles = append(profiles, guidedWorkflowProfileForTestCase(tc))
	}
	return profiles
}

func guidedWorkflowProfileForTestCase(tc providerTestCase) guidedWorkflowProviderProfile {
	name := tc.name
	return guidedWorkflowProviderProfile{
		testCase:             tc,
		replyTransport:       guidedWorkflowReplyTransportForProvider(name),
		supportsDispatch:     guidedworkflows.SupportsDispatchProvider(name),
		invalidModelRelevant: guidedWorkflowInvalidModelMeaningfulPolicy(name),
	}
}

func guidedWorkflowDispatchProviderProfiles() []guidedWorkflowProviderProfile {
	profiles := guidedWorkflowProviderProfiles()
	filtered := make([]guidedWorkflowProviderProfile, 0, len(profiles))
	for _, profile := range profiles {
		if !profile.supportsDispatch {
			continue
		}
		filtered = append(filtered, profile)
	}
	return filtered
}

// guidedWorkflowInvalidModelProviderProfiles returns providers where guided
// workflow invalid-model assertions are meaningful and stable in integration.
func guidedWorkflowInvalidModelProviderProfiles() []guidedWorkflowProviderProfile {
	profiles := guidedWorkflowDispatchProviderProfiles()
	filtered := make([]guidedWorkflowProviderProfile, 0, len(profiles))
	for _, profile := range profiles {
		if !profile.invalidModelRelevant {
			continue
		}
		filtered = append(filtered, profile)
	}
	return filtered
}

func guidedWorkflowReplyTransportForProvider(provider string) guidedWorkflowReplyTransport {
	if providers.CapabilitiesFor(provider).UsesItems {
		return guidedWorkflowReplyTransportItems
	}
	return guidedWorkflowReplyTransportHistory
}

func guidedWorkflowInvalidModelMeaningfulPolicy(provider string) bool {
	// Invalid-model API assertions are intentionally scoped to OpenCode-family
	// providers where this behavior is currently deterministic in integration.
	switch providers.Normalize(provider) {
	case "opencode", "kilocode":
		return true
	default:
		return false
	}
}

func guidedWorkflowTimeout(profile guidedWorkflowProviderProfile) time.Duration {
	timeout := profile.baseTimeout()
	if timeout <= 0 {
		return 3 * time.Minute
	}
	// Guided workflows dispatch at least one extra turn beyond basic provider
	// session tests, so keep a small safety margin above provider defaults.
	return timeout + 1*time.Minute
}

func guidedWorkflowInvalidModelTimeout(profile guidedWorkflowProviderProfile) time.Duration {
	timeout := profile.baseTimeout()
	if timeout <= 0 {
		return 150 * time.Second
	}
	return timeout + 30*time.Second
}

func requireGuidedWorkflowProviderCoverage(t *testing.T, profiles []guidedWorkflowProviderProfile, testName string) {
	t.Helper()
	if err := validateGuidedWorkflowProviderCoverage(profiles, testName); err != nil {
		t.Fatalf("%v", err)
	}
}

func requireGuidedWorkflowReplyTransport(t *testing.T, provider string, transport guidedWorkflowReplyTransport) {
	t.Helper()
	if err := validateGuidedWorkflowReplyTransport(provider, transport); err != nil {
		t.Fatalf("%v", err)
	}
}

func validateGuidedWorkflowProviderCoverage(profiles []guidedWorkflowProviderProfile, testName string) error {
	if len(profiles) > 0 {
		return nil
	}
	return fmt.Errorf("%s: no guided workflow provider cases were discovered", testName)
}

func validateGuidedWorkflowReplyTransport(provider string, transport guidedWorkflowReplyTransport) error {
	if transport == guidedWorkflowReplyTransportHistory || transport == guidedWorkflowReplyTransportItems {
		return nil
	}
	return fmt.Errorf("provider %q has unsupported guided workflow reply transport %q", provider, transport)
}
