package daemon

import (
	"sort"
	"testing"
	"time"
)

func TestGuidedWorkflowDispatchProviderProfilesCoverage(t *testing.T) {
	profiles := guidedWorkflowDispatchProviderProfiles()
	if len(profiles) == 0 {
		t.Fatalf("expected dispatch provider profiles")
	}

	seen := map[string]struct{}{}
	for _, profile := range profiles {
		name := profile.name()
		if _, ok := seen[name]; ok {
			t.Fatalf("duplicate provider profile %q", name)
		}
		seen[name] = struct{}{}
		if !profile.supportsDispatch {
			t.Fatalf("expected supportsDispatch=true for %q", name)
		}
	}

	for _, want := range []string{"codex", "claude", "opencode", "kilocode"} {
		if _, ok := seen[want]; !ok {
			t.Fatalf("expected dispatch provider %q in guided workflow coverage (got=%v)", want, sortedMapKeys(seen))
		}
	}
}

func TestGuidedWorkflowInvalidModelProviderProfilesPolicy(t *testing.T) {
	profiles := guidedWorkflowInvalidModelProviderProfiles()
	if len(profiles) == 0 {
		t.Fatalf("expected invalid-model provider profiles")
	}

	got := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if !profile.invalidModelRelevant {
			t.Fatalf("expected invalidModelRelevant=true for %q", profile.name())
		}
		if !profile.supportsDispatch {
			t.Fatalf("expected supportsDispatch=true for %q", profile.name())
		}
		got = append(got, profile.name())
	}
	sort.Strings(got)
	want := []string{"kilocode", "opencode"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected invalid-model provider profiles: got=%v want=%v", got, want)
	}
}

func TestGuidedWorkflowReplyTransportPolicy(t *testing.T) {
	expected := map[string]guidedWorkflowReplyTransport{
		"codex":    guidedWorkflowReplyTransportHistory,
		"claude":   guidedWorkflowReplyTransportItems,
		"opencode": guidedWorkflowReplyTransportItems,
		"kilocode": guidedWorkflowReplyTransportItems,
	}

	for _, profile := range guidedWorkflowProviderProfiles() {
		want, tracked := expected[profile.name()]
		if !tracked {
			continue
		}
		if profile.replyTransport != want {
			t.Fatalf("unexpected reply transport for %q: got=%q want=%q", profile.name(), profile.replyTransport, want)
		}
	}

	if got := guidedWorkflowReplyTransportForProvider("unknown"); got != guidedWorkflowReplyTransportHistory {
		t.Fatalf("expected unknown provider to default to history transport, got %q", got)
	}
}

func TestGuidedWorkflowTimeoutPolicy(t *testing.T) {
	t.Run("uses fallback when timeout function missing", func(t *testing.T) {
		profile := guidedWorkflowProviderProfile{
			testCase: providerTestCase{name: "no-timeout"},
		}
		if got := guidedWorkflowTimeout(profile); got != 3*time.Minute {
			t.Fatalf("expected fallback timeout, got %s", got)
		}
	})

	t.Run("uses fallback when timeout function returns non-positive", func(t *testing.T) {
		profile := guidedWorkflowProviderProfile{
			testCase: providerTestCase{
				name:    "zero-timeout",
				timeout: func() time.Duration { return 0 },
			},
		}
		if got := guidedWorkflowTimeout(profile); got != 3*time.Minute {
			t.Fatalf("expected fallback timeout, got %s", got)
		}
	})

	t.Run("adds guided workflow buffer to provider timeout", func(t *testing.T) {
		profile := guidedWorkflowProviderProfile{
			testCase: providerTestCase{
				name:    "custom-timeout",
				timeout: func() time.Duration { return 10 * time.Second },
			},
		}
		if got := guidedWorkflowTimeout(profile); got != 70*time.Second {
			t.Fatalf("expected provider timeout + 1m buffer, got %s", got)
		}
	})
}

func TestGuidedWorkflowInvalidModelTimeoutPolicy(t *testing.T) {
	t.Run("uses fallback when timeout function missing", func(t *testing.T) {
		profile := guidedWorkflowProviderProfile{
			testCase: providerTestCase{name: "no-timeout"},
		}
		if got := guidedWorkflowInvalidModelTimeout(profile); got != 150*time.Second {
			t.Fatalf("expected fallback invalid-model timeout, got %s", got)
		}
	})

	t.Run("adds invalid-model buffer to provider timeout", func(t *testing.T) {
		profile := guidedWorkflowProviderProfile{
			testCase: providerTestCase{
				name:    "custom-timeout",
				timeout: func() time.Duration { return 20 * time.Second },
			},
		}
		if got := guidedWorkflowInvalidModelTimeout(profile); got != 50*time.Second {
			t.Fatalf("expected provider timeout + 30s buffer, got %s", got)
		}
	})
}

func TestValidateGuidedWorkflowProviderCoverage(t *testing.T) {
	if err := validateGuidedWorkflowProviderCoverage(nil, "test-case"); err == nil {
		t.Fatalf("expected validation error for empty coverage")
	}

	profiles := []guidedWorkflowProviderProfile{
		{testCase: providerTestCase{name: "codex"}},
	}
	if err := validateGuidedWorkflowProviderCoverage(profiles, "test-case"); err != nil {
		t.Fatalf("expected no validation error, got %v", err)
	}
}

func TestValidateGuidedWorkflowReplyTransport(t *testing.T) {
	for _, transport := range []guidedWorkflowReplyTransport{
		guidedWorkflowReplyTransportHistory,
		guidedWorkflowReplyTransportItems,
	} {
		if err := validateGuidedWorkflowReplyTransport("codex", transport); err != nil {
			t.Fatalf("expected valid transport %q, got %v", transport, err)
		}
	}

	if err := validateGuidedWorkflowReplyTransport("codex", guidedWorkflowReplyTransport("unknown")); err == nil {
		t.Fatalf("expected validation error for unsupported transport")
	}
}

func sortedMapKeys(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
