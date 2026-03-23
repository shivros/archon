package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"control/internal/store"
	"control/internal/types"
)

type notificationIntegrationEnvironment struct {
	server              *httptest.Server
	manager             *SessionManager
	stores              *Stores
	live                *CompositeLiveManager
	recorder            NotificationRecorder
	dispatchProbe       NotificationDispatchProbe
	notificationService *NotificationService
	notifier            NotificationPublisher
}

type notificationEventProcessor interface {
	Process(ctx context.Context, event types.NotificationEvent)
}

func (e *notificationIntegrationEnvironment) Close() {
	if e == nil {
		return
	}
	if e.notificationService != nil {
		e.notificationService.Close()
	}
	if e.server != nil {
		e.server.Close()
	}
}

func (e *notificationIntegrationEnvironment) Publish(event types.NotificationEvent) {
	if e == nil || e.notifier == nil {
		return
	}
	e.notifier.Publish(event)
}

// newNotificationIntegrationServer builds an integration server with provider
// notification publication and real synchronous dispatch wiring.
func newNotificationIntegrationServer(t *testing.T) *notificationIntegrationEnvironment {
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
	dispatchProbe := newCapturingNotificationDispatchProbe()
	notificationDefaults := types.DefaultNotificationSettings()
	notificationDefaults.Methods = []types.NotificationMethod{types.NotificationMethodBell}
	notificationDefaults.ScriptCommands = nil
	notificationDefaults.ScriptTimeoutSeconds = 1
	notificationDefaults.DedupeWindowSeconds = 60

	service := NewNotificationService(
		NewNotificationPolicyResolver(notificationDefaultsFromCoreConfigForIntegration(notificationDefaults), stores, logger),
		NewNotificationDispatcher([]NotificationSink{newNotificationDispatchProbeSink(dispatchProbe)}, logger),
		logger,
	)
	processor := newNotificationServiceEventProcessor(service)
	notifier := newFanoutNotificationPublisher(
		recorder,
		newSynchronousNotificationServicePublisher(processor),
	)

	compositeLive.SetNotificationPublisher(notifier)
	liveCodex.SetNotificationPublisher(notifier)
	turnNotifier.SetNotificationPublisher(notifier)
	manager.SetNotificationPublisher(notifier)

	api := &API{
		Version:     "test",
		Manager:     manager,
		Stores:      stores,
		Logger:      logger,
		LiveCodex:   liveCodex,
		LiveManager: compositeLive,
		Notifier:    notifier,
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	return &notificationIntegrationEnvironment{
		server:              server,
		manager:             manager,
		stores:              stores,
		live:                compositeLive,
		recorder:            recorder,
		dispatchProbe:       dispatchProbe,
		notificationService: service,
		notifier:            notifier,
	}
}

func notificationDefaultsFromCoreConfigForIntegration(defaults types.NotificationSettings) types.NotificationSettings {
	settings := types.NormalizeNotificationSettings(defaults)
	settings.Enabled = true
	if len(settings.Triggers) == 0 {
		settings.Triggers = append([]types.NotificationTrigger{}, types.DefaultNotificationSettings().Triggers...)
	}
	return settings
}

type fanoutNotificationPublisher struct {
	publishers []NotificationPublisher
}

func newFanoutNotificationPublisher(publishers ...NotificationPublisher) NotificationPublisher {
	filtered := make([]NotificationPublisher, 0, len(publishers))
	for _, publisher := range publishers {
		if publisher != nil {
			filtered = append(filtered, publisher)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return fanoutNotificationPublisher{publishers: filtered}
}

func (p fanoutNotificationPublisher) Publish(event types.NotificationEvent) {
	for _, publisher := range p.publishers {
		if publisher == nil {
			continue
		}
		publisher.Publish(event)
	}
}

type synchronousNotificationServicePublisher struct {
	processor notificationEventProcessor
}

func newSynchronousNotificationServicePublisher(processor notificationEventProcessor) NotificationPublisher {
	if processor == nil {
		return nil
	}
	return synchronousNotificationServicePublisher{processor: processor}
}

func (p synchronousNotificationServicePublisher) Publish(event types.NotificationEvent) {
	if p.processor == nil {
		return
	}
	p.processor.Process(context.Background(), event)
}

type notificationServiceEventProcessor struct {
	service *NotificationService
}

func newNotificationServiceEventProcessor(service *NotificationService) notificationEventProcessor {
	if service == nil {
		return nil
	}
	return notificationServiceEventProcessor{service: service}
}

func (p notificationServiceEventProcessor) Process(ctx context.Context, event types.NotificationEvent) {
	if p.service == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.service.handle(ctx, event)
}
