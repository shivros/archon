package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestResolveRootCommandName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "-h", want: "help"},
		{input: "--help", want: "help"},
		{input: "-v", want: "version"},
		{input: "--version", want: "version"},
		{input: "ps", want: "ps"},
	}
	for _, tc := range cases {
		if got := resolveRootCommandName(tc.input); got != tc.want {
			t.Fatalf("resolveRootCommandName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRunCLIWithCommandsHelpAliasShowsUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := runCLIWithCommands([]string{"--help"}, commandWiring{stderr: stderr}, map[string]commandRunner{})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if got := stderr.String(); got == "" {
		t.Fatalf("expected usage output")
	}
}

func TestRunCLIWithCommandsVersionAliasDispatchesRunner(t *testing.T) {
	stderr := &bytes.Buffer{}
	runner := &stubCommandRunner{}
	code := runCLIWithCommands(
		[]string{"--version"},
		commandWiring{stderr: stderr},
		map[string]commandRunner{"version": runner},
	)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if runner.calls != 1 {
		t.Fatalf("expected runner to be called once, got %d", runner.calls)
	}
	if len(runner.lastArgs) != 0 {
		t.Fatalf("expected no forwarded args, got %#v", runner.lastArgs)
	}
}

func TestRunCLIWithCommandsUnknownCommandReturnsTwo(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := runCLIWithCommands([]string{"nope"}, commandWiring{stderr: stderr}, map[string]commandRunner{})
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d", code)
	}
	out := stderr.String()
	if out == "" || !bytes.Contains([]byte(out), []byte("unknown command: nope")) {
		t.Fatalf("expected unknown command output, got %q", out)
	}
}

func TestRunCLIWithCommandsCommandErrorReturnsOne(t *testing.T) {
	stderr := &bytes.Buffer{}
	runner := &stubCommandRunner{err: errors.New("boom")}
	code := runCLIWithCommands(
		[]string{"ps"},
		commandWiring{stderr: stderr},
		map[string]commandRunner{"ps": runner},
	)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("ps error: boom")) {
		t.Fatalf("expected command error output, got %q", stderr.String())
	}
}

func TestRunCLIShowsUsageWhenNoArgs(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := runCLI([]string{}, commandWiring{stderr: stderr})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if stderr.Len() == 0 {
		t.Fatalf("expected usage output")
	}
}

type stubCommandRunner struct {
	calls    int
	lastArgs []string
	err      error
}

func (r *stubCommandRunner) Run(args []string) error {
	r.calls++
	r.lastArgs = append([]string(nil), args...)
	return r.err
}
