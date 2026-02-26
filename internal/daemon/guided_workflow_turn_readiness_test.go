package daemon

import (
	"testing"

	"control/internal/types"
)

func TestTurnProgressionReadinessRegistryKnownAndFallbackProviders(t *testing.T) {
	registry := newDefaultTurnProgressionReadinessRegistry()
	openCodePolicy := registry.ForProvider("opencode")
	kiloPolicy := registry.ForProvider("kilocode")
	unknownPolicy := registry.ForProvider("codex")

	if !openCodePolicy.AllowProgression(types.NotificationEvent{}, "failed", "err", true, "") {
		t.Fatalf("expected open code policy to allow terminal failures")
	}
	if !kiloPolicy.AllowProgression(types.NotificationEvent{
		Payload: map[string]any{"artifacts_persisted": true},
	}, "completed", "", true, "") {
		t.Fatalf("expected kilocode policy to allow when artifacts are persisted")
	}
	if !unknownPolicy.AllowProgression(types.NotificationEvent{}, "completed", "", false, "") {
		t.Fatalf("expected fallback policy to allow unknown providers")
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
			name: "terminal success with artifacts flag allowed",
			event: types.NotificationEvent{
				Payload: map[string]any{"turn_status": "completed", "artifacts_persisted": true},
			},
			want: true,
		},
		{
			name: "terminal success with artifact count allowed",
			event: types.NotificationEvent{
				Payload: map[string]any{"turn_status": "completed", "assistant_artifact_count": 2},
			},
			want: true,
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
			status := notificationPayloadString(tc.event.Payload, "turn_status")
			errMsg := notificationPayloadString(tc.event.Payload, "turn_error")
			output := notificationPayloadString(tc.event.Payload, "turn_output")
			terminal := status == "completed" || status == "failed"
			got := policy.AllowProgression(tc.event, status, errMsg, terminal, output)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}
