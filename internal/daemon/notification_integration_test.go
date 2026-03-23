package daemon

import (
	"strings"
	"testing"

	"control/internal/providers"
	"control/internal/types"
)

func TestProviderTurnCompletedNotification(t *testing.T) {
	profiles := providerNotificationProfiles()
	requireProviderNotificationCoverage(t, profiles, "TestProviderTurnCompletedNotification")
	matchPolicy := newProviderNotificationMatchPolicy()

	for _, profile := range profiles {
		profile := profile
		t.Run(profile.name(), func(t *testing.T) {
			t.Parallel()
			profile.require(t)

			repoDir, runtimeOpts := profile.setup(t)
			env := newNotificationIntegrationServer(t)
			defer env.Close()

			ws := createWorkspace(t, env.server, repoDir)
			session := startSession(t, env.server, StartSessionRequest{
				Provider:       profile.name(),
				WorkspaceID:    ws.ID,
				RuntimeOptions: runtimeOpts,
			})
			if strings.TrimSpace(session.ID) == "" {
				t.Fatalf("session id missing")
			}

			timeout := providerNotificationTimeout(profile)
			turnID := sendMessageWithRetry(t, env.server, session.ID, "Say \"ok\" and nothing else.", timeout)
			if strings.TrimSpace(turnID) == "" {
				t.Fatalf("turn id missing from send")
			}

			completion := profile.waitForTurnCompletion(t, env, session.ID, turnID, timeout)
			if strings.TrimSpace(completion.TurnID) == "" {
				t.Fatalf("provider completion returned empty turn id (expected=%q)\n%s", turnID, sessionDiagnostics(env.manager, session.ID))
			}
			if strings.TrimSpace(completion.TurnID) != strings.TrimSpace(turnID) {
				t.Fatalf("provider completion turn id mismatch got=%q want=%q\n%s",
					completion.TurnID, turnID, sessionDiagnostics(env.manager, session.ID))
			}

			target := NotificationMatchTarget{
				Trigger:   types.NotificationTriggerTurnCompleted,
				SessionID: session.ID,
				Provider:  profile.name(),
				TurnID:    completion.TurnID,
			}
			event, ok := env.recorder.WaitForMatch(target, matchPolicy, timeout)
			if !ok {
				t.Fatalf("timeout waiting for turn-completed notification (target=%+v events=%s)\n%s",
					target, notificationEventsDebugString(env.recorder.Snapshot()), sessionDiagnostics(env.manager, session.ID))
			}

			matched := env.recorder.MatchingEvents(target, matchPolicy)
			if len(matched) != 1 {
				t.Fatalf("expected exactly one matching notification, got %d (target=%+v all=%s)\n%s",
					len(matched), target, notificationEventsDebugString(env.recorder.Snapshot()), sessionDiagnostics(env.manager, session.ID))
			}

			if event.Trigger != types.NotificationTriggerTurnCompleted {
				t.Fatalf("unexpected trigger %q", event.Trigger)
			}
			if strings.TrimSpace(event.SessionID) != strings.TrimSpace(session.ID) {
				t.Fatalf("notification session mismatch got=%q want=%q", event.SessionID, session.ID)
			}
			if providers.Normalize(event.Provider) != providers.Normalize(profile.name()) {
				t.Fatalf("notification provider mismatch got=%q want=%q", event.Provider, profile.name())
			}
			if strings.TrimSpace(event.TurnID) == "" {
				t.Fatalf("notification turn id missing")
			}

			expectedStatus := profile.normalizeExpectedTurnStatus(completion.Status)
			actualStatus := profile.normalizeExpectedTurnStatus(notificationTurnStatus(event))
			if expectedStatus != "" && actualStatus != "" && expectedStatus != actualStatus {
				t.Fatalf("notification turn status mismatch got=%q want=%q (raw_completion=%q raw_notification=%q)",
					actualStatus, expectedStatus, completion.Status, notificationTurnStatus(event))
			}
		})
	}
}
