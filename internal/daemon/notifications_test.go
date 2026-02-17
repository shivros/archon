package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"control/internal/config"
	"control/internal/logging"
	"control/internal/store"
	"control/internal/types"
)

func TestNotificationPolicyResolverSessionOverridesWorktree(t *testing.T) {
	tmp := t.TempDir()
	ctx := context.Background()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(tmp, "workspaces.json"))
	repoPath := filepath.Join(tmp, "repo")
	worktreePath := filepath.Join(tmp, "wt")
	if err := os.MkdirAll(repoPath, 0o700); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if err := os.MkdirAll(worktreePath, 0o700); err != nil {
		t.Fatalf("mkdir wt: %v", err)
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{Name: "repo", RepoPath: repoPath})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}
	disabled := false
	wt, err := workspaceStore.AddWorktree(ctx, ws.ID, &types.Worktree{
		Path: worktreePath,
		NotificationOverrides: &types.NotificationSettingsPatch{
			Enabled: &disabled,
			Methods: []types.NotificationMethod{types.NotificationMethodBell},
		},
	})
	if err != nil {
		t.Fatalf("add worktree: %v", err)
	}

	metaStore := store.NewFileSessionMetaStore(filepath.Join(tmp, "session_meta.json"))
	enabled := true
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   "sess-1",
		WorkspaceID: ws.ID,
		WorktreeID:  wt.ID,
		NotificationOverrides: &types.NotificationSettingsPatch{
			Enabled: &enabled,
			Methods: []types.NotificationMethod{types.NotificationMethodNotifySend},
		},
	}); err != nil {
		t.Fatalf("upsert meta: %v", err)
	}

	resolver := NewNotificationPolicyResolver(types.DefaultNotificationSettings(), &Stores{
		Worktrees:   workspaceStore,
		SessionMeta: metaStore,
	}, logging.Nop())

	settings := resolver.Resolve(context.Background(), types.NotificationEvent{SessionID: "sess-1", Trigger: types.NotificationTriggerTurnCompleted})
	if !settings.Enabled {
		t.Fatalf("expected session override to enable notifications")
	}
	if len(settings.Methods) == 0 || settings.Methods[0] != types.NotificationMethodNotifySend {
		t.Fatalf("expected session override methods, got %#v", settings.Methods)
	}
}

func TestNotificationPolicyResolverHonorsContextCancellation(t *testing.T) {
	defaults := types.DefaultNotificationSettings()
	defaults.Enabled = true
	defaults.Methods = []types.NotificationMethod{types.NotificationMethodBell}
	resolver := NewNotificationPolicyResolver(defaults, &Stores{
		SessionMeta: blockingSessionMetaStore{},
	}, logging.Nop())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	settings := resolver.Resolve(ctx, types.NotificationEvent{
		SessionID: "sess-timeout",
		Trigger:   types.NotificationTriggerTurnCompleted,
	})
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("expected resolver to return quickly with canceled context, took %s", elapsed)
	}
	if !settings.Enabled {
		t.Fatalf("expected defaults to remain enabled")
	}
	if len(settings.Methods) != 1 || settings.Methods[0] != types.NotificationMethodBell {
		t.Fatalf("expected default methods, got %#v", settings.Methods)
	}
}

func TestNotificationDispatcherRunsScriptWithPayload(t *testing.T) {
	tmp := t.TempDir()
	payloadPath := filepath.Join(tmp, "payload.json")
	dispatcher := NewNotificationDispatcher(nil, logging.Nop())
	event := types.NotificationEvent{
		Trigger:    types.NotificationTriggerTurnCompleted,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  "sess-script",
		Provider:   "claude",
	}
	settings := types.NotificationSettings{
		Enabled:              true,
		ScriptCommands:       []string{"cat > " + payloadPath},
		ScriptTimeoutSeconds: 2,
	}
	if err := dispatcher.Dispatch(context.Background(), event, settings); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "sess-script") {
		t.Fatalf("expected session id in payload, got %q", text)
	}
	if !strings.Contains(text, "turn.completed") {
		t.Fatalf("expected trigger in payload, got %q", text)
	}
}

