package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"control/internal/types"
)

func TestSessionServiceStartOpenCodeDoesNotBlockOnInitialPrompt(t *testing.T) {
	cases := []struct {
		name       string
		provider   string
		baseURLEnv string
		providerID string
	}{
		{
			name:       "opencode",
			provider:   "opencode",
			baseURLEnv: "OPENCODE_BASE_URL",
			providerID: "open-s-1",
		},
		{
			name:       "kilocode",
			provider:   "kilocode",
			baseURLEnv: "KILOCODE_BASE_URL",
			providerID: "kilo-s-1",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/session":
					writeJSON(w, http.StatusCreated, map[string]any{"id": tc.providerID})
					return
				default:
					http.NotFound(w, r)
					return
				}
			}))
			defer server.Close()

			t.Setenv("HOME", t.TempDir())
			t.Setenv(tc.baseURLEnv, server.URL)
			rememberOpenCodeRuntimeBaseURL(tc.provider, server.URL)

			manager := newTestManager(t)
			lm := &asyncStubLiveManager{
				turnID: "turn-init",
			}
			service := NewSessionService(manager, nil, nil, WithLiveManager(lm))

			session, err := service.Start(context.Background(), StartSessionRequest{
				Provider: tc.provider,
				Cwd:      t.TempDir(),
				Text:     "hello async start",
			})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			if strings.TrimSpace(session.ID) == "" {
				t.Fatalf("expected session id")
			}

			// Wait for the async goroutine to dispatch via LiveManager.StartTurn
			deadline := time.After(2 * time.Second)
			for {
				if lm.calls() > 0 {
					break
				}
				select {
				case <-deadline:
					t.Fatalf("expected async initial send via LiveManager.StartTurn")
				case <-time.After(10 * time.Millisecond):
				}
			}

			if lm.calls() != 1 {
				t.Fatalf("expected 1 StartTurn call, got %d", lm.calls())
			}
		})
	}
}

// asyncStubLiveManager is a thread-safe stub for verifying async StartTurn calls.
type asyncStubLiveManager struct {
	mu             sync.Mutex
	startTurnCalls int
	turnID         string
}

func (s *asyncStubLiveManager) StartTurn(_ context.Context, _ *types.Session, _ *types.SessionMeta, _ []map[string]any, _ *types.SessionRuntimeOptions) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startTurnCalls++
	return s.turnID, nil
}

func (s *asyncStubLiveManager) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startTurnCalls
}

func (s *asyncStubLiveManager) Subscribe(_ *types.Session, _ *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}, nil
}

func (s *asyncStubLiveManager) Respond(context.Context, *types.Session, *types.SessionMeta, int, map[string]any) error {
	return nil
}

func (s *asyncStubLiveManager) Interrupt(context.Context, *types.Session, *types.SessionMeta) error {
	return nil
}

func (s *asyncStubLiveManager) SetNotificationPublisher(NotificationPublisher) {}
