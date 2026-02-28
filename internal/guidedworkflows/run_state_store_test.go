package guidedworkflows

import (
	"sync"
	"testing"
	"time"
)

func TestMemoryRunStateStoreGetClonedAndListCloned(t *testing.T) {
	store := NewMemoryRunStateStore()
	now := time.Now().UTC()
	run := &WorkflowRun{
		ID:        "gwf-1",
		Status:    WorkflowRunStatusRunning,
		CreatedAt: now,
		AuditTrail: []RunAuditEntry{
			{At: now, Action: "created"},
		},
	}
	store.Set(run.ID, run)
	store.SetTimeline(run.ID, []RunTimelineEvent{{At: now, Type: "run_created", RunID: run.ID}})

	cloned, ok := store.GetCloned(run.ID)
	if !ok || cloned == nil {
		t.Fatalf("expected cloned run")
	}
	cloned.Status = WorkflowRunStatusFailed
	cloned.AuditTrail[0].Action = "mutated"

	original, _ := store.Get(run.ID)
	if original.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected original status unchanged, got %q", original.Status)
	}
	if original.AuditTrail[0].Action != "created" {
		t.Fatalf("expected original audit unchanged, got %q", original.AuditTrail[0].Action)
	}

	list := store.ListCloned(false)
	if len(list) != 1 || list[0].ID != run.ID {
		t.Fatalf("expected one cloned run in list, got %#v", list)
	}
}

func TestSharedLockedRunStateStoreGetTimelineCloned(t *testing.T) {
	mu := &sync.RWMutex{}
	runs := map[string]*WorkflowRun{
		"gwf-1": {ID: "gwf-1", Status: WorkflowRunStatusRunning, CreatedAt: time.Now().UTC()},
	}
	timelines := map[string][]RunTimelineEvent{
		"gwf-1": {{Type: "run_created", RunID: "gwf-1"}},
	}
	store := NewSharedLockedRunStateStore(mu, runs, timelines)
	cloned := store.GetTimelineCloned("gwf-1")
	if len(cloned) != 1 {
		t.Fatalf("expected one timeline entry, got %d", len(cloned))
	}
	cloned[0].Type = "mutated"
	if timelines["gwf-1"][0].Type != "run_created" {
		t.Fatalf("expected original timeline unchanged, got %q", timelines["gwf-1"][0].Type)
	}
}
