package daemon

import (
	"testing"

	"control/internal/types"
)

func TestTurnProgressionReadinessRegistryKnownAndFallbackProviders(t *testing.T) {
	registry := newDefaultTurnProgressionReadinessRegistry()
	openCodePolicy := registry.ForProvider("opencode")
	kiloPolicy := registry.ForProvider("kilocode")
	codexPolicy := registry.ForProvider("codex")
	claudePolicy := registry.ForProvider("claude")
	unknownPolicy := registry.ForProvider("gemini")

	if !openCodePolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "failed", Error: "err", Terminal: true}) {
		t.Fatalf("expected open code policy to allow terminal failures")
	}
	if !kiloPolicy.AllowProgression(types.NotificationEvent{
		Payload: map[string]any{"turn_output_fresh": true},
	}, TurnProgressionEvidence{Status: "completed", Terminal: true, FreshOutput: true}) {
		t.Fatalf("expected kilocode policy to allow when turn output is fresh")
	}
	if codexPolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "in_progress", Terminal: false}) {
		t.Fatalf("expected codex policy to block non-terminal events")
	}
	if claudePolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "in_progress", Terminal: false}) {
		t.Fatalf("expected claude policy to block non-terminal events")
	}
	if !codexPolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "completed", Terminal: true, Output: "done"}) {
		t.Fatalf("expected codex policy to allow terminal completion")
	}
	if !unknownPolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "completed", Terminal: true, Output: "done"}) {
		t.Fatalf("expected fallback policy to allow terminal unknown-provider events")
	}
	if unknownPolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "in_progress", Terminal: false}) {
		t.Fatalf("expected fallback policy to block non-terminal unknown-provider events")
	}
}

func TestOpenCodeTurnProgressionReadinessPolicyMatrix(t *testing.T) {
	policy := openCodeTurnProgressionReadinessPolicy{}
	cases := []struct {
		name  string
		event types.NotificationEvent
		want  bool
	}{
		{
			name: "non-terminal blocked",
			event: types.NotificationEvent{
				Payload: map[string]any{"turn_status": "in_progress"},
			},
			want: false,
		},
		{
			name: "terminal failure allowed",
			event: types.NotificationEvent{
				Payload: map[string]any{"turn_status": "failed", "turn_error": "boom"},
			},
			want: true,
		},
		{
			name: "terminal success with output allowed",
			event: types.NotificationEvent{
				Payload: map[string]any{"turn_status": "completed", "turn_output": "done"},
			},
			want: true,
		},
		{
			name: "terminal success with fresh signal allowed",
			event: types.NotificationEvent{
				Payload: map[string]any{"turn_status": "completed", "turn_output_fresh": true},
			},
			want: true,
		},
		{
			name: "terminal success with artifact count blocked",
			event: types.NotificationEvent{
				Payload: map[string]any{"turn_status": "completed", "assistant_artifact_count": 2},
			},
			want: false,
		},
		{
			name: "terminal success without evidence blocked",
			event: types.NotificationEvent{
				Payload: map[string]any{"turn_status": "completed"},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := turnProgressionEvidenceFromNotification(tc.event)
			got := policy.AllowProgression(tc.event, evidence)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestTurnProgressionReadinessRegistryOptionsOverrideProviderAndFallback(t *testing.T) {
	registry := newTurnProgressionReadinessRegistry(
		withTurnProgressionProviderReadiness("gemini", allowAllTurnProgressionReadinessPolicy{}),
		withTurnProgressionFallbackReadiness(allowAllTurnProgressionReadinessPolicy{}),
	)
	geminiPolicy := registry.ForProvider("gemini")
	if !geminiPolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "in_progress"}) {
		t.Fatalf("expected overridden gemini policy to allow progression")
	}
	unknownPolicy := registry.ForProvider("custom-provider")
	if !unknownPolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "in_progress"}) {
		t.Fatalf("expected overridden fallback policy to allow progression")
	}
}

func TestTurnProgressionReadinessRegistryOptionsIgnoreInvalidInputs(t *testing.T) {
	registry := newTurnProgressionReadinessRegistry(
		withTurnProgressionProviderReadiness("", allowAllTurnProgressionReadinessPolicy{}),
		withTurnProgressionProviderReadiness("codex", nil),
		withTurnProgressionFallbackReadiness(nil),
	)
	codexPolicy := registry.ForProvider("codex")
	if codexPolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "in_progress", Terminal: false}) {
		t.Fatalf("expected default codex policy to remain in effect")
	}
	unknownPolicy := registry.ForProvider("unknown")
	if unknownPolicy.AllowProgression(types.NotificationEvent{}, TurnProgressionEvidence{Status: "in_progress", Terminal: false}) {
		t.Fatalf("expected default fallback policy to remain in effect")
	}
}

func TestTurnProgressionReadinessOptionsSafeOnNilRegistry(t *testing.T) {
	withTurnProgressionProviderReadiness("codex", allowAllTurnProgressionReadinessPolicy{})(nil)
	withTurnProgressionFallbackReadiness(allowAllTurnProgressionReadinessPolicy{})(nil)
}
