package app

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultFileLinkResolverResolve(t *testing.T) {
	resolver := defaultFileLinkResolver{}
	tests := []struct {
		name    string
		target  string
		path    string
		line    int
		column  int
		wantErr bool
	}{
		{name: "absolute path", target: "/tmp/main.go", path: "/tmp/main.go"},
		{name: "path with line and col", target: "/tmp/main.go:12:3", path: "/tmp/main.go", line: 12, column: 3},
		{name: "file url with fragment", target: "file:///tmp/main.go#L9C2", path: "/tmp/main.go", line: 9, column: 2},
		{name: "file url with query and fragment", target: "file:///tmp/main.go?x=1#L5", path: "/tmp/main.go", line: 5, column: 0},
		{name: "path query removed", target: "/tmp/main.go?foo=bar", path: "/tmp/main.go"},
		{name: "path fragment removed", target: "/tmp/main.go#L10", path: "/tmp/main.go", line: 10, column: 0},
		{name: "zero fragment ignored", target: "file:///tmp/main.go#L0", path: "/tmp/main.go", line: 0, column: 0},
		{name: "relative path rejected", target: "main.go", wantErr: true},
		{name: "http rejected", target: "https://example.com", wantErr: true},
		{name: "malformed url rejected", target: "file://%", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolver.Resolve(tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected resolve error: %v", err)
			}
			if resolved.Path != tt.path || resolved.Line != tt.line || resolved.Column != tt.column {
				t.Fatalf("unexpected resolved target: %#v", resolved)
			}
		})
	}
}

func TestDefaultFileLinkOpenerUsesResolvedCommand(t *testing.T) {
	policy := stubFileLinkOpenPolicy{command: FileLinkOpenCommand{Name: "opener", Args: []string{"/tmp/main.go"}}}
	runner := &observedFileLinkCommandRunner{}
	opener := defaultFileLinkOpener{policy: policy, runner: runner}

	err := opener.Open(context.Background(), ResolvedFileLink{Path: "/tmp/main.go"})
	if err != nil {
		t.Fatalf("unexpected open error: %v", err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("expected one runner call, got %#v", runner.commands)
	}
	if runner.commands[0].Name != "opener" || len(runner.commands[0].Args) != 1 || runner.commands[0].Args[0] != "/tmp/main.go" {
		t.Fatalf("unexpected runner command: %#v", runner.commands[0])
	}
}

func TestDefaultFileLinkOpenerEmptyPathRejected(t *testing.T) {
	opener := defaultFileLinkOpener{}
	err := opener.Open(context.Background(), ResolvedFileLink{Path: " "})
	if err == nil || !strings.Contains(err.Error(), "link target is empty") {
		t.Fatalf("expected empty target error, got %v", err)
	}
}

func TestDefaultFileLinkOpenerPropagatesRunnerError(t *testing.T) {
	opener := defaultFileLinkOpener{
		policy: stubFileLinkOpenPolicy{command: FileLinkOpenCommand{Name: "opener", Args: []string{"/tmp/main.go"}}},
		runner: &observedFileLinkCommandRunner{err: errors.New("boom")},
	}

	err := opener.Open(context.Background(), ResolvedFileLink{Path: "/tmp/main.go"})
	if err == nil {
		t.Fatalf("expected open error")
	}
}

func TestDefaultFileLinkOpenerPropagatesPolicyError(t *testing.T) {
	opener := defaultFileLinkOpener{
		policy: stubFileLinkOpenPolicy{err: errors.New("policy boom")},
		runner: &observedFileLinkCommandRunner{},
	}
	err := opener.Open(context.Background(), ResolvedFileLink{Path: "/tmp/main.go"})
	if err == nil || !strings.Contains(err.Error(), "policy boom") {
		t.Fatalf("expected policy error, got %v", err)
	}
}

func TestDefaultFileLinkOpenerFallsBackWhenPolicyOrRunnerNil(t *testing.T) {
	withNilPolicy := defaultFileLinkOpener{
		policy: nil,
		runner: &observedFileLinkCommandRunner{},
	}
	if err := withNilPolicy.Open(context.Background(), ResolvedFileLink{Path: "/tmp/main.go"}); err != nil {
		t.Fatalf("expected nil policy fallback, got %v", err)
	}

	withNilRunner := defaultFileLinkOpener{
		policy: stubFileLinkOpenPolicy{command: FileLinkOpenCommand{Name: "definitely-not-a-real-opener", Args: []string{"/tmp/main.go"}}},
		runner: nil,
	}
	err := withNilRunner.Open(context.Background(), ResolvedFileLink{Path: "/tmp/main.go"})
	if err == nil {
		t.Fatalf("expected nil runner fallback to attempt execution and fail")
	}
}

func TestDefaultFileLinkOpenPolicyBuildCommand(t *testing.T) {
	policy := newDefaultFileLinkOpenPolicy()
	command, err := policy.BuildCommand(ResolvedFileLink{Path: "/tmp/main.go"})
	if err != nil {
		t.Fatalf("unexpected policy error: %v", err)
	}
	switch runtime.GOOS {
	case "linux":
		if command.Name != "xdg-open" {
			t.Fatalf("unexpected linux command: %#v", command)
		}
	case "darwin":
		if command.Name != "open" {
			t.Fatalf("unexpected darwin command: %#v", command)
		}
	case "windows":
		if command.Name != "cmd" {
			t.Fatalf("unexpected windows command: %#v", command)
		}
	default:
		t.Fatalf("expected known runtime in tests, got %q", runtime.GOOS)
	}
	if len(command.Args) == 0 {
		t.Fatalf("expected command args, got %#v", command)
	}
}

func TestDefaultFileLinkOpenPolicyRejectsEmptyPath(t *testing.T) {
	policy := newDefaultFileLinkOpenPolicy()
	_, err := policy.BuildCommand(ResolvedFileLink{Path: " "})
	if err == nil || !strings.Contains(err.Error(), "link target is empty") {
		t.Fatalf("expected empty path error, got %v", err)
	}
}

func TestDefaultFileLinkCommandRunnerRejectsEmptyCommand(t *testing.T) {
	runner := defaultFileLinkCommandRunner{}
	err := runner.Run(context.Background(), FileLinkOpenCommand{Name: " "})
	if err == nil || !strings.Contains(err.Error(), "open command is empty") {
		t.Fatalf("expected empty command error, got %v", err)
	}
}

func TestDefaultFileLinkCommandRunnerPropagatesExecutionError(t *testing.T) {
	runner := defaultFileLinkCommandRunner{}
	err := runner.Run(context.Background(), FileLinkOpenCommand{Name: "definitely-not-a-command", Args: []string{"x"}})
	if err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("expected command failure error, got %v", err)
	}
}

type stubFileLinkOpenPolicy struct {
	command FileLinkOpenCommand
	err     error
}

func (s stubFileLinkOpenPolicy) BuildCommand(target ResolvedFileLink) (FileLinkOpenCommand, error) {
	if s.err != nil {
		return FileLinkOpenCommand{}, s.err
	}
	return s.command, nil
}

type observedFileLinkCommandRunner struct {
	commands []FileLinkOpenCommand
	err      error
}

func (o *observedFileLinkCommandRunner) Run(_ context.Context, command FileLinkOpenCommand) error {
	o.commands = append(o.commands, command)
	return o.err
}
