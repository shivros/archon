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

	sink := newLogSink(stdoutFile, stderrFile, newLogBuffer(logBufferMaxBytes), newLogBuffer(logBufferMaxBytes), nil)
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

func TestCodexProviderStartSkipsInitialTurnWhenInputIsEmpty(t *testing.T) {
	wrapper := codexProviderHelperWrapper(t)
	sink := &testProviderLogSink{}
	provider := &codexProvider{cmdName: wrapper, model: "gpt-5"}

	proc, err := provider.Start(StartSessionConfig{
		Cwd: t.TempDir(),
		Env: []string{
			"GO_WANT_CODEX_PROVIDER_HELPER_PROCESS=1",
			"ARCHON_CODEX_HELPER_FAIL_ON_TURN_START=1",
		},
	}, sink, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if proc == nil || proc.Process == nil {
		t.Fatalf("expected provider process")
	}
	if strings.TrimSpace(proc.ThreadID) == "" {
		t.Fatalf("expected thread id")
	}
	stopProviderProcess(proc)
}

func TestCodexProviderStartRunsInitialTurnWhenInputProvided(t *testing.T) {
	wrapper := codexProviderHelperWrapper(t)
	sink := &testProviderLogSink{}
	provider := &codexProvider{cmdName: wrapper, model: "gpt-5"}
	inputFile := filepath.Join(t.TempDir(), "turn-input.txt")
	initialInput := "workflow bootstrap prompt"

	proc, err := provider.Start(StartSessionConfig{
		Cwd:  t.TempDir(),
		Args: []string{initialInput},
		Env: []string{
			"GO_WANT_CODEX_PROVIDER_HELPER_PROCESS=1",
			"ARCHON_CODEX_HELPER_TURN_INPUT_FILE=" + inputFile,
		},
	}, sink, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if proc == nil || proc.Process == nil {
		t.Fatalf("expected provider process")
	}
	if strings.TrimSpace(proc.ThreadID) == "" {
		t.Fatalf("expected thread id")
	}
	got, err := os.ReadFile(filepath.Clean(inputFile))
	if err != nil {
		t.Fatalf("read turn input file: %v", err)
	}
	if strings.TrimSpace(string(got)) != initialInput {
		t.Fatalf("unexpected turn input payload: %q", strings.TrimSpace(string(got)))
	}
	stopProviderProcess(proc)
}

func TestCodexProviderStartIncludesAdditionalDirectoryArgs(t *testing.T) {
	wrapper := codexProviderHelperWrapper(t)
	sink := &testProviderLogSink{}
	provider := &codexProvider{cmdName: wrapper, model: "gpt-5"}
	argsFile := filepath.Join(t.TempDir(), "codex-args.txt")

	proc, err := provider.Start(StartSessionConfig{
		Cwd:                   t.TempDir(),
		AdditionalDirectories: []string{"/tmp/backend", "/tmp/shared"},
		Env: []string{
			"GO_WANT_CODEX_PROVIDER_HELPER_PROCESS=1",
			"ARCHON_CODEX_HELPER_ARGS_FILE=" + argsFile,
		},
	}, sink, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if proc == nil || proc.Process == nil {
		t.Fatalf("expected provider process")
	}
	got, err := os.ReadFile(filepath.Clean(argsFile))
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	args := string(got)
	if !strings.Contains(args, "--add-dir") {
		t.Fatalf("expected --add-dir flag, got %q", args)
	}
	if !strings.Contains(args, "/tmp/backend") || !strings.Contains(args, "/tmp/shared") {
		t.Fatalf("expected additional directory args, got %q", args)
	}
	stopProviderProcess(proc)
}

func codexProviderHelperWrapper(t *testing.T) string {
	t.Helper()
	testBin := os.Args[0]
	wrapper := filepath.Join(t.TempDir(), "codex-provider-helper.sh")
	script := "#!/bin/sh\nexec \"" + testBin + "\" -test.run=TestCodexProviderHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatalf("write helper wrapper: %v", err)
	}
	return wrapper
}

func stopProviderProcess(proc *providerProcess) {
	if proc == nil {
		return
	}
	if proc.Process != nil {
		_ = proc.Process.Kill()
	}
	if proc.Wait != nil {
		_ = proc.Wait()
	}
}

func TestCodexProviderHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CODEX_PROVIDER_HELPER_PROCESS") != "1" {
		return
	}
	if argsFile := strings.TrimSpace(os.Getenv("ARCHON_CODEX_HELPER_ARGS_FILE")); argsFile != "" {
		_ = os.WriteFile(argsFile, []byte(strings.Join(helperProcessArgs(), "\n")), 0o600)
	}
	failOnTurnStart := os.Getenv("ARCHON_CODEX_HELPER_FAIL_ON_TURN_START") == "1"
	turnInputFile := strings.TrimSpace(os.Getenv("ARCHON_CODEX_HELPER_TURN_INPUT_FILE"))

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
		id, hasID := msg["id"].(float64)
		if !hasID {
			continue
		}
		switch method {
		case "initialize":
			_ = encoder.Encode(map[string]any{
				"id": int(id),
				"result": map[string]any{
					"userAgent": "codex-helper",
				},
			})
		case "thread/start":
			_ = encoder.Encode(map[string]any{
				"id": int(id),
				"result": map[string]any{
					"thread": map[string]any{
						"id": "thr-helper",
					},
				},
			})
		case "turn/start":
			if failOnTurnStart {
				_ = encoder.Encode(map[string]any{
					"id": int(id),
					"error": map[string]any{
						"code":    -32600,
						"message": "unexpected turn/start",
					},
				})
				continue
			}
			if turnInputFile != "" {
				text := helperTurnInputText(msg["params"])
				_ = os.WriteFile(turnInputFile, []byte(text), 0o600)
			}
			_ = encoder.Encode(map[string]any{
				"id": int(id),
				"result": map[string]any{
					"turn": map[string]any{
						"id": "turn-helper",
					},
				},
			})
		default:
			_ = encoder.Encode(map[string]any{
				"id":     int(id),
				"result": map[string]any{},
			})
		}
	}
	os.Exit(0)
}

func helperTurnInputText(raw any) string {
	params, _ := raw.(map[string]any)
	if params == nil {
		return ""
	}
	input, _ := params["input"].([]any)
	if len(input) == 0 {
		return ""
	}
	first, _ := input[0].(map[string]any)
	if first == nil {
		return ""
	}
	text, _ := first["text"].(string)
	return strings.TrimSpace(text)
}

func helperProcessArgs() []string {
	args := os.Args
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep+1 >= len(args) {
		return nil
	}
	return append([]string(nil), args[sep+1:]...)
}
