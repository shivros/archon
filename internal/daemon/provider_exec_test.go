package daemon

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testProviderLogSink struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func (s *testProviderLogSink) StdoutWriter() io.Writer {
	return &s.stdout
}

func (s *testProviderLogSink) StderrWriter() io.Writer {
	return &s.stderr
}

func (s *testProviderLogSink) Write(stream string, data []byte) {
	switch stream {
	case "stdout":
		_, _ = s.stdout.Write(data)
	case "stderr":
		_, _ = s.stderr.Write(data)
	}
}

func TestNewExecProviderValidation(t *testing.T) {
	if _, err := newExecProvider("custom", "", nil); err == nil {
		t.Fatalf("expected empty command error")
	}
	if _, err := newExecProvider("custom", "cmd", errString("boom")); err == nil {
		t.Fatalf("expected pass-through error")
	}
}

func TestLookupCommandMissing(t *testing.T) {
	if _, err := lookupCommand("definitely-missing-binary-123456"); err == nil {
		t.Fatalf("expected missing command error")
	}
}

func TestExecProviderStartRunsProcessAndStreamsOutput(t *testing.T) {
	provider, err := newExecProvider("custom", os.Args[0], nil)
	if err != nil {
		t.Fatalf("newExecProvider: %v", err)
	}
	sink := &testProviderLogSink{}
	proc, err := provider.Start(StartSessionConfig{
		Args: helperArgs("stdout=hello", "stderr=oops", "exit=0"),
		Env:  []string{"GO_WANT_HELPER_PROCESS=1"},
	}, sink, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if proc == nil || proc.Process == nil || proc.Wait == nil {
		t.Fatalf("expected provider process to be initialized")
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !strings.Contains(sink.stdout.String(), "hello") {
		t.Fatalf("expected stdout output, got %q", sink.stdout.String())
	}
	if !strings.Contains(sink.stderr.String(), "oops") {
		t.Fatalf("expected stderr output, got %q", sink.stderr.String())
	}
}

func TestExecProviderStartGeminiIncludesDirectoryArgs(t *testing.T) {
	wrapper := filepath.Join(t.TempDir(), "gemini-wrapper.sh")
	script := `#!/bin/sh
if [ -n "$ARCHON_EXEC_ARGS_FILE" ]; then
  printf '%s\n' "$@" > "$ARCHON_EXEC_ARGS_FILE"
fi
echo hello
echo oops >&2
`
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	argsFile := filepath.Join(t.TempDir(), "gemini-args.txt")
	backendDir := t.TempDir()
	sharedDir := t.TempDir()
	provider, err := newExecProvider("gemini", wrapper, nil)
	if err != nil {
		t.Fatalf("newExecProvider: %v", err)
	}
	sink := &testProviderLogSink{}
	proc, err := provider.Start(StartSessionConfig{
		Args:                  []string{"run", "hello"},
		AdditionalDirectories: []string{backendDir, sharedDir},
		Env:                   []string{"ARCHON_EXEC_ARGS_FILE=" + argsFile},
	}, sink, nil)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if proc == nil || proc.Wait == nil {
		t.Fatalf("expected provider process to be initialized")
	}
	if err := proc.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	args := string(got)
	if !strings.Contains(args, "--include-directories") {
		t.Fatalf("expected include directories args, got %q", args)
	}
	if !strings.Contains(args, backendDir) || !strings.Contains(args, sharedDir) {
		t.Fatalf("expected additional directory paths in args, got %q", args)
	}
}
