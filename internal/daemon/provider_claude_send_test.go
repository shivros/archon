package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type testItemSink struct {
	mu    sync.Mutex
	items []map[string]any
}

func (s *testItemSink) Append(item map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append(s.items, item)
}

func (s *testItemSink) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func (s *testItemSink) Snapshot() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]map[string]any, len(s.items))
	for i, item := range s.items {
		cloned := make(map[string]any, len(item))
		for k, v := range item {
			cloned[k] = v
		}
		items[i] = cloned
	}
	return items
}

func TestClaudeRunnerSendValidation(t *testing.T) {
	var nilRunner *claudeRunner
	if err := nilRunner.Send([]byte(`{"type":"user"}`)); err == nil {
		t.Fatalf("expected nil runner error")
	}

	runner := &claudeRunner{}
	if err := runner.Send(nil); err == nil {
		t.Fatalf("expected payload required error")
	}
	if err := runner.Send([]byte("{broken")); err == nil {
		t.Fatalf("expected invalid json error")
	}
	if err := runner.Send([]byte(`{"type":"assistant"}`)); err == nil {
		t.Fatalf("expected unsupported type error")
	}
	if err := runner.Send(buildClaudeUserPayloadWithRuntime("   ", nil)); err == nil {
		t.Fatalf("expected text required error")
	}
}

func TestClaudeRunnerUpdateSessionIDNotifiesOnChange(t *testing.T) {
	var updates []string
	runner := &claudeRunner{
		onSession: func(id string) {
			updates = append(updates, id)
		},
	}
	runner.updateSessionID(" session-1 ")
	runner.updateSessionID("session-1")
	runner.updateSessionID("session-2")
	runner.updateSessionID("")

	if got := runner.getSessionID(); got != "session-2" {
		t.Fatalf("unexpected session id: %q", got)
	}
	if len(updates) != 2 || updates[0] != "session-1" || updates[1] != "session-2" {
		t.Fatalf("unexpected session id updates: %#v", updates)
	}
}

func TestReadClaudeStreamParseErrorAndSessionID(t *testing.T) {
	logSink := &testProviderLogSink{}
	itemSink := &testItemSink{}
	var sessionID string

	stream := strings.NewReader("not-json\n" +
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"abc123\"}\n")
	if err := readClaudeStream(stream, "provider_stdout_raw", logSink, itemSink, func(id string) {
		sessionID = id
	}); err != nil {
		t.Fatalf("readClaudeStream: %v", err)
	}

	if sessionID != "abc123" {
		t.Fatalf("expected session id callback, got %q", sessionID)
	}
	items := itemSink.Snapshot()
	if len(items) < 2 {
		t.Fatalf("expected parse error log + system item, got %d items", len(items))
	}
	firstType, _ := items[0]["type"].(string)
	if firstType != "log" {
		t.Fatalf("expected first item to be parse log, got %#v", items[0])
	}
	if !strings.Contains(logSink.stderr.String(), "claude parse error") {
		t.Fatalf("expected parse error in stderr sink, got %q", logSink.stderr.String())
	}
}

