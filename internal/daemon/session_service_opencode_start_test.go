package daemon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionServiceStartOpenCodeDoesNotBlockOnInitialPrompt(t *testing.T) {
	cases := []struct {
		name        string
		provider    string
		baseURLEnv  string
		providerID  string
		sessionPath string
	}{
		{
			name:        "opencode",
			provider:    "opencode",
			baseURLEnv:  "OPENCODE_BASE_URL",
			providerID:  "open-s-1",
			sessionPath: "/session/open-s-1/message",
		},
		{
			name:        "kilocode",
			provider:    "kilocode",
			baseURLEnv:  "KILOCODE_BASE_URL",
			providerID:  "kilo-s-1",
			sessionPath: "/session/kilo-s-1/message",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var promptCalls atomic.Int32
			promptStarted := make(chan struct{}, 1)
			unblockPrompt := make(chan struct{})

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/session":
					writeJSON(w, http.StatusCreated, map[string]any{"id": tc.providerID})
					return
				case r.Method == http.MethodGet && r.URL.Path == tc.sessionPath:
					writeJSON(w, http.StatusOK, []map[string]any{})
					return
				case r.Method == http.MethodPost && r.URL.Path == tc.sessionPath:
					promptCalls.Add(1)
					select {
					case promptStarted <- struct{}{}:
					default:
					}
					<-unblockPrompt
					writeJSON(w, http.StatusOK, map[string]any{
						"parts": []map[string]any{
							{"type": "text", "text": "ok"},
						},
					})
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
			service := NewSessionService(manager, nil, nil, nil)

			startDone := make(chan struct {
				sessionID string
				err       error
			}, 1)
			go func() {
				session, err := service.Start(context.Background(), StartSessionRequest{
					Provider: tc.provider,
					Cwd:      t.TempDir(),
					Text:     "hello async start",
				})
				result := struct {
					sessionID string
					err       error
				}{err: err}
				if session != nil {
					result.sessionID = session.ID
				}
				startDone <- result
			}()

			select {
			case result := <-startDone:
				if result.err != nil {
					close(unblockPrompt)
					t.Fatalf("Start: %v", result.err)
				}
				if strings.TrimSpace(result.sessionID) == "" {
					close(unblockPrompt)
					t.Fatalf("expected session id")
				}
			case <-time.After(500 * time.Millisecond):
				close(unblockPrompt)
				t.Fatalf("start blocked on initial prompt; expected immediate session creation")
			}

			select {
			case <-promptStarted:
				// expected async prompt dispatch after session creation
			case <-time.After(2 * time.Second):
				close(unblockPrompt)
				t.Fatalf("expected async initial prompt dispatch")
			}
			if promptCalls.Load() == 0 {
				close(unblockPrompt)
				t.Fatalf("expected prompt call")
			}

			close(unblockPrompt)
		})
	}
}
