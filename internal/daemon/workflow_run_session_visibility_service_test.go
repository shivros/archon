package daemon

import (
	"path/filepath"
	"testing"

	"control/internal/store"
)

func TestNewWorkflowRunSessionVisibilitySyncService(t *testing.T) {
	if got := newWorkflowRunSessionVisibilitySyncService(nil, nil); got != nil {
		t.Fatalf("expected nil service when stores are nil")
	}
	if got := newWorkflowRunSessionVisibilitySyncService(&Stores{}, nil); got != nil {
		t.Fatalf("expected nil service when session meta store is missing")
	}

	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json"))
	service := newWorkflowRunSessionVisibilitySyncService(&Stores{SessionMeta: metaStore}, nil)
	if service == nil {
		t.Fatalf("expected sync service when session meta store is configured")
	}

	syncer, ok := service.(*workflowRunSessionVisibilitySyncService)
	if !ok {
		t.Fatalf("expected concrete sync service type, got %T", service)
	}
	if syncer.sessionMeta == nil {
		t.Fatalf("expected sync service to retain session meta store")
	}
}
