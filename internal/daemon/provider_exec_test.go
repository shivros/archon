package daemon

import (
	"bytes"
	"io"
	"os"
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
