package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"control/internal/store"
	"control/internal/types"
)

func TestIsCodexMissingThreadError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "rollout missing",
			err:  errors.New("RPC error -32600: No rollout found for thread ID thr_123"),
			want: true,
		},
		{
			name: "thread missing",
			err:  errors.New("thread not found"),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("permission denied"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCodexMissingThreadError(tt.err); got != tt.want {
				t.Fatalf("isCodexMissingThreadError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestReserveSessionTurnRejectsConcurrentStart(t *testing.T) {
	ls := &codexLiveSession{client: &codexAppServer{}}
	started := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	firstTurnID := ""
	firstErr := error(nil)
	go func() {
		defer wg.Done()
		firstTurnID, firstErr = reserveSessionTurn(context.Background(), ls, nil, func() (string, error) {
			close(started)
			<-release
			return "turn-1", nil
		})
	}()

	<-started
	secondTurnID := ""
	secondErr := error(nil)
	go func() {
		defer wg.Done()
		secondTurnID, secondErr = reserveSessionTurn(context.Background(), ls, nil, func() (string, error) {
			return "turn-2", nil
		})
	}()

	close(release)
	wg.Wait()

	if firstErr != nil || firstTurnID != "turn-1" {
		t.Fatalf("expected first start to succeed, got turn=%q err=%v", firstTurnID, firstErr)
	}
	if secondErr == nil || secondErr.Error() != "turn already in progress" {
		t.Fatalf("expected second start to fail with turn already in progress, got turn=%q err=%v", secondTurnID, secondErr)
	}
	if ls.activeTurn != "turn-1" {
		t.Fatalf("expected active turn to remain turn-1, got %q", ls.activeTurn)
	}
	if ls.lastActive.IsZero() || time.Since(ls.lastActive) > time.Minute {
		t.Fatalf("expected recent lastActive timestamp, got %v", ls.lastActive)
	}
}

func TestReserveSessionTurnClearsNothingOnStartError(t *testing.T) {
	ls := &codexLiveSession{client: &codexAppServer{}}
	_, err := reserveSessionTurn(context.Background(), ls, nil, func() (string, error) {
		return "", errors.New("start failed")
	})
	if err == nil || err.Error() != "start failed" {
		t.Fatalf("expected start error to propagate, got %v", err)
	}
	if ls.activeTurn != "" {
		t.Fatalf("expected active turn to stay empty on start failure, got %q", ls.activeTurn)
	}
}

func TestReserveSessionTurnBusyProbeActiveReturnsBusy(t *testing.T) {
	ls := &codexLiveSession{
		client:     &codexAppServer{},
		threadID:   "thr-1",
		activeTurn: "turn-1",
	}
	startCalls := 0
	_, err := reserveSessionTurn(context.Background(), ls, testTurnActivityProbe{
		status: turnActivityActive,
	}, func() (string, error) {
		startCalls++
		return "turn-2", nil
	})
	if err == nil || err.Error() != "turn already in progress" {
		t.Fatalf("expected busy error, got %v", err)
	}
	if startCalls != 0 {
		t.Fatalf("expected no start attempt while provider reports active, got %d", startCalls)
	}
	if ls.activeTurn != "turn-1" {
		t.Fatalf("expected active turn unchanged, got %q", ls.activeTurn)
	}
}

func TestReserveSessionTurnBusyProbeInactiveClearsStaleAndRetries(t *testing.T) {
	ls := &codexLiveSession{
		client:     &codexAppServer{},
		threadID:   "thr-1",
		activeTurn: "turn-stale",
	}
	startCalls := 0
	turnID, err := reserveSessionTurn(context.Background(), ls, testTurnActivityProbe{
		status: turnActivityInactive,
	}, func() (string, error) {
		startCalls++
		return "turn-fresh", nil
	})
	if err != nil {
		t.Fatalf("expected stale clear retry to succeed, got %v", err)
	}
	if turnID != "turn-fresh" {
		t.Fatalf("expected fresh turn id, got %q", turnID)
	}
	if startCalls != 1 {
		t.Fatalf("expected one start attempt after stale clear, got %d", startCalls)
	}
	if ls.activeTurn != "turn-fresh" {
		t.Fatalf("expected active turn updated to fresh turn, got %q", ls.activeTurn)
	}
}

func TestReserveSessionTurnBusyProbeUnknownReturnsBusy(t *testing.T) {
	ls := &codexLiveSession{
		client:     &codexAppServer{},
		threadID:   "thr-1",
		activeTurn: "turn-1",
	}
	startCalls := 0
	_, err := reserveSessionTurn(context.Background(), ls, testTurnActivityProbe{
		status: turnActivityUnknown,
	}, func() (string, error) {
		startCalls++
		return "turn-2", nil
	})
	if err == nil || err.Error() != "turn already in progress" {
		t.Fatalf("expected busy error for unknown probe status, got %v", err)
	}
	if startCalls != 0 {
		t.Fatalf("expected no start attempt for unknown probe status, got %d", startCalls)
	}
}

func TestReserveSessionTurnBusyProbeDoesNotClearWhenTurnChanged(t *testing.T) {
	ls := &codexLiveSession{
		client:     &codexAppServer{},
		threadID:   "thr-1",
		activeTurn: "turn-stale",
	}
	startCalls := 0
	_, err := reserveSessionTurn(context.Background(), ls, testTurnActivityProbe{
		status: turnActivityInactive,
		onProbe: func() {
			ls.mu.Lock()
			ls.activeTurn = "turn-newer"
			ls.mu.Unlock()
		},
	}, func() (string, error) {
		startCalls++
		return "turn-fresh", nil
	})
	if err == nil || err.Error() != "turn already in progress" {
		t.Fatalf("expected busy error when turn changed during probe, got %v", err)
	}
	if startCalls != 0 {
		t.Fatalf("expected no start attempt when active turn changed, got %d", startCalls)
	}
	if ls.activeTurn != "turn-newer" {
		t.Fatalf("expected active turn to remain latest value, got %q", ls.activeTurn)
	}
}

func TestCodexLiveSessionHandleNoteClearsActiveTurnBeforePublishingCompletion(t *testing.T) {
	ls := &codexLiveSession{
		sessionID:  "sess-1",
		client:     &codexAppServer{},
		hub:        newCodexSubscriberHub(),
		activeTurn: "turn-1",
	}
	probe := &activeTurnProbeNotifier{session: ls}
	ls.notifier = probe

	ls.handleNote(rpcMessage{
		Method: "turn/completed",
		Params: json.RawMessage(`{"turn":{"id":"turn-1"}}`),
	})

	if probe.activeTurnAtPublish != "" {
		t.Fatalf("expected active turn to be cleared before publish, got %q", probe.activeTurnAtPublish)
	}
	if ls.activeTurn != "" {
		t.Fatalf("expected active turn to stay cleared after completion, got %q", ls.activeTurn)
	}
}

func TestCodexLiveSessionHandleRequestPublishesApprovalNotification(t *testing.T) {
	notifier := &captureCodexNotificationPublisher{}
	ls := &codexLiveSession{
		sessionID: "sess-approval",
		client:    &codexAppServer{},
		hub:       newCodexSubscriberHub(),
		notifier:  notifier,
	}
	requestID := 42
	ls.handleRequest(rpcMessage{
		ID:     &requestID,
		Method: "item/commandExecution/requestApproval",
		Params: json.RawMessage(`{"command":"touch file.txt"}`),
	})

	if len(notifier.events) != 1 {
		t.Fatalf("expected one approval notification, got %d", len(notifier.events))
	}
	event := notifier.events[0]
	if event.Trigger != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected trigger: %q", event.Trigger)
	}
	if event.Status != "approval_required" {
		t.Fatalf("unexpected status: %q", event.Status)
	}
	if event.SessionID != "sess-approval" {
		t.Fatalf("unexpected session id: %q", event.SessionID)
	}
	if event.Source != "approval_request:sess-approval:42" {
		t.Fatalf("unexpected source: %q", event.Source)
	}
}

func TestCodexLiveStartTurnRecoversMissingThreadByCreatingNewThread(t *testing.T) {
	wrapper := codexLiveHelperWrapper(t)
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, ".archon"), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configText := "[providers.codex]\ncommand = \"" + wrapper + "\"\n"
	if err := os.WriteFile(filepath.Join(home, ".archon", "config.toml"), []byte(configText), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("GO_WANT_CODEX_LIVE_HELPER_PROCESS", "1")
	t.Setenv("ARCHON_CODEX_LIVE_HELPER_MODE", "resume_missing")

	base := t.TempDir()
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "session_meta.json"))
	stores := &Stores{SessionMeta: metaStore}
	live := NewCodexLiveManager(stores, nil)
	session := &types.Session{
		ID:       "sess-1",
		Provider: "codex",
		Cwd:      t.TempDir(),
	}
	initialMeta := &types.SessionMeta{
		SessionID: session.ID,
		ThreadID:  "thr-stale",
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model: "gpt-5",
		},
	}
	if _, err := metaStore.Upsert(context.Background(), initialMeta); err != nil {
		t.Fatalf("seed session meta: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	turnID, err := live.StartTurn(ctx, session, initialMeta, t.TempDir(), []map[string]any{
		{"type": "text", "text": "hello"},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if strings.TrimSpace(turnID) == "" {
		t.Fatalf("expected turn id")
	}
	updatedMeta, ok, err := metaStore.Get(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("load updated meta: %v", err)
	}
	if !ok || updatedMeta == nil {
		t.Fatalf("expected updated session meta")
	}
	if strings.TrimSpace(updatedMeta.ThreadID) == "" {
		t.Fatalf("expected recovered thread id")
	}
	if strings.TrimSpace(updatedMeta.ThreadID) == "thr-stale" {
		t.Fatalf("expected recovered thread id to replace stale thread id")
	}
	live.dropSession(session.ID)
}

func TestCodexLiveStartTurnRetriesTransientMissingRollout(t *testing.T) {
	wrapper := codexLiveHelperWrapper(t)
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(home, ".archon"), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configText := "[providers.codex]\ncommand = \"" + wrapper + "\"\n"
	if err := os.WriteFile(filepath.Join(home, ".archon", "config.toml"), []byte(configText), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("GO_WANT_CODEX_LIVE_HELPER_PROCESS", "1")
	t.Setenv("ARCHON_CODEX_LIVE_HELPER_MODE", "turn_missing_twice")

	base := t.TempDir()
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "session_meta.json"))
	stores := &Stores{SessionMeta: metaStore}
	live := NewCodexLiveManager(stores, nil)
	session := &types.Session{
		ID:       "sess-retry",
		Provider: "codex",
		Cwd:      t.TempDir(),
	}
	meta := &types.SessionMeta{
		SessionID: session.ID,
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model: "gpt-5",
		},
	}
	if _, err := metaStore.Upsert(context.Background(), meta); err != nil {
		t.Fatalf("seed session meta: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	turnID, err := live.StartTurn(ctx, session, meta, t.TempDir(), []map[string]any{
		{"type": "text", "text": "hello"},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if strings.TrimSpace(turnID) == "" {
		t.Fatalf("expected turn id")
	}
	live.dropSession(session.ID)
}

func codexLiveHelperWrapper(t *testing.T) string {
	t.Helper()
	testBin := os.Args[0]
	wrapper := filepath.Join(t.TempDir(), "codex-live-helper.sh")
	script := "#!/bin/sh\nexec \"" + testBin + "\" -test.run=TestCodexLiveHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	return wrapper
}

func TestCodexLiveHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CODEX_LIVE_HELPER_PROCESS") != "1" {
		return
	}
	mode := strings.TrimSpace(os.Getenv("ARCHON_CODEX_LIVE_HELPER_MODE"))
	type rpcErr struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	threads := map[string]struct{}{}
	threadSeq := 0
	turnSeq := 0
	turnMissingBudget := 0
	switch mode {
	case "turn_missing_once":
		turnMissingBudget = 1
	case "turn_missing_twice":
		turnMissingBudget = 2
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		method, _ := msg["method"].(string)
		idFloat, hasID := msg["id"].(float64)
		if !hasID {
			continue
		}
		id := int(idFloat)
		params, _ := msg["params"].(map[string]any)
		switch method {
		case "initialize":
			_ = encoder.Encode(map[string]any{"id": id, "result": map[string]any{"userAgent": "codex-live-helper"}})
		case "thread/start":
			threadSeq++
			threadID := fmt.Sprintf("thr-live-%d", threadSeq)
			threads[threadID] = struct{}{}
			_ = encoder.Encode(map[string]any{
				"id": id,
				"result": map[string]any{
					"thread": map[string]any{"id": threadID},
				},
			})
		case "thread/resume":
			threadID, _ := params["threadId"].(string)
			if mode == "resume_missing" {
				_ = encoder.Encode(map[string]any{"id": id, "error": rpcErr{Code: -32600, Message: "No rollout found for thread ID " + threadID}})
				continue
			}
			if _, ok := threads[strings.TrimSpace(threadID)]; !ok {
				_ = encoder.Encode(map[string]any{"id": id, "error": rpcErr{Code: -32600, Message: "No rollout found for thread ID " + threadID}})
				continue
			}
			_ = encoder.Encode(map[string]any{"id": id, "result": map[string]any{}})
		case "turn/start":
			threadID, _ := params["threadId"].(string)
			if turnMissingBudget > 0 {
				turnMissingBudget--
				_ = encoder.Encode(map[string]any{"id": id, "error": rpcErr{Code: -32600, Message: "No rollout found for thread ID " + threadID}})
				continue
			}
			if _, ok := threads[strings.TrimSpace(threadID)]; !ok {
				_ = encoder.Encode(map[string]any{"id": id, "error": rpcErr{Code: -32600, Message: "No rollout found for thread ID " + threadID}})
				continue
			}
			turnSeq++
			turnID := fmt.Sprintf("turn-live-%d", turnSeq)
			_ = encoder.Encode(map[string]any{
				"id": id,
				"result": map[string]any{
					"turn": map[string]any{"id": turnID},
				},
			})
			_ = encoder.Encode(map[string]any{
				"method": "turn/completed",
				"params": map[string]any{
					"turn": map[string]any{"id": turnID, "status": "completed"},
				},
			})
		default:
			_ = encoder.Encode(map[string]any{"id": id, "result": map[string]any{}})
		}
	}
	os.Exit(0)
}

type activeTurnProbeNotifier struct {
	session             *codexLiveSession
	activeTurnAtPublish string
}

func (n *activeTurnProbeNotifier) Publish(_ types.NotificationEvent) {
	if n == nil || n.session == nil {
		return
	}
	n.session.mu.Lock()
	n.activeTurnAtPublish = n.session.activeTurn
	n.session.mu.Unlock()
}

type testTurnActivityProbe struct {
	status  turnActivityStatus
	err     error
	onProbe func()
}

func (p testTurnActivityProbe) Probe(_ context.Context, _ codexTurnReader, _, _ string) (turnActivityStatus, error) {
	if p.onProbe != nil {
		p.onProbe()
	}
	return p.status, p.err
}

type captureCodexNotificationPublisher struct {
	events []types.NotificationEvent
}

func (p *captureCodexNotificationPublisher) Publish(event types.NotificationEvent) {
	p.events = append(p.events, event)
}