func TestNotificationServiceStopWhilePublishing(t *testing.T) {
	resolver := stubNotificationPolicyResolver{settings: types.NotificationSettings{
		Enabled:  true,
		Triggers: []types.NotificationTrigger{types.NotificationTriggerTurnCompleted},
		Methods:  []types.NotificationMethod{types.NotificationMethodBell},
	}}
	dispatcher := &stubNotificationDispatcher{}
	service := NewNotificationService(resolver, dispatcher, logging.Nop())

	var wg sync.WaitGroup
	event := types.NotificationEvent{Trigger: types.NotificationTriggerTurnCompleted, SessionID: "sess-race"}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				service.Publish(event)
			}
		}()
	}

	time.Sleep(10 * time.Millisecond)
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := service.Stop(stopCtx); err != nil {
		t.Fatalf("stop notifier: %v", err)
	}
	wg.Wait()

	// Publishing after stop should be ignored and never panic.
	service.Publish(event)
}

func TestNotificationServiceCloseIdempotent(t *testing.T) {
	resolver := stubNotificationPolicyResolver{settings: types.NotificationSettings{
		Enabled:  true,
		Triggers: []types.NotificationTrigger{types.NotificationTriggerTurnCompleted},
		Methods:  []types.NotificationMethod{types.NotificationMethodBell},
	}}
	dispatcher := &stubNotificationDispatcher{}
	service := NewNotificationService(resolver, dispatcher, logging.Nop())

	service.Close()
	service.Close()
	service.Publish(types.NotificationEvent{Trigger: types.NotificationTriggerTurnCompleted, SessionID: "after-close"})
	time.Sleep(25 * time.Millisecond)

	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	if dispatcher.count != 0 {
		t.Fatalf("expected no dispatch after close, got %d", dispatcher.count)
	}
}

func TestNotificationDispatcherAutoFallback(t *testing.T) {
	var dunstifyCalls, notifySendCalls, bellCalls int
	dispatcher := NewNotificationDispatcher([]NotificationSink{
		stubNotificationSink{
			method: types.NotificationMethodDunstify,
			notify: func(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
				dunstifyCalls++
				return errors.New("dunstify unavailable")
			},
		},
		stubNotificationSink{
			method: types.NotificationMethodNotifySend,
			notify: func(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
				notifySendCalls++
				return nil
			},
		},
		stubNotificationSink{
			method: types.NotificationMethodBell,
			notify: func(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
				bellCalls++
				return nil
			},
		},
	}, logging.Nop())

	settings := types.NotificationSettings{
		Enabled: true,
		Methods: []types.NotificationMethod{types.NotificationMethodAuto},
	}
	err := dispatcher.Dispatch(context.Background(), types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-auto",
	}, settings)
	if err != nil {
		t.Fatalf("dispatch auto: %v", err)
	}
	if dunstifyCalls != 1 || notifySendCalls != 1 || bellCalls != 0 {
		t.Fatalf("unexpected sink calls: dunstify=%d notify-send=%d bell=%d", dunstifyCalls, notifySendCalls, bellCalls)
	}
}

func TestNotificationDispatcherUnknownMethod(t *testing.T) {
	dispatcher := NewNotificationDispatcher(nil, logging.Nop())
	settings := types.NotificationSettings{
		Enabled: true,
		Methods: []types.NotificationMethod{types.NotificationMethod("unknown-method")},
	}
	err := dispatcher.Dispatch(context.Background(), types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-unknown-method",
	}, settings)
	if err == nil || !strings.Contains(err.Error(), "unknown notification method") {
		t.Fatalf("expected unknown method error, got %v", err)
	}
}

func TestNotificationDispatcherAutoNoSinks(t *testing.T) {
	dispatcher := NewNotificationDispatcher(nil, logging.Nop())
	settings := types.NotificationSettings{
		Enabled: true,
		Methods: []types.NotificationMethod{types.NotificationMethodAuto},
	}
	err := dispatcher.Dispatch(context.Background(), types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-auto-no-sink",
	}, settings)
	if err == nil || !strings.Contains(err.Error(), "no notification sink available for auto") {
		t.Fatalf("expected auto fallback error, got %v", err)
	}
}

func TestDefaultNotificationSinks(t *testing.T) {
	sinks := defaultNotificationSinks()
	if len(sinks) != 3 {
		t.Fatalf("expected 3 default sinks, got %d", len(sinks))
	}
	seen := map[types.NotificationMethod]bool{}
	for _, sink := range sinks {
		if sink == nil {
			t.Fatalf("expected non-nil sink")
		}
		seen[sink.Method()] = true
	}
	if !seen[types.NotificationMethodDunstify] || !seen[types.NotificationMethodNotifySend] || !seen[types.NotificationMethodBell] {
		t.Fatalf("unexpected sink methods: %#v", seen)
	}
}