func TestBuildClaudeUserPayloadAndExtractText(t *testing.T) {
	payload := buildClaudeUserPayload("hello world")
	text, err := extractClaudeUserText(payload)
	if err != nil {
		t.Fatalf("extractClaudeUserText: %v", err)
	}
	if text != "hello world" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestClaudeRunnerAppendUserItem(t *testing.T) {
	items := &testItemSink{}
	runner := &claudeRunner{items: items}
	runner.appendUserItem("hello")
	if items.Len() != 1 {
		t.Fatalf("expected one item, got %d", items.Len())
	}
	snapshot := items.Snapshot()
	if snapshot[0]["type"] != "userMessage" {
		t.Fatalf("unexpected item type: %#v", snapshot[0]["type"])
	}
}

func TestClaudeProviderStartValidationAndLifecycle(t *testing.T) {
	if _, err := newClaudeProvider("   "); err == nil {
		t.Fatalf("expected empty command validation error")
	}
	provider, err := newClaudeProvider("claude")
	if err != nil {
		t.Fatalf("newClaudeProvider: %v", err)
	}
	if _, err := provider.Start(StartSessionConfig{
		Resume: true,
	}, &testProviderLogSink{}, &testItemSink{}); err == nil {
		t.Fatalf("expected resume validation error")
	}

	proc, err := provider.Start(StartSessionConfig{}, &testProviderLogSink{}, &testItemSink{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- proc.Wait()
	}()
	select {
	case <-waitDone:
		t.Fatalf("wait should block until interrupt")
	case <-time.After(20 * time.Millisecond):
	}
	if err := proc.Interrupt(); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("Wait: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("wait did not unblock after interrupt")
	}
}

func TestClaudeRunnerSendUser(t *testing.T) {
	testBin := os.Args[0]
	wrapper := filepath.Join(t.TempDir(), "claude-wrapper.sh")
	wrapperScript := "#!/bin/sh\nexec \"" + testBin + "\" -test.run=TestHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(wrapperScript), 0o755); err != nil {
		t.Fatalf("WriteFile wrapper: %v", err)
	}
	argsFile := filepath.Join(t.TempDir(), "claude-args.txt")
	runner := &claudeRunner{
		cmdName: wrapper,
		env:     []string{"GO_WANT_HELPER_PROCESS=1"},
	}
	if err := runner.SendUser("args_file=" + argsFile); err != nil {
		t.Fatalf("SendUser: %v", err)
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("ReadFile args: %v", err)
	}
	if !strings.Contains(string(data), "args_file=") {
		t.Fatalf("expected user text arg to be passed through, got %q", string(data))
	}
}

func TestClaudeRunnerRunMissingCommand(t *testing.T) {
	runner := &claudeRunner{cmdName: "definitely-missing-claude-command-12345"}
	if err := runner.run("hello", nil); err == nil {
		t.Fatalf("expected command start error")
	}
}

func TestClaudeRunnerRunIncludesResumeFlag(t *testing.T) {
	testBin := os.Args[0]
	wrapper := filepath.Join(t.TempDir(), "claude-wrapper.sh")
	wrapperScript := "#!/bin/sh\nexec \"" + testBin + "\" -test.run=TestHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(wrapperScript), 0o755); err != nil {
		t.Fatalf("WriteFile wrapper: %v", err)
	}
	argsFile := filepath.Join(t.TempDir(), "claude-args.txt")
	runner := &claudeRunner{
		cmdName:   wrapper,
		env:       []string{"GO_WANT_HELPER_PROCESS=1"},
		sessionID: "session-abc",
	}
	if err := runner.run("args_file="+argsFile, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("ReadFile args: %v", err)
	}
	args := string(data)
	if !strings.Contains(args, "--resume") || !strings.Contains(args, "session-abc") {
		t.Fatalf("expected resume arguments, got %q", args)
	}
}

func TestClaudeRunnerRunIncludesAdditionalDirectoryArgs(t *testing.T) {
	testBin := os.Args[0]
	wrapper := filepath.Join(t.TempDir(), "claude-wrapper.sh")
	wrapperScript := "#!/bin/sh\nexec \"" + testBin + "\" -test.run=TestHelperProcess -- \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(wrapperScript), 0o755); err != nil {
		t.Fatalf("WriteFile wrapper: %v", err)
	}
	argsFile := filepath.Join(t.TempDir(), "claude-args.txt")
	runner := &claudeRunner{
		cmdName: wrapper,
		env:     []string{"GO_WANT_HELPER_PROCESS=1"},
		dirs:    []string{"/tmp/backend", "/tmp/shared"},
	}
	if err := runner.run("args_file="+argsFile, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("ReadFile args: %v", err)
	}
	args := string(data)
	if !strings.Contains(args, "--add-dir") {
		t.Fatalf("expected --add-dir arguments, got %q", args)
	}
	if !strings.Contains(args, "/tmp/backend") || !strings.Contains(args, "/tmp/shared") {
		t.Fatalf("expected additional directory args, got %q", args)
	}
}
