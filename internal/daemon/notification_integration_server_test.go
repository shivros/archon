package daemon

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"control/internal/store"
)

// newNotificationIntegrationServer builds an integration server with
// notification capture wired through manager + live providers.
func newNotificationIntegrationServer(t *testing.T) (*httptest.Server, *SessionManager, *Stores, NotificationRecorder) {
	t.Helper()

	base := newDaemonIntegrationTempDir(t, "notification-server-*")
	manager, err := NewSessionManager(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}

	workspaces := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	state := store.NewFileAppStateStore(filepath.Join(base, "state.json"))
	meta := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))
	sessions := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))
	approvals := store.NewFileApprovalStore(filepath.Join(base, "approvals.json"))

	stores := &Stores{
		Workspaces:  workspaces,
		Worktrees:   workspaces,
		Groups:      workspaces,
		AppState:    state,
		SessionMeta: meta,
		Sessions:    sessions,
		Approvals:   approvals,
	}
	manager.SetMetaStore(meta)
	manager.SetSessionStore(sessions)

	logger := integrationTestLogger()
	artifactRepository := newFileSessionItemsRepository(manager)
	liveCodex := NewCodexLiveManager(stores, logger)
	turnNotifier := NewTurnCompletionNotifier(nil, stores)
	approvalStore := NewStoreApprovalStorage(stores)
	compositeLive := NewCompositeLiveManager(stores, logger,
		newCodexLiveSessionFactory(liveCodex),
		newClaudeLiveSessionFactory(manager, stores, nil, turnNotifier, logger),
		newOpenCodeLiveSessionFactory("opencode", turnNotifier, approvalStore, artifactRepository, nil, nil, logger),
		newOpenCodeLiveSessionFactory("kilocode", turnNotifier, approvalStore, artifactRepository, nil, nil, logger),
	)

	recorder := newCapturingNotificationRecorder()
	compositeLive.SetNotificationPublisher(recorder)
	liveCodex.SetNotificationPublisher(recorder)
	turnNotifier.SetNotificationPublisher(recorder)
	manager.SetNotificationPublisher(recorder)

	api := &API{
		Version:     "test",
		Manager:     manager,
		Stores:      stores,
		Logger:      logger,
		LiveCodex:   liveCodex,
		LiveManager: compositeLive,
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	return server, manager, stores, recorder
}