func TestNotificationDedupePolicyWindow(t *testing.T) {
	policy := newWindowNotificationDedupePolicy()
	settings := types.NotificationSettings{DedupeWindowSeconds: 10}
	event := types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-dedupe",
		TurnID:    "turn-1",
	}
	if policy.ShouldSuppress(event, settings) {
		t.Fatalf("expected first event not suppressed")
	}
	if !policy.ShouldSuppress(event, settings) {
		t.Fatalf("expected duplicate event to be suppressed")
	}
	key := notificationDedupeKey(event)
	windowPolicy, ok := policy.(*windowNotificationDedupePolicy)
	if !ok {
		t.Fatalf("expected concrete window policy")
	}
	windowPolicy.mu.Lock()
	windowPolicy.lastSent[key] = time.Now().UTC().Add(-11 * time.Second)
	windowPolicy.mu.Unlock()
	if policy.ShouldSuppress(event, settings) {
		t.Fatalf("expected event outside dedupe window not suppressed")
	}
}

func TestNotificationDedupeKeyUsesTurnIDThenStatusSource(t *testing.T) {
	withTurn := notificationDedupeKey(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess",
		TurnID:    "turn-a",
		Status:    "ignored",
		Source:    "ignored",
	})
	withoutTurn := notificationDedupeKey(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess",
		Status:    "done",
		Source:    "stream",
	})
	if withTurn != "turn.completed|sess|turn-a" {
		t.Fatalf("unexpected dedupe key with turn: %q", withTurn)
	}
	if withoutTurn != "turn.completed|sess|done|stream" {
		t.Fatalf("unexpected dedupe key without turn: %q", withoutTurn)
	}
}

func TestNotificationDedupeKeyGuidedWorkflowDecisionIncludesSource(t *testing.T) {
	key := notificationDedupeKey(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess",
		TurnID:    "turn-a",
		Status:    "decision_needed",
		Source:    "guided_workflow_decision:gwf-1:cd-1",
	})
	if key != "turn.completed|sess|turn-a|guided_workflow_decision:gwf-1:cd-1" {
		t.Fatalf("unexpected guided workflow decision dedupe key: %q", key)
	}
}

func TestNotificationTitleBody(t *testing.T) {
	cases := []struct {
		name         string
		trigger      types.NotificationTrigger
		wantSummary  string
		wantContains []string
	}{
		{
			name:        "turn completed",
			trigger:     types.NotificationTriggerTurnCompleted,
			wantSummary: "Archon turn completed",
		},
		{
			name:        "session failed",
			trigger:     types.NotificationTriggerSessionFailed,
			wantSummary: "Archon session failed",
		},
		{
			name:        "session killed",
			trigger:     types.NotificationTriggerSessionKilled,
			wantSummary: "Archon session killed",
		},
		{
			name:        "session exited",
			trigger:     types.NotificationTriggerSessionExited,
			wantSummary: "Archon session exited",
		},
		{
			name:        "fallback",
			trigger:     types.NotificationTrigger("custom.trigger"),
			wantSummary: "Archon notification",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			summary, body := notificationTitleBody(types.NotificationEvent{
				Trigger:   tc.trigger,
				SessionID: "sess-title",
				Provider:  "claude",
				Status:    "done",
			})
			if summary != tc.wantSummary {
				t.Fatalf("unexpected summary: got=%q want=%q", summary, tc.wantSummary)
			}
			if !strings.Contains(body, "sess-title (claude)") || !strings.Contains(body, "status: done") {
				t.Fatalf("unexpected body: %q", body)
			}
		})
	}
}

func TestNotificationTitleBodyGuidedWorkflowDecision(t *testing.T) {
	summary, body := notificationTitleBody(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-decision",
		Status:    "decision_needed",
		Source:    "guided_workflow_decision:gwf-1:cd-1",
		Payload: map[string]any{
			"reason":             "confidence_below_threshold",
			"risk_summary":       "severity=high tier=tier_2 score=0.62 pause_threshold=0.60",
			"recommended_action": "request_revision",
		},
	})
	if summary != "Archon workflow decision needed" {
		t.Fatalf("unexpected summary: %q", summary)
	}
	if !strings.Contains(body, "reason: confidence_below_threshold") {
		t.Fatalf("expected reason in body, got %q", body)
	}
	if !strings.Contains(body, "recommended: request_revision") {
		t.Fatalf("expected recommendation in body, got %q", body)
	}
}

