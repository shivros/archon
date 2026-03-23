package daemon

import (
	"strings"
	"testing"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

type notificationEventProbe interface {
	WaitForMatch(target NotificationMatchTarget, policy NotificationMatchPolicy, timeout time.Duration) (types.NotificationEvent, bool)
	MatchingEvents(target NotificationMatchTarget, policy NotificationMatchPolicy) []types.NotificationEvent
	Snapshot() []types.NotificationEvent
}

type notificationAssertionPhase string

const (
	notificationAssertionPhasePublished  notificationAssertionPhase = "published"
	notificationAssertionPhaseDispatched notificationAssertionPhase = "dispatched"
)

func TestProviderTurnCompletedNotificationPublished(t *testing.T) {
	profiles := providerNotificationProfiles()
	requireProviderNotificationCoverage(t, profiles, "TestProviderTurnCompletedNotificationPublished")
	matchPolicy := newProviderNotificationMatchPolicy()
	scenario := requireTerminalScenario(t, providerNotificationScenarioCompleted)

	for _, profile := range profiles {
		profile := profile
		t.Run(profile.name(), func(t *testing.T) {
			profile.require(t)

			repoDir, runtimeOpts := profile.setup(t)
			timeout := providerNotificationTimeout(profile)
			env := newNotificationIntegrationServer(t)
			defer env.Close()

			outcome := scenario.induce(t, profile, env, providerNotificationScenarioContext{
				repoDir:     repoDir,
				runtimeOpts: runtimeOpts,
				timeout:     timeout,
			})
			target := notificationTargetForScenario(profile, outcome)

			event := requireScenarioEventForPhase(
				t,
				env.recorder,
				profile,
				scenario,
				notificationAssertionPhasePublished,
				target,
				matchPolicy,
				outcome.turn.Status,
				timeout,
				sessionDiagnostics(env.manager, outcome.sessionID),
			)
			assertNotificationEventContract(t, profile, target, event)
			assertScenarioStatusContract(t, profile, scenario, outcome.turn.Status, event, notificationAssertionPhasePublished)
		})
	}
}

func TestProviderTurnCompletedNotificationSyncDispatch(t *testing.T) {
	profiles := providerNotificationProfiles()
	requireProviderNotificationCoverage(t, profiles, "TestProviderTurnCompletedNotificationSyncDispatch")
	matchPolicy := newProviderNotificationMatchPolicy()
	scenario := requireTerminalScenario(t, providerNotificationScenarioCompleted)

	for _, profile := range profiles {
		profile := profile
		t.Run(profile.name(), func(t *testing.T) {
			profile.require(t)

			repoDir, runtimeOpts := profile.setup(t)
			timeout := providerNotificationTimeout(profile)
			env := newNotificationIntegrationServer(t)
			defer env.Close()

			outcome := scenario.induce(t, profile, env, providerNotificationScenarioContext{
				repoDir:     repoDir,
				runtimeOpts: runtimeOpts,
				timeout:     timeout,
			})
			target := notificationTargetForScenario(profile, outcome)

			event := requireScenarioEventForPhase(
				t,
				env.dispatchProbe,
				profile,
				scenario,
				notificationAssertionPhaseDispatched,
				target,
				matchPolicy,
				outcome.turn.Status,
				timeout,
				sessionDiagnostics(env.manager, outcome.sessionID),
			)
			assertNotificationEventContract(t, profile, target, event)
			assertScenarioStatusContract(t, profile, scenario, outcome.turn.Status, event, notificationAssertionPhaseDispatched)
		})
	}
}

func TestProviderTurnTerminalNotificationVariants(t *testing.T) {
	profiles := providerNotificationProfiles()
	requireProviderNotificationCoverage(t, profiles, "TestProviderTurnTerminalNotificationVariants")
	matchPolicy := newProviderNotificationMatchPolicy()
	scenarios := providerTerminalNotificationScenarios()

	for _, profile := range profiles {
		profile := profile
		t.Run(profile.name(), func(t *testing.T) {
			profile.require(t)

			repoDir, runtimeOpts := profile.setup(t)
			timeout := providerNotificationTimeout(profile)
			for _, scenario := range scenarios {
				scenario := scenario
				t.Run(scenario.name, func(t *testing.T) {
					if !profile.SupportsScenario(scenario.name) {
						t.Skipf("provider %q scenario %q unsupported: %s", profile.name(), scenario.name, profile.ScenarioSkipReason(scenario.name))
					}

					env := newNotificationIntegrationServer(t)
					defer env.Close()

					outcome := scenario.induce(t, profile, env, providerNotificationScenarioContext{
						repoDir:     repoDir,
						runtimeOpts: runtimeOpts,
						timeout:     timeout,
					})
					target := notificationTargetForScenario(profile, outcome)

					published := requireScenarioEventForPhase(
						t,
						env.recorder,
						profile,
						scenario,
						notificationAssertionPhasePublished,
						target,
						matchPolicy,
						outcome.turn.Status,
						timeout,
						sessionDiagnostics(env.manager, outcome.sessionID),
					)
					assertNotificationEventContract(t, profile, target, published)
					assertScenarioStatusContract(t, profile, scenario, outcome.turn.Status, published, notificationAssertionPhasePublished)

					dispatched := requireScenarioEventForPhase(
						t,
						env.dispatchProbe,
						profile,
						scenario,
						notificationAssertionPhaseDispatched,
						target,
						matchPolicy,
						outcome.turn.Status,
						timeout,
						sessionDiagnostics(env.manager, outcome.sessionID),
					)
					assertNotificationEventContract(t, profile, target, dispatched)
					assertScenarioStatusContract(t, profile, scenario, outcome.turn.Status, dispatched, notificationAssertionPhaseDispatched)
				})
			}
		})
	}
}

func TestProviderTurnTerminalNotificationSyncDispatchDeduped(t *testing.T) {
	profiles := providerNotificationProfiles()
	requireProviderNotificationCoverage(t, profiles, "TestProviderTurnTerminalNotificationSyncDispatchDeduped")
	matchPolicy := newProviderNotificationMatchPolicy()
	scenario := requireTerminalScenario(t, providerNotificationScenarioCompleted)

	for _, profile := range profiles {
		profile := profile
		t.Run(profile.name(), func(t *testing.T) {
			profile.require(t)

			repoDir, runtimeOpts := profile.setup(t)
			timeout := providerNotificationTimeout(profile)
			env := newNotificationIntegrationServer(t)
			defer env.Close()

			outcome := scenario.induce(t, profile, env, providerNotificationScenarioContext{
				repoDir:     repoDir,
				runtimeOpts: runtimeOpts,
				timeout:     timeout,
			})
			target := notificationTargetForScenario(profile, outcome)

			firstDispatch := requireProbeMatchOnce(
				t,
				env.dispatchProbe,
				"dispatch",
				target,
				matchPolicy,
				timeout,
				sessionDiagnostics(env.manager, outcome.sessionID),
			)
			assertNotificationEventContract(t, profile, target, firstDispatch)

			env.Publish(firstDispatch)
			time.Sleep(100 * time.Millisecond)

			matched := env.dispatchProbe.MatchingEvents(target, matchPolicy)
			if len(matched) != 1 {
				t.Fatalf("expected dispatch dedupe to keep exactly one matching dispatch, got %d (target=%+v all=%s)\n%s",
					len(matched), target, notificationEventsDebugString(env.dispatchProbe.Snapshot()), sessionDiagnostics(env.manager, outcome.sessionID))
			}
		})
	}
}

func requireTerminalScenario(t *testing.T, name string) NotificationTerminalScenario {
	t.Helper()
	scenario, ok := providerTerminalNotificationScenarioByName(name)
	if !ok {
		t.Fatalf("terminal notification scenario %q not found", name)
	}
	return scenario
}

func notificationTargetForScenario(profile ProviderNotificationProfile, outcome providerNotificationScenarioResult) NotificationMatchTarget {
	return NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: outcome.sessionID,
		Provider:  profile.name(),
		TurnID:    outcome.turn.TurnID,
	}
}

