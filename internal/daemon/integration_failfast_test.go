package daemon

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

func waitForSSEDataWithFailure(ch <-chan string, failures <-chan string, timeout time.Duration) (string, string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case failure, ok := <-failures:
			if ok && strings.TrimSpace(failure) != "" {
				return "", failure, false
			}
		case data, ok := <-ch:
			if !ok {
				return "", "", false
			}
			return data, "", true
		case <-time.After(200 * time.Millisecond):
		}
	}
	return "", "", false
}

func startSessionTurnFailureMonitor(server *httptest.Server, sessionID string) (<-chan string, func()) {
	failures := make(chan string, 8)
	closeFn := func() { close(failures) }
	if server == nil || strings.TrimSpace(sessionID) == "" {
		return failures, closeFn
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/sessions/"+sessionID+"/events?follow=1", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return failures, closeFn
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return failures, closeFn
	}

	stop := sync.OnceFunc(func() {
		_ = resp.Body.Close()
		close(failures)
	})
	go func() {
		defer stop()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" {
				continue
			}
			var event types.CodexEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				continue
			}
			if failure := turnFailureFromEvent(event); failure != "" {
				select {
				case failures <- failure:
				default:
				}
			}
		}
	}()
	return failures, stop
}

func turnFailureFromEvent(event types.CodexEvent) string {
	switch strings.TrimSpace(strings.ToLower(event.Method)) {
	case "error", "codex/event/error", "codex/event/stream_error":
		if len(event.Params) == 0 {
			return strings.TrimSpace(event.Method)
		}
		var payload map[string]any
		if err := json.Unmarshal(event.Params, &payload); err != nil {
			return strings.TrimSpace(event.Method)
		}
		if msg := extractIntegrationErrorMessage(payload["error"]); msg != "" {
			return msg
		}
		return strings.TrimSpace(event.Method)
	case "turn/completed":
		turn := parseTurnEventFromParams(event.Params)
		status := strings.TrimSpace(strings.ToLower(turn.Status))
		if status == "failed" || status == "error" || status == "cancelled" || status == "canceled" || status == "interrupted" {
			if msg := strings.TrimSpace(turn.Error); msg != "" {
				return "turn " + status + ": " + msg
			}
			return "turn " + status
		}
		if status == "" && strings.TrimSpace(turn.Error) != "" {
			return "turn error: " + strings.TrimSpace(turn.Error)
		}
	}
	return ""
}

func extractIntegrationErrorMessage(raw any) string {
	if raw == nil {
		return ""
	}
	switch val := raw.(type) {
	case map[string]any:
		if msg, ok := val["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	case string:
		return strings.TrimSpace(val)
	}
	return ""
}

func sessionTerminalFailure(server *httptest.Server, sessionID string) string {
	if server == nil || strings.TrimSpace(sessionID) == "" {
		return ""
	}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/sessions/"+sessionID, nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var session types.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return ""
	}
	switch session.Status {
	case types.SessionStatusFailed, types.SessionStatusKilled, types.SessionStatusOrphaned:
		return string(session.Status)
	default:
		return ""
	}
}
