package types

import "time"

type AppState struct {
	ActiveWorkspaceID         string                            `json:"active_workspace_id"`
	ActiveWorktreeID          string                            `json:"active_worktree_id"`
	ActiveWorkspaceGroupIDs   []string                          `json:"active_workspace_group_ids"`
	SidebarCollapsed          bool                              `json:"sidebar_collapsed"`
	SidebarWorkspaceExpanded  map[string]bool                   `json:"sidebar_workspace_expanded,omitempty"`
	SidebarWorktreeExpanded   map[string]bool                   `json:"sidebar_worktree_expanded,omitempty"`
	ComposeHistory            map[string][]string               `json:"compose_history,omitempty"`
	ComposeDrafts             map[string]string                 `json:"compose_drafts,omitempty"`
	NoteDrafts                map[string]string                 `json:"note_drafts,omitempty"`
	ComposeDefaultsByProvider map[string]*SessionRuntimeOptions `json:"compose_defaults_by_provider,omitempty"`
	ProviderBadges            map[string]*ProviderBadgeConfig   `json:"provider_badges,omitempty"`
	Recents                   *AppStateRecents                  `json:"recents,omitempty"`
	GuidedWorkflowTelemetry   *GuidedWorkflowTelemetryState     `json:"guided_workflow_telemetry,omitempty"`
}

type ProviderBadgeConfig struct {
	Prefix string `json:"prefix,omitempty"`
	Color  string `json:"color,omitempty"`
}

type AppStateRecents struct {
	Version       int                          `json:"version,omitempty"`
	Running       map[string]AppStateRecentRun `json:"running,omitempty"`
	Ready         map[string]AppStateReadyItem `json:"ready,omitempty"`
	ReadyQueue    []AppStateReadyQueueEntry    `json:"ready_queue,omitempty"`
	DismissedTurn map[string]string            `json:"dismissed_turn,omitempty"`
}

type AppStateRecentRun struct {
	SessionID      string `json:"session_id"`
	BaselineTurnID string `json:"baseline_turn_id,omitempty"`
	StartedAtUnix  int64  `json:"started_at_unix,omitempty"`
}

type AppStateReadyItem struct {
	SessionID       string `json:"session_id"`
	CompletionTurn  string `json:"completion_turn,omitempty"`
	CompletedAtUnix int64  `json:"completed_at_unix,omitempty"`
	LastKnownTurnID string `json:"last_known_turn_id,omitempty"`
}

type AppStateReadyQueueEntry struct {
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
}

type GuidedWorkflowTelemetryState struct {
	CapturedAt           time.Time      `json:"captured_at,omitempty"`
	RunsStarted          int            `json:"runs_started,omitempty"`
	RunsCompleted        int            `json:"runs_completed,omitempty"`
	RunsFailed           int            `json:"runs_failed,omitempty"`
	PauseCount           int            `json:"pause_count,omitempty"`
	PauseRate            float64        `json:"pause_rate,omitempty"`
	ApprovalCount        int            `json:"approval_count,omitempty"`
	ApprovalLatencyAvgMS int64          `json:"approval_latency_avg_ms,omitempty"`
	ApprovalLatencyMaxMS int64          `json:"approval_latency_max_ms,omitempty"`
	InterventionCauses   map[string]int `json:"intervention_causes,omitempty"`
}