func requireProbeMatchOnce(
	t *testing.T,
	probe notificationEventProbe,
	probeName string,
	target NotificationMatchTarget,
	policy NotificationMatchPolicy,
	timeout time.Duration,
	diagnostics string,
) types.NotificationEvent {
	t.Helper()

	event, ok := probe.WaitForMatch(target, policy, timeout)
	if !ok {
		t.Fatalf("timeout waiting for %s notification (target=%+v events=%s)\n%s",
			probeName, target, notificationEventsDebugString(probe.Snapshot()), diagnostics)
	}

	matched := probe.MatchingEvents(target, policy)
	if len(matched) != 1 {
		t.Fatalf("expected exactly one %s notification, got %d (target=%+v all=%s)\n%s",
			probeName, len(matched), target, notificationEventsDebugString(probe.Snapshot()), diagnostics)
	}
	return event
}

func requireProbeMatchByStatus(
	t *testing.T,
	probe notificationEventProbe,
	probeName string,
	target NotificationMatchTarget,
	policy NotificationMatchPolicy,
	expectedStatus string,
	timeout time.Duration,
	diagnostics string,
) types.NotificationEvent {
	t.Helper()

	expectedStatus = normalizeProviderTurnStatus(expectedStatus)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		waitWindow := providerNotificationPollInterval
		if remaining < waitWindow {
			waitWindow = remaining
		}
		if waitWindow <= 0 {
			break
		}
		_, _ = probe.WaitForMatch(target, policy, waitWindow)

		matched := probe.MatchingEvents(target, policy)
		statusMatched := make([]types.NotificationEvent, 0, len(matched))
		for _, event := range matched {
			if normalizeProviderTurnStatus(notificationTurnStatus(event)) == expectedStatus {
				statusMatched = append(statusMatched, event)
			}
		}
		if len(statusMatched) == 1 {
			return statusMatched[0]
		}
		if len(statusMatched) > 1 {
			t.Fatalf("expected exactly one %s notification with status=%q, got %d (target=%+v all=%s)\n%s",
				probeName, expectedStatus, len(statusMatched), target, notificationEventsDebugString(probe.Snapshot()), diagnostics)
		}
	}

	t.Fatalf("timeout waiting for %s notification with status=%q (target=%+v events=%s)\n%s",
		probeName, expectedStatus, target, notificationEventsDebugString(probe.Snapshot()), diagnostics)
	return types.NotificationEvent{}
}