func TestSessionLifecycleEmitterBackfillsMetaContext(t *testing.T) {
	publisher := &captureNotificationPublisher{}
	metaStore := &stubEmitterSessionMetaStore{
		meta: &types.SessionMeta{
			SessionID:   "sess-emitter",
			WorkspaceID: "ws-meta",
			WorktreeID:  "wt-meta",
		},
	}
	emitter := NewSessionLifecycleEmitter(publisher, metaStore, logging.Nop())
	if emitter == nil {
		t.Fatalf("expected emitter")
	}

	emitter.EmitSessionLifecycleEvent(context.Background(), &types.Session{
		ID:       "sess-emitter",
		Provider: "claude",
		Status:   types.SessionStatusExited,
	}, StartSessionConfig{}, types.SessionStatusExited, "test_emitter")

	events := publisher.Events()
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].WorkspaceID != "ws-meta" || events[0].WorktreeID != "wt-meta" {
		t.Fatalf("expected workspace/worktree from meta, got %q/%q", events[0].WorkspaceID, events[0].WorktreeID)
	}
}

func TestSessionLifecycleEmitterSkipsUnsupportedStatus(t *testing.T) {
	publisher := &captureNotificationPublisher{}
	emitter := NewSessionLifecycleEmitter(publisher, nil, logging.Nop())
	emitter.EmitSessionLifecycleEvent(context.Background(), &types.Session{
		ID:     "sess-running",
		Status: types.SessionStatusRunning,
	}, StartSessionConfig{}, types.SessionStatusRunning, "test_emitter")
	if got := len(publisher.Events()); got != 0 {
		t.Fatalf("expected no events for unsupported status, got %d", got)
	}
}

func TestSessionLifecycleEmitterPrefersConfigContext(t *testing.T) {
	publisher := &captureNotificationPublisher{}
	metaStore := &stubEmitterSessionMetaStore{
		meta: &types.SessionMeta{
			SessionID:   "sess-prefers-cfg",
			WorkspaceID: "ws-meta",
			WorktreeID:  "wt-meta",
		},
	}
	emitter := NewSessionLifecycleEmitter(publisher, metaStore, logging.Nop())
	emitter.EmitSessionLifecycleEvent(context.Background(), &types.Session{
		ID:     "sess-prefers-cfg",
		Status: types.SessionStatusExited,
	}, StartSessionConfig{
		WorkspaceID: "ws-cfg",
		WorktreeID:  "wt-cfg",
	}, types.SessionStatusExited, "test_emitter")

	if metaStore.getCalls != 0 {
		t.Fatalf("expected meta store not consulted when config IDs are present")
	}
	events := publisher.Events()
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].WorkspaceID != "ws-cfg" || events[0].WorktreeID != "wt-cfg" {
		t.Fatalf("expected config workspace/worktree, got %q/%q", events[0].WorkspaceID, events[0].WorktreeID)
	}
}

func TestNotificationDefaultsFromCoreConfig(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	enabled := false
	cfg.Notifications.Enabled = &enabled
	cfg.Notifications.Triggers = []string{" turn_completed ", "invalid", "turn.completed"}
	cfg.Notifications.Methods = []string{"notify_send", "invalid", "bell", "bell"}
	cfg.Notifications.ScriptCommands = []string{" echo hi ", "", "echo hi"}
	cfg.Notifications.ScriptTimeoutSeconds = 0
	cfg.Notifications.DedupeWindowSeconds = 0

	settings := notificationDefaultsFromCoreConfig(cfg)
	if settings.Enabled {
		t.Fatalf("expected notifications disabled")
	}
	if len(settings.Triggers) != 1 || settings.Triggers[0] != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected normalized triggers: %#v", settings.Triggers)
	}
	if len(settings.Methods) != 2 || settings.Methods[0] != types.NotificationMethodNotifySend || settings.Methods[1] != types.NotificationMethodBell {
		t.Fatalf("unexpected normalized methods: %#v", settings.Methods)
	}
	if len(settings.ScriptCommands) != 1 || settings.ScriptCommands[0] != "echo hi" {
		t.Fatalf("unexpected normalized scripts: %#v", settings.ScriptCommands)
	}
	if settings.ScriptTimeoutSeconds != 10 || settings.DedupeWindowSeconds != 5 {
		t.Fatalf("unexpected default timeout/dedupe: %d/%d", settings.ScriptTimeoutSeconds, settings.DedupeWindowSeconds)
	}
}

