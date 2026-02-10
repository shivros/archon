package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexControllerFlow(t *testing.T) {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	stdoutFile, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("stdout file: %v", err)
	}
	stderrFile, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("stderr file: %v", err)
	}
	defer stdoutFile.Close()
	defer stderrFile.Close()

	sink := newLogSink(stdoutFile, stderrFile, newLogBuffer(logBufferMaxBytes), newLogBuffer(logBufferMaxBytes))
	controller := newCodexController(stdinWriter, stdoutReader, sink)
	go controller.readLoop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdinReader)
		encoder := json.NewEncoder(stdoutWriter)
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
			id, hasID := msg["id"].(float64)
			switch method {
			case "initialize":
				if hasID {
					_ = encoder.Encode(map[string]any{"id": int(id), "result": map[string]any{"userAgent": "codex-test"}})
				}
			case "initialized":
			case "thread/start":
				if hasID {
					_ = encoder.Encode(map[string]any{"id": int(id), "result": map[string]any{"thread": map[string]any{"id": "thr_test"}}})
				}
			case "turn/start":
				if hasID {
					_ = encoder.Encode(map[string]any{"id": int(id), "result": map[string]any{"turn": map[string]any{"id": "turn_test"}}})
				}
				_ = encoder.Encode(map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{"delta": "hello from codex\n"}})
				_ = encoder.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"id": "turn_test", "status": "completed"}}})
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := controller.initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	threadID, err := controller.startThread(ctx, "gpt-5.1-codex", "", nil)
	if err != nil {
		t.Fatalf("thread start: %v", err)
	}
	if threadID != "thr_test" {
		t.Fatalf("unexpected thread id: %s", threadID)
	}
	turnID, err := controller.startTurn(ctx, threadID, "Hello", nil, "gpt-5.1-codex")
	if err != nil {
		t.Fatalf("turn start: %v", err)
	}
	if turnID != "turn_test" {
		t.Fatalf("unexpected turn id: %s", turnID)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not finish")
	}

	data, err := os.ReadFile(filepath.Clean(stdoutFile.Name()))
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if !strings.Contains(string(data), "hello from codex") {
		t.Fatalf("expected stdout to contain codex output")
	}
}
