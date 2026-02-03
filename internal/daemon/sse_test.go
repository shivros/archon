package daemon

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

func TestSSETailStream(t *testing.T) {
	manager := newTestManager(t)
	server := newTestServer(t, manager)
	defer server.Close()

	cfg := StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=hello", "sleep_ms=50", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
	}
	session, err := manager.StartSession(cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/sessions/"+session.ID+"/tail?follow=1&stream=stdout", nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request sse: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	eventCh := make(chan types.LogEvent, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				payload := strings.TrimSpace(line[len("data:"):])
				var event types.LogEvent
				if err := json.Unmarshal([]byte(payload), &event); err == nil {
					eventCh <- event
					return
				}
			}
		}
	}()

	select {
	case event := <-eventCh:
		if event.Stream != "stdout" {
			t.Fatalf("expected stdout event, got %s", event.Stream)
		}
		if !strings.Contains(event.Chunk, "hello") {
			t.Fatalf("expected chunk to contain hello, got %q", event.Chunk)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for sse event")
	}
}