func TestSetSessionLifecycleEmitterUsesCustomEmitter(t *testing.T) {
	manager, err := NewSessionManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	emitter := &stubSessionLifecycleEmitter{}
	manager.SetSessionLifecycleEmitter(emitter)

	manager.publishSessionLifecycleEvent(&types.Session{
		ID:     "sess-custom-emitter",
		Status: types.SessionStatusExited,
	}, StartSessionConfig{}, types.SessionStatusExited, "test_emitter")

	if emitter.calls != 1 {
		t.Fatalf("expected custom emitter call, got %d", emitter.calls)
	}
}

func TestWithNotificationPublisherOption(t *testing.T) {
	publisher := &captureNotificationPublisher{}
	svc := &SessionService{}
	WithNotificationPublisher(publisher)(svc)
	if svc.notifier != publisher {
		t.Fatalf("expected notifier option to set service notifier")
	}
}

func TestCodexLiveManagerSetNotificationPublisher(t *testing.T) {
	publisher := &captureNotificationPublisher{}
	manager := NewCodexLiveManager(nil, logging.Nop())
	manager.SetNotificationPublisher(publisher)
	if manager.notifier != publisher {
		t.Fatalf("expected codex live manager notifier to be set")
	}
}

type stubNotificationPolicyResolver struct {
	settings types.NotificationSettings
}

func (r stubNotificationPolicyResolver) Resolve(ctx context.Context, event types.NotificationEvent) types.NotificationSettings {
	return r.settings
}

type stubNotificationDispatcher struct {
	mu    sync.Mutex
	count int
}

func (d *stubNotificationDispatcher) Dispatch(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
	d.mu.Lock()
	d.count++
	d.mu.Unlock()
	return nil
}

type blockingSessionMetaStore struct{}

func (blockingSessionMetaStore) List(ctx context.Context) ([]*types.SessionMeta, error) {
	return nil, nil
}

func (blockingSessionMetaStore) Get(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	<-ctx.Done()
	return nil, false, ctx.Err()
}

func (blockingSessionMetaStore) Upsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	return meta, nil
}

func (blockingSessionMetaStore) Delete(ctx context.Context, sessionID string) error {
	return nil
}

type stubNotificationSink struct {
	method types.NotificationMethod
	notify func(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error
}

func (s stubNotificationSink) Method() types.NotificationMethod {
	return s.method
}

func (s stubNotificationSink) Notify(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
	if s.notify != nil {
		return s.notify(ctx, event, settings)
	}
	return nil
}

type captureNotificationPublisher struct {
	mu     sync.Mutex
	events []types.NotificationEvent
}

func (p *captureNotificationPublisher) Publish(event types.NotificationEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *captureNotificationPublisher) Events() []types.NotificationEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]types.NotificationEvent, len(p.events))
	copy(out, p.events)
	return out
}

type stubEmitterSessionMetaStore struct {
	meta     *types.SessionMeta
	err      error
	getCalls int
}

func (s *stubEmitterSessionMetaStore) List(ctx context.Context) ([]*types.SessionMeta, error) {
	return nil, nil
}

func (s *stubEmitterSessionMetaStore) Get(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	s.getCalls++
	if s.err != nil {
		return nil, false, s.err
	}
	if s.meta == nil {
		return nil, false, nil
	}
	return s.meta, true, nil
}

func (s *stubEmitterSessionMetaStore) Upsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	return meta, nil
}

func (s *stubEmitterSessionMetaStore) Delete(ctx context.Context, sessionID string) error {
	return nil
}

type stubSessionLifecycleEmitter struct {
	calls int
}

func (s *stubSessionLifecycleEmitter) EmitSessionLifecycleEvent(ctx context.Context, session *types.Session, cfg StartSessionConfig, status types.SessionStatus, source string) {
	s.calls++
}