func requireScenarioEventForPhase(
	t *testing.T,
	probe notificationEventProbe,
	profile ProviderNotificationProfile,
	scenario NotificationTerminalScenario,
	phase notificationAssertionPhase,
	target NotificationMatchTarget,
	policy NotificationMatchPolicy,
	observedTurnStatus string,
	timeout time.Duration,
	diagnostics string,
) types.NotificationEvent {
	t.Helper()

	expectedStatus := profile.ExpectedNotificationStatus(scenario.name, observedTurnStatus)
	switch phase {
	case notificationAssertionPhasePublished:
		if profile.CanAssertPublishedTerminalStatus(scenario.name) && strings.TrimSpace(expectedStatus) != "" {
			return requireProbeMatchByStatus(t, probe, string(phase), target, policy, expectedStatus, timeout, diagnostics)
		}
	case notificationAssertionPhaseDispatched:
		if profile.CanAssertDispatchedTerminalStatus(scenario.name) && strings.TrimSpace(expectedStatus) != "" {
			return requireProbeMatchByStatus(t, probe, string(phase), target, policy, expectedStatus, timeout, diagnostics)
		}
	}
	return requireProbeMatchOnce(t, probe, string(phase), target, policy, timeout, diagnostics)
}

func assertNotificationEventContract(
	t *testing.T,
	profile ProviderNotificationProfile,
	target NotificationMatchTarget,
	event types.NotificationEvent,
) {
	t.Helper()
	if event.Trigger != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected trigger %q", event.Trigger)
	}
	if strings.TrimSpace(event.SessionID) != strings.TrimSpace(target.SessionID) {
		t.Fatalf("notification session mismatch got=%q want=%q", event.SessionID, target.SessionID)
	}
	if providers.Normalize(event.Provider) != providers.Normalize(profile.name()) {
		t.Fatalf("notification provider mismatch got=%q want=%q", event.Provider, profile.name())
	}
	if strings.TrimSpace(event.TurnID) == "" {
		t.Fatalf("notification turn id missing")
	}
	if strings.TrimSpace(event.TurnID) != strings.TrimSpace(target.TurnID) {
		t.Fatalf("notification turn id mismatch got=%q want=%q", event.TurnID, target.TurnID)
	}
}

func assertScenarioStatusContract(
	t *testing.T,
	profile ProviderNotificationProfile,
	scenario NotificationTerminalScenario,
	observedTurnStatus string,
	event types.NotificationEvent,
	phase notificationAssertionPhase,
) {
	t.Helper()

	expectedStatus := profile.ExpectedNotificationStatus(scenario.name, observedTurnStatus)
	actualStatus := profile.normalizeExpectedTurnStatus(notificationTurnStatus(event))
	switch phase {
	case notificationAssertionPhasePublished:
		if !profile.CanAssertPublishedTerminalStatus(scenario.name) {
			return
		}
	case notificationAssertionPhaseDispatched:
		if !profile.CanAssertDispatchedTerminalStatus(scenario.name) {
			return
		}
	default:
		t.Fatalf("unknown assertion phase %q", phase)
	}
	if expectedStatus == "" {
		t.Fatalf("%s notification expected status missing for scenario %q", phase, scenario.name)
	}
	if actualStatus == "" {
		t.Fatalf("%s notification status missing for scenario %q", phase, scenario.name)
	}
	if expectedStatus != actualStatus {
		t.Fatalf("%s notification turn status mismatch got=%q want=%q (scenario=%q raw_observed=%q raw_notification=%q)",
			phase, actualStatus, expectedStatus, scenario.name, observedTurnStatus, notificationTurnStatus(event))
	}
}
