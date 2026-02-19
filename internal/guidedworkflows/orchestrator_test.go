package guidedworkflows

import (
	"context"
	"testing"

	"control/internal/types"
)

func TestNormalizeConfigDefaults(t *testing.T) {
	cfg := NormalizeConfig(Config{})
	if cfg.Enabled {
		t.Fatalf("expected enabled=false by default")
	}
	if cfg.AutoStart {
		t.Fatalf("expected auto_start=false by default")
	}
	if cfg.CheckpointStyle != DefaultCheckpointStyle {
		t.Fatalf("unexpected default checkpoint style: %q", cfg.CheckpointStyle)
	}
	if cfg.Mode != DefaultMode {
		t.Fatalf("unexpected default mode: %q", cfg.Mode)
	}
}

func TestNormalizeConfigAcceptsAliases(t *testing.T) {
	cfg := NormalizeConfig(Config{
		CheckpointStyle: "confidence-weighted",
		Mode:            "guarded autopilot",
	})
	if cfg.CheckpointStyle != "confidence_weighted" {
		t.Fatalf("unexpected checkpoint style normalization: %q", cfg.CheckpointStyle)
	}
	if cfg.Mode != "guarded_autopilot" {
		t.Fatalf("unexpected mode normalization: %q", cfg.Mode)
	}
}

func TestNewReturnsNoopWhenDisabled(t *testing.T) {
	orchestrator := New(Config{})
	if orchestrator.Enabled() {
		t.Fatalf("expected disabled orchestrator")
	}
	if _, err := orchestrator.StartRun(context.Background(), StartRunRequest{WorktreeID: "wt-1"}); err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestNewEnabledStartRunUsesNormalizedConfig(t *testing.T) {
	orchestrator := New(Config{
		Enabled:         true,
		CheckpointStyle: "confidence-weighted",
		Mode:            "guarded-autopilot",
	})
	if !orchestrator.Enabled() {
		t.Fatalf("expected enabled orchestrator")
	}
	run, err := orchestrator.StartRun(context.Background(), StartRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
		TaskID:      "task-1",
	})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run == nil {
		t.Fatalf("expected run")
	}
	if run.ID == "" {
		t.Fatalf("expected run id")
	}
	if run.Mode != "guarded_autopilot" {
		t.Fatalf("unexpected mode: %q", run.Mode)
	}
	if run.CheckpointStyle != "confidence_weighted" {
		t.Fatalf("unexpected checkpoint style: %q", run.CheckpointStyle)
	}
}

func TestNewEnabledStartRunRequiresWorkspaceOrWorktree(t *testing.T) {
	orchestrator := New(Config{Enabled: true})
	if _, err := orchestrator.StartRun(context.Background(), StartRunRequest{}); err != ErrMissingContext {
		t.Fatalf("expected ErrMissingContext, got %v", err)
	}
}

func TestOnTurnEventNoopForFoundation(t *testing.T) {
	orchestrator := New(Config{Enabled: true})
	orchestrator.OnTurnEvent(context.Background(), types.NotificationEvent{
		Trigger: types.NotificationTriggerTurnCompleted,
	})
}
