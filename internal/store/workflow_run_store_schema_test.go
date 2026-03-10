package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"control/internal/guidedworkflows"
)

func TestFileWorkflowRunStoreReadsV1AndWritesV2(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "workflow_runs.json")
	legacy := struct {
		Version int                                 `json:"version"`
		Runs    []guidedworkflows.RunStatusSnapshot `json:"runs"`
	}{
		Version: 1,
		Runs: []guidedworkflows.RunStatusSnapshot{
			{
				Run: &guidedworkflows.WorkflowRun{
					ID:        "gwf-legacy",
					Status:    guidedworkflows.WorkflowRunStatusCreated,
					CreatedAt: time.Now().UTC(),
				},
			},
		},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy payload: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write legacy payload: %v", err)
	}

	store := NewFileWorkflowRunStore(path)
	snapshots, err := store.ListWorkflowRuns(context.Background())
	if err != nil {
		t.Fatalf("list legacy snapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].Run == nil || snapshots[0].Run.ID != "gwf-legacy" {
		t.Fatalf("unexpected legacy snapshots: %#v", snapshots)
	}
	if err := store.UpsertWorkflowRun(context.Background(), snapshots[0]); err != nil {
		t.Fatalf("upsert migrated snapshot: %v", err)
	}

	var saved struct {
		Version int `json:"version"`
	}
	if err := readJSON(path, &saved); err != nil {
		t.Fatalf("read saved payload: %v", err)
	}
	if saved.Version != workflowRunSchemaVersion {
		t.Fatalf("expected saved schema version %d, got %d", workflowRunSchemaVersion, saved.Version)
	}
}

func TestRunSnapshotSortTimeUsesMostRecentTimestamp(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC)
	started := created.Add(1 * time.Minute)
	paused := created.Add(2 * time.Minute)
	completed := created.Add(3 * time.Minute)
	dismissed := created.Add(4 * time.Minute)

	snapshot := guidedworkflows.RunStatusSnapshot{
		Run: &guidedworkflows.WorkflowRun{
			ID:          "gwf-1",
			CreatedAt:   created,
			StartedAt:   &started,
			PausedAt:    &paused,
			CompletedAt: &completed,
			DismissedAt: &dismissed,
		},
	}
	if got := runSnapshotSortTime(snapshot); !got.Equal(dismissed) {
		t.Fatalf("expected dismissed_at to drive sort time, got %s", got)
	}
}
