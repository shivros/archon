package daemon

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

func openSSE(t *testing.T, server *httptest.Server, path string) (<-chan string, func()) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open sse: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("open sse status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "data: ") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
				if payload != "" {
					ch <- payload
				}
			}
		}
	}()
	closeFn := func() {
		_ = resp.Body.Close()
	}
	return ch, closeFn
}

func waitForSSEData(ch <-chan string, timeout time.Duration) (string, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return "", false
			}
			return data, true
		case <-time.After(200 * time.Millisecond):
		}
	}
	return "", false
}

func collectEvents(ch <-chan string, timeout time.Duration) []types.CodexEvent {
	deadline := time.Now().Add(timeout)
	out := make([]types.CodexEvent, 0)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return out
			}
			var event types.CodexEvent
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				out = append(out, event)
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	return out
}

func waitForEvent(ch <-chan string, method string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case data, ok := <-ch:
			if !ok {
				return false
			}
			var event types.CodexEvent
			if err := json.Unmarshal([]byte(data), &event); err == nil {
				if event.Method == method {
					return true
				}
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	return false
}
