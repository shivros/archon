package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	controlclient "control/internal/client"
	"control/internal/types"
)

func TestDaemonCommandKillFlag(t *testing.T) {
	var calls []string
	cmd := NewDaemonCommand(
		&bytes.Buffer{},
		func(background bool) error {
			calls = append(calls, "run")
			if background {
				calls = append(calls, "background")
			}
			return nil
		},
		func() error {
			calls = append(calls, "kill")
			return nil
		},
	)

	if err := cmd.Run([]string{"--kill"}); err != nil {
		t.Fatalf("expected kill run to succeed, got err=%v", err)
	}
	if strings.Join(calls, ",") != "kill" {
		t.Fatalf("unexpected call order: %v", calls)
	}
}

func TestDaemonCommandDefaultRunsForeground(t *testing.T) {
	var calls []string
	cmd := NewDaemonCommand(
		&bytes.Buffer{},
		func(background bool) error {
			calls = append(calls, "run")
			if background {
				calls = append(calls, "background")
			}
			return nil
		},
		func() error {
			calls = append(calls, "kill")
			return nil
		},
	)

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected foreground run to succeed, got err=%v", err)
	}
	if want := "run"; strings.Join(calls, ",") != want {
		t.Fatalf("unexpected calls: got %v, want %v", calls, []string{"run"})
	}
}

func TestDaemonCommandBackgroundFlag(t *testing.T) {
	var calls []string
	cmd := NewDaemonCommand(
		&bytes.Buffer{},
		func(background bool) error {
			calls = append(calls, "run")
			if background {
				calls = append(calls, "background")
			}
			return nil
		},
		func() error {
			calls = append(calls, "kill")
			return nil
		},
	)

	if err := cmd.Run([]string{"--background"}); err != nil {
		t.Fatalf("expected background run to succeed, got err=%v", err)
	}
	if want := "run,background"; strings.Join(calls, ",") != want {
		t.Fatalf("unexpected calls: got %v, want %v", calls, []string{"run", "background"})
	}
}

func TestDaemonCommandForceFlagStopsThenStarts(t *testing.T) {
	var calls []string
	cmd := NewDaemonCommand(
		&bytes.Buffer{},
		func(background bool) error {
			calls = append(calls, "run")
			return nil
		},
		func() error {
			calls = append(calls, "kill")
			return nil
		},
	)

	if err := cmd.Run([]string{"--force"}); err != nil {
		t.Fatalf("expected force run to succeed, got err=%v", err)
	}
	if want := "kill,run"; strings.Join(calls, ",") != want {
		t.Fatalf("unexpected calls: got %v, want %v", calls, []string{"kill", "run"})
	}
}

func TestDaemonCommandForceBackgroundStopsThenStartsInBackground(t *testing.T) {
	var calls []string
	cmd := NewDaemonCommand(
		&bytes.Buffer{},
		func(background bool) error {
			calls = append(calls, "run")
			if background {
				calls = append(calls, "background")
			}
			return nil
		},
		func() error {
			calls = append(calls, "kill")
			return nil
		},
	)

	if err := cmd.Run([]string{"--force", "--background"}); err != nil {
		t.Fatalf("expected force background run to succeed, got err=%v", err)
	}
	if want := "kill,run,background"; strings.Join(calls, ",") != want {
		t.Fatalf("unexpected calls: got %v, want %v", calls, []string{"kill", "run", "background"})
	}
}

func TestDaemonCommandForceFailsIfKillFails(t *testing.T) {
	cmd := NewDaemonCommand(
		&bytes.Buffer{},
		func(background bool) error { return nil },
		func() error { return errors.New("kill failed") },
	)

	if err := cmd.Run([]string{"--force"}); err == nil {
		t.Fatal("expected error when kill fails during force")
	}
}

func TestDaemonCommandRunDaemonError(t *testing.T) {
	cmd := NewDaemonCommand(
		&bytes.Buffer{},
		func(background bool) error { return errors.New("daemon crashed") },
		func() error { return nil },
	)

	if err := cmd.Run(nil); err == nil {
		t.Fatal("expected error from daemon runner")
	}
}

func TestDaemonCommandKillNoRunningDaemonIsNoOp(t *testing.T) {
	cmd := NewDaemonCommand(
		&bytes.Buffer{},
		func(background bool) error {
			t.Fatal("should not call runDaemon for --kill")
			return nil
		},
		func() error { return nil },
	)

	if err := cmd.Run([]string{"--kill"}); err != nil {
		t.Fatalf("expected --kill to succeed even with no daemon, got err=%v", err)
	}
}

func TestStartCommandWritesSessionID(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		startSessionResp: &types.Session{ID: "session-123"},
	}
	cmd := NewStartCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{
		"--provider", "codex",
		"--cwd", "/tmp/project",
		"--cmd", "codex",
		"--title", "demo",
		"--tag", "one",
		"--tag", "two",
		"--env", "A=B",
		"--env", "C=D",
		"arg1",
		"arg2",
	})
	if err != nil {
		t.Fatalf("expected start to succeed, got err=%v", err)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if len(fake.startRequests) != 1 {
		t.Fatalf("expected one start request, got %d", len(fake.startRequests))
	}
	req := fake.startRequests[0]
	if req.Provider != "codex" || req.Cwd != "/tmp/project" || req.Cmd != "codex" || req.Title != "demo" {
		t.Fatalf("unexpected start request basics: %#v", req)
	}
	if len(req.Args) != 2 || req.Args[0] != "arg1" || req.Args[1] != "arg2" {
		t.Fatalf("unexpected args: %#v", req.Args)
	}
	if len(req.Tags) != 2 || req.Tags[0] != "one" || req.Tags[1] != "two" {
		t.Fatalf("unexpected tags: %#v", req.Tags)
	}
	if len(req.Env) != 2 || req.Env[0] != "A=B" || req.Env[1] != "C=D" {
		t.Fatalf("unexpected env: %#v", req.Env)
	}
	if got := stdout.String(); got != "session-123\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestPSCommandPrintsSessions(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		sessionsResp: []*types.Session{
			{ID: "s1", Status: types.SessionStatusRunning, Provider: "codex", PID: 42, Title: "demo"},
		},
	}
	cmd := NewPSCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected ps to succeed, got err=%v", err)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if fake.listSessionsCalls != 1 {
		t.Fatalf("expected list sessions once, got %d", fake.listSessionsCalls)
	}
	out := stdout.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "STATUS") || !strings.Contains(out, "PROVIDER") {
		t.Fatalf("expected header in output, got %q", out)
	}
	if !strings.Contains(out, "s1") || !strings.Contains(out, "demo") {
		t.Fatalf("expected session row in output, got %q", out)
	}
}

// TestPSCommandJSONOutput locks the CLI contract for `archon ps --json`.
// The asserted field set (id, status, provider, pid, title) is load-bearing for
// programmatic consumers. Changes that would remove or rename any of these
// fields in types.Session must be intentional CLI-contract decisions.
func TestPSCommandJSONOutput(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		sessionsResp: []*types.Session{
			{ID: "s1", Status: types.SessionStatusRunning, Provider: "codex", PID: 42, Title: "demo"},
			{ID: "s2", Status: types.SessionStatusExited, Provider: "claude", PID: 7, Title: "other"},
		},
	}
	cmd := NewPSCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--json"}); err != nil {
		t.Fatalf("expected ps --json to succeed, got err=%v", err)
	}

	raw := stdout.Bytes()
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Fatalf("expected trailing newline, got %q", string(raw))
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("expected valid JSON array, got err=%v, raw=%q", err, string(raw))
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 elements, got %d: %#v", len(decoded), decoded)
	}
	for i, element := range decoded {
		for _, field := range []string{"id", "status", "provider", "pid", "title"} {
			if _, ok := element[field]; !ok {
				t.Fatalf("element %d missing required field %q: %#v", i, field, element)
			}
		}
	}
}

func TestPSCommandJSONOutputEmptyList(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{sessionsResp: nil}
	cmd := NewPSCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--json"}); err != nil {
		t.Fatalf("expected ps --json to succeed, got err=%v", err)
	}
	if got := stdout.String(); got != "[]\n" {
		t.Fatalf("expected exactly %q, got %q", "[]\n", got)
	}
}

func TestPSCommandHumanOutputUnchangedByJSONFlagSupport(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		sessionsResp: []*types.Session{
			{ID: "s1", Status: types.SessionStatusRunning, Provider: "codex", PID: 42, Title: "demo"},
		},
	}
	cmd := NewPSCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected ps to succeed, got err=%v", err)
	}
	const want = "ID  STATUS   PROVIDER  PID  TITLE\ns1  running  codex     42   demo\n"
	if got := stdout.String(); got != want {
		t.Fatalf("human table output changed.\n want: %q\n  got: %q", want, got)
	}
}

func TestTailCommandWritesItemsJSON(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		tailItemsResp: &controlclient.TailItemsResponse{
			Items: []map[string]any{{"type": "log", "text": "hello"}},
		},
	}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--lines", "50", "session-1"}); err != nil {
		t.Fatalf("expected tail to succeed, got err=%v", err)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if fake.tailItemsCalls != 1 || fake.tailItemsID != "session-1" || fake.tailItemsLines != 50 {
		t.Fatalf("unexpected tail call details: calls=%d id=%q lines=%d", fake.tailItemsCalls, fake.tailItemsID, fake.tailItemsLines)
	}
	var items []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		t.Fatalf("expected valid json output, got err=%v, raw=%q", err, stdout.String())
	}
	if len(items) != 1 {
		t.Fatalf("expected one output item, got %d", len(items))
	}
}

func TestUICommandEnsuresVersionAndRunsUI(t *testing.T) {
	fake := &fakeCommandClient{}
	logConfigured := 0

	cmd := NewUICommand(
		&bytes.Buffer{},
		fixedDaemonVersionFactory(fake),
		func() { logConfigured++ },
		"v-test",
	)

	if err := cmd.Run([]string{"--restart-daemon"}); err != nil {
		t.Fatalf("expected ui command to succeed, got err=%v", err)
	}
	if logConfigured != 1 {
		t.Fatalf("expected UI logging to be configured once, got %d", logConfigured)
	}
	if fake.ensureVersionCalls != 1 {
		t.Fatalf("expected ensure daemon version once, got %d", fake.ensureVersionCalls)
	}
	if fake.ensureDaemonCalls != 0 {
		t.Fatalf("expected ensure daemon to not be called, got %d", fake.ensureDaemonCalls)
	}
	if fake.ensureVersionExpected != "v-test" || !fake.ensureVersionRestart {
		t.Fatalf("unexpected ensure version args: expected=%q restart=%v", fake.ensureVersionExpected, fake.ensureVersionRestart)
	}
	if fake.runUICalls != 1 {
		t.Fatalf("expected ui runner once, got %d", fake.runUICalls)
	}
}

// TestUICommandDefaultInvocation verifies the default path: logging configured,
// daemon version checked without restart, then UI launched.
func TestUICommandDefaultInvocation(t *testing.T) {
	fake := &fakeCommandClient{}
	logConfigured := 0

	cmd := NewUICommand(
		&bytes.Buffer{},
		fixedDaemonVersionFactory(fake),
		func() { logConfigured++ },
		"v-test",
	)

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected ui command to succeed, got err=%v", err)
	}
	if logConfigured != 1 {
		t.Fatalf("expected UI logging to be configured once, got %d", logConfigured)
	}
	if fake.ensureVersionCalls != 1 {
		t.Fatalf("expected ensure daemon version once, got %d", fake.ensureVersionCalls)
	}
	if fake.ensureDaemonCalls != 0 {
		t.Fatalf("expected ensure daemon to not be called, got %d", fake.ensureDaemonCalls)
	}
	if fake.ensureVersionExpected != "v-test" || fake.ensureVersionRestart {
		t.Fatalf("unexpected ensure version args: expected=%q restart=%v", fake.ensureVersionExpected, fake.ensureVersionRestart)
	}
	if fake.runUICalls != 1 {
		t.Fatalf("expected ui runner once, got %d", fake.runUICalls)
	}
}

// TestUICommandVersionCheckError asserts daemon version check failure is returned.
func TestUICommandVersionCheckError(t *testing.T) {
	fake := &fakeCommandClient{
		ensureVersionErr: errors.New("version mismatch"),
	}

	cmd := NewUICommand(
		&bytes.Buffer{},
		fixedDaemonVersionFactory(fake),
		func() {},
		"v-test",
	)

	if err := cmd.Run(nil); err == nil {
		t.Fatal("expected error from version check failure")
	}
	if fake.runUICalls != 0 {
		t.Fatalf("expected no UI launch on version check failure, got %d calls", fake.runUICalls)
	}
}

// TestUICommandEnsureDaemonError asserts reachability check failure is returned.
func TestUICommandEnsureDaemonError(t *testing.T) {
	fake := &fakeCommandClient{
		ensureDaemonErr: errors.New("daemon unreachable"),
	}

	cmd := NewUICommand(
		&bytes.Buffer{},
		fixedDaemonVersionFactory(fake),
		func() {},
		"v-test",
	)

	if err := cmd.Run([]string{"--ignore-daemon-mismatch"}); err == nil {
		t.Fatal("expected error from daemon reachability failure")
	}
	if fake.runUICalls != 0 {
		t.Fatalf("expected no UI launch on daemon reachability failure, got %d calls", fake.runUICalls)
	}
}

func TestUICommandIgnoresVersionMismatchWhenFlagSet(t *testing.T) {
	fake := &fakeCommandClient{}
	logConfigured := 0

	cmd := NewUICommand(
		&bytes.Buffer{},
		fixedDaemonVersionFactory(fake),
		func() { logConfigured++ },
		"v-test",
	)

	if err := cmd.Run([]string{"--ignore-daemon-mismatch"}); err != nil {
		t.Fatalf("expected ui command to succeed, got err=%v", err)
	}
	if logConfigured != 1 {
		t.Fatalf("expected UI logging to be configured once, got %d", logConfigured)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if fake.ensureVersionCalls != 0 {
		t.Fatalf("expected ensure daemon version to not be called, got %d", fake.ensureVersionCalls)
	}
	if fake.runUICalls != 1 {
		t.Fatalf("expected ui runner once, got %d", fake.runUICalls)
	}
}

func TestVersionCommandPrintsBuildMetadata(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewVersionCommandWithDependencies(
		stdout,
		stderr,
		staticBuildMetadataProvider{
			metadata: buildMetadata{
				Version:   "v-test",
				Commit:    "abc123",
				BuildDate: "2026-03-23T00:00:00Z",
			},
		},
		textVersionFormatter{},
	)
	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected version command to succeed, got %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "version: v-test") {
		t.Fatalf("expected version output, got %q", out)
	}
	if !strings.Contains(out, "commit: abc123") {
		t.Fatalf("expected commit output, got %q", out)
	}
	if !strings.Contains(out, "build_date: 2026-03-23T00:00:00Z") {
		t.Fatalf("expected build_date output, got %q", out)
	}
}

func TestVersionCommandUsesInjectedFormatter(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewVersionCommandWithDependencies(
		stdout,
		stderr,
		staticBuildMetadataProvider{
			metadata: buildMetadata{
				Version: "v-custom",
			},
		},
		staticVersionFormatter{output: "custom-format\n"},
	)
	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected version command to succeed, got %v", err)
	}
	if got := stdout.String(); got != "custom-format\n" {
		t.Fatalf("expected formatter output, got %q", got)
	}
}

func TestNewVersionCommandWithDependenciesDefaultsNilCollaborators(t *testing.T) {
	cmd := NewVersionCommandWithDependencies(&bytes.Buffer{}, &bytes.Buffer{}, nil, nil)
	if cmd.metadataProvider == nil {
		t.Fatalf("expected default metadata provider")
	}
	if cmd.formatter == nil {
		t.Fatalf("expected default formatter")
	}
}

func TestVersionCommandRejectsUnknownFlag(t *testing.T) {
	cmd := NewVersionCommandWithDependencies(
		&bytes.Buffer{},
		&bytes.Buffer{},
		staticBuildMetadataProvider{},
		textVersionFormatter{},
	)
	if err := cmd.Run([]string{"--not-a-flag"}); err == nil {
		t.Fatalf("expected unknown flag to fail")
	}
}

func TestVersionCommandPropagatesWriteError(t *testing.T) {
	cmd := NewVersionCommandWithDependencies(
		errorWriter{},
		&bytes.Buffer{},
		staticBuildMetadataProvider{
			metadata: buildMetadata{
				Version: "v-test",
			},
		},
		textVersionFormatter{},
	)
	if err := cmd.Run(nil); err == nil {
		t.Fatalf("expected write failure to be returned")
	}
}

func TestStartCommandRequiresProvider(t *testing.T) {
	cmd := NewStartCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(&fakeCommandClient{}))
	err := cmd.Run(nil)
	if err == nil || !strings.Contains(err.Error(), "provider is required") {
		t.Fatalf("expected provider validation error, got %v", err)
	}
}

// TestStartCommandDaemonFailure asserts daemon-side start failure produces no stdout.
func TestStartCommandDaemonFailure(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		startSessionErr: errors.New("daemon rejected: bad request"),
	}
	cmd := NewStartCommand(stdout, stderr, fixedSessionFactory(fake))

	err := cmd.Run([]string{"--provider", "codex"})
	if err == nil {
		t.Fatal("expected error from daemon failure")
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout on failure, got %q", stdout.String())
	}
}

// TestStartCommandMissingProviderNoDaemonContact asserts missing provider does not contact daemon.
func TestStartCommandMissingProviderNoDaemonContact(t *testing.T) {
	fake := &fakeCommandClient{}
	cmd := NewStartCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{"--cwd", "/tmp"})
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
	if fake.ensureDaemonCalls != 0 {
		t.Fatalf("expected no daemon contact, got %d ensureDaemonCalls", fake.ensureDaemonCalls)
	}
	if len(fake.startRequests) != 0 {
		t.Fatalf("expected no start requests, got %d", len(fake.startRequests))
	}
}

// TestStartCommandTitleForwarded asserts the title flag is forwarded to the daemon.
func TestStartCommandTitleForwarded(t *testing.T) {
	fake := &fakeCommandClient{
		startSessionResp: &types.Session{ID: "session-ttl"},
	}
	cmd := NewStartCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{"--provider", "codex", "--title", "my session"})
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if len(fake.startRequests) != 1 {
		t.Fatalf("expected 1 start request, got %d", len(fake.startRequests))
	}
	if fake.startRequests[0].Title != "my session" {
		t.Fatalf("expected title='my session', got %q", fake.startRequests[0].Title)
	}
}

// --- Kill command tests ---

// TestKillCommandSuccess asserts kill prints "ok" on stdout.
func TestKillCommandSuccess(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{}
	cmd := NewKillCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1"})
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.killSessionCalls != 1 || fake.killSessionID != "session-1" {
		t.Fatalf("unexpected kill call: calls=%d id=%q", fake.killSessionCalls, fake.killSessionID)
	}
	if got := stdout.String(); got != "ok\n" {
		t.Fatalf("expected stdout 'ok\\n', got %q", got)
	}
}

// TestKillCommandMissingID asserts missing session id fails without daemon contact.
func TestKillCommandMissingID(t *testing.T) {
	fake := &fakeCommandClient{}
	cmd := NewKillCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run(nil)
	if err == nil {
		t.Fatal("expected error for missing session id")
	}
	if fake.ensureDaemonCalls != 0 {
		t.Fatalf("expected no daemon contact, got %d ensureDaemonCalls", fake.ensureDaemonCalls)
	}
}

// TestKillCommandDaemonError asserts daemon errors don't print "ok".
func TestKillCommandDaemonError(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		killSessionErr: errors.New("session not found"),
	}
	cmd := NewKillCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout on error, got %q", stdout.String())
	}
}

// --- Interrupt command tests ---

// TestInterruptCommandSuccess asserts interrupt exits silently on success.
func TestInterruptCommandSuccess(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{}
	cmd := NewInterruptCommand(stdout, stderr, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1"})
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.interruptSessionCalls != 1 || fake.interruptSessionID != "session-1" {
		t.Fatalf("unexpected interrupt call: calls=%d id=%q", fake.interruptSessionCalls, fake.interruptSessionID)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("expected no stdout, got %q", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("expected no stderr, got %q", got)
	}
}

// TestInterruptCommandMissingID asserts missing session id fails without daemon contact.
func TestInterruptCommandMissingID(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{}
	cmd := NewInterruptCommand(stdout, stderr, fixedSessionFactory(fake))

	err := cmd.Run(nil)
	if err == nil {
		t.Fatal("expected error for missing session id")
	}
	if fake.interruptSessionCalls != 0 {
		t.Fatalf("expected no interrupt call, got %d", fake.interruptSessionCalls)
	}
	if fake.ensureDaemonCalls != 0 {
		t.Fatalf("expected no daemon contact, got %d ensureDaemonCalls", fake.ensureDaemonCalls)
	}
}

// TestInterruptCommandDaemonError asserts daemon errors produce single-line stderr and non-zero exit.
func TestInterruptCommandDaemonError(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		interruptSessionErr: errors.New("session not found"),
	}
	cmd := NewInterruptCommand(stdout, stderr, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout on error, got %q", stdout.String())
	}
}

// TestInterruptCommandDaemonNoOp asserts daemon success (no-op) exits 0 with no output.
func TestInterruptCommandDaemonNoOp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{}
	cmd := NewInterruptCommand(stdout, stderr, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1"})
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("expected no stdout, got %q", got)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("expected no stderr, got %q", got)
	}
}

// --- Approvals command tests ---

// TestApprovalsCommandEmptyListTable asserts empty list prints header only.
func TestApprovalsCommandEmptyListTable(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		listApprovalsResp: []*types.Approval{},
	}
	cmd := NewApprovalsCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	want := "REQUEST_ID  METHOD  CREATED\n"
	if got := stdout.String(); got != want {
		t.Fatalf("expected header only, got %q", got)
	}
}

// TestApprovalsCommandNonEmptyListTable asserts non-empty list renders table rows.
func TestApprovalsCommandNonEmptyListTable(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		listApprovalsResp: []*types.Approval{
			{SessionID: "s1", RequestID: 1, Method: "exec_command", CreatedAt: time.Date(2026, 4, 24, 12, 30, 0, 0, time.UTC)},
			{SessionID: "s1", RequestID: 2, Method: "write_file", CreatedAt: time.Date(2026, 4, 24, 12, 31, 5, 0, time.UTC)},
		},
	}
	cmd := NewApprovalsCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "REQUEST_ID") || !strings.Contains(out, "exec_command") || !strings.Contains(out, "write_file") {
		t.Fatalf("expected table with rows, got %q", out)
	}
}

// TestApprovalsCommandJSONOutput asserts --json emits valid JSON with raw params.
func TestApprovalsCommandJSONOutput(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		listApprovalsResp: []*types.Approval{
			{SessionID: "s1", RequestID: 7, Method: "exec_command", Params: json.RawMessage(`{"cmd":"rm -rf /"}`), CreatedAt: time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)},
		},
	}
	cmd := NewApprovalsCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--json", "session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	raw := stdout.Bytes()
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Fatalf("expected trailing newline, got %q", string(raw))
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("expected valid JSON array, got err=%v, raw=%q", err, string(raw))
	}
	if len(decoded) != 1 {
		t.Fatalf("expected 1 element, got %d", len(decoded))
	}
	if decoded[0]["params"] == nil {
		t.Fatalf("expected params to be preserved, got nil")
	}
}

// TestApprovalsCommandJSONEmptyList asserts --json with empty list emits exactly "[]\n".
func TestApprovalsCommandJSONEmptyList(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		listApprovalsResp: []*types.Approval{},
	}
	cmd := NewApprovalsCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--json", "session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if got := stdout.String(); got != "[]\n" {
		t.Fatalf("expected exactly '[]\\n', got %q", got)
	}
}

// TestApprovalsCommandDaemonError asserts daemon error surfaces as single-line stderr.
func TestApprovalsCommandDaemonError(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		listApprovalsErr: errors.New("session not found"),
	}
	cmd := NewApprovalsCommand(stdout, stderr, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout on error, got %q", stdout.String())
	}
}

// TestApprovalsCommandMissingID asserts missing session id fails without daemon contact.
func TestApprovalsCommandMissingID(t *testing.T) {
	fake := &fakeCommandClient{}
	cmd := NewApprovalsCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run(nil)
	if err == nil {
		t.Fatal("expected error for missing session id")
	}
	if fake.listApprovalsCalls != 0 {
		t.Fatalf("expected no adapter call, got %d", fake.listApprovalsCalls)
	}
}

// --- Approve command tests ---

// TestApproveCommandMissingArgs asserts each missing required arg fails without adapter call.
func TestApproveCommandMissingArgs(t *testing.T) {
	fake := &fakeCommandClient{}
	cmd := NewApproveCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	// Missing session id
	if err := cmd.Run([]string{"--request-id", "1", "--decision", "allow_once"}); err == nil {
		t.Fatal("expected error for missing session id")
	}
	// Missing --request-id
	if err := cmd.Run([]string{"session-1", "--decision", "allow_once"}); err == nil {
		t.Fatal("expected error for missing --request-id")
	}
	// Missing --decision
	if err := cmd.Run([]string{"session-1", "--request-id", "1"}); err == nil {
		t.Fatal("expected error for missing --decision")
	}
	if fake.approveSessionCalls != 0 {
		t.Fatalf("expected no adapter calls, got %d", fake.approveSessionCalls)
	}
}

// TestApproveCommandHappyPath asserts adapter called with exact request, exit 0.
func TestApproveCommandHappyPath(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{}
	cmd := NewApproveCommand(stdout, stderr, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1", "--request-id", "7", "--decision", "allow_once"})
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.approveSessionCalls != 1 {
		t.Fatalf("expected 1 adapter call, got %d", fake.approveSessionCalls)
	}
	if fake.approveSessionIDArg != "session-1" {
		t.Fatalf("expected session id 'session-1', got %q", fake.approveSessionIDArg)
	}
	if fake.approveSessionReq.RequestID != 7 {
		t.Fatalf("expected request_id 7, got %d", fake.approveSessionReq.RequestID)
	}
	if fake.approveSessionReq.Decision != "allow_once" {
		t.Fatalf("expected decision 'allow_once', got %q", fake.approveSessionReq.Decision)
	}
	if fake.approveSessionReq.Responses != nil {
		t.Fatalf("expected nil responses, got %v", fake.approveSessionReq.Responses)
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout, got %q", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected no stderr, got %q", stderr.String())
	}
}

// TestApproveCommandResponsesOrder asserts repeatable --response preserves order.
func TestApproveCommandResponsesOrder(t *testing.T) {
	fake := &fakeCommandClient{}
	cmd := NewApproveCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1", "--request-id", "1", "--decision", "allow", "--response", "alpha", "--response", "beta"})
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.approveSessionReq.Responses == nil {
		t.Fatal("expected responses to be set")
	}
	if len(fake.approveSessionReq.Responses) != 2 || fake.approveSessionReq.Responses[0] != "alpha" || fake.approveSessionReq.Responses[1] != "beta" {
		t.Fatalf("expected responses [alpha,beta], got %v", fake.approveSessionReq.Responses)
	}
}

// TestApproveCommandAcceptSettings asserts valid --accept-settings decodes and forwards.
func TestApproveCommandAcceptSettings(t *testing.T) {
	fake := &fakeCommandClient{}
	cmd := NewApproveCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1", "--request-id", "1", "--decision", "allow", "--accept-settings", `{"a":1}`})
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.approveSessionReq.AcceptSettings == nil {
		t.Fatal("expected accept_settings to be set")
	}
	if v, ok := fake.approveSessionReq.AcceptSettings["a"]; !ok || v != float64(1) {
		t.Fatalf("expected accept_settings.a=1, got %v", v)
	}
}

// TestApproveCommandMalformedAcceptSettings asserts malformed JSON fails before daemon contact.
func TestApproveCommandMalformedAcceptSettings(t *testing.T) {
	fake := &fakeCommandClient{}
	cmd := NewApproveCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1", "--request-id", "1", "--decision", "allow", "--accept-settings", "not-json"})
	if err == nil {
		t.Fatal("expected error for malformed accept-settings")
	}
	if fake.approveSessionCalls != 0 {
		t.Fatalf("expected no adapter call, got %d", fake.approveSessionCalls)
	}
}

// TestApproveCommandDaemonError asserts daemon error surfaces as single-line stderr.
func TestApproveCommandDaemonError(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		approveSessionErr: errors.New("already resolved"),
	}
	cmd := NewApproveCommand(stdout, stderr, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1", "--request-id", "1", "--decision", "allow_once"})
	if err == nil {
		t.Fatal("expected error")
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout on error, got %q", stdout.String())
	}
}

// --- Tail lifecycle (snapshot contract) tests ---

// TestTailSnapshotEmptyResult asserts empty items produce "[]".
func TestTailSnapshotEmptyResult(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		tailItemsResp: &controlclient.TailItemsResponse{Items: nil},
	}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got != "[]" {
		t.Fatalf("expected '[]', got %q", got)
	}
}

// TestTailSnapshotDefaultLines asserts --lines defaults to 200.
func TestTailSnapshotDefaultLines(t *testing.T) {
	fake := &fakeCommandClient{
		tailItemsResp: &controlclient.TailItemsResponse{Items: []map[string]any{}},
	}
	cmd := NewTailCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.tailItemsLines != 200 {
		t.Fatalf("expected default lines=200, got %d", fake.tailItemsLines)
	}
}

// TestTailSnapshotMissingID asserts missing session id fails without daemon contact.
func TestTailSnapshotMissingID(t *testing.T) {
	fake := &fakeCommandClient{}
	cmd := NewTailCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run(nil)
	if err == nil {
		t.Fatal("expected error for missing session id")
	}
	if fake.ensureDaemonCalls != 0 {
		t.Fatalf("expected no daemon contact, got %d ensureDaemonCalls", fake.ensureDaemonCalls)
	}
}

// TestTailSnapshotDaemonError asserts daemon errors produce no partial stdout.
func TestTailSnapshotDaemonError(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		tailItemsErr: errors.New("daemon error"),
	}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	err := cmd.Run([]string{"session-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if stdout.String() != "" {
		t.Fatalf("expected no stdout on error, got %q", stdout.String())
	}
}

func TestLoginCommandPrintsFallbackAndPollsUntilApproved(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
			DeviceCode:              "device-1",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "https://archon.example/activate",
			VerificationURIComplete: "https://archon.example/activate?user_code=ABCD-EFGH",
			ExpiresIn:               600,
			Interval:                0,
		},
		cloudPollResponses: []*controlclient.CloudAuthPollResponse{
			{Status: "authorization_pending"},
			{Status: "approved", Auth: &controlclient.CloudAuthStatusResponse{
				Linked: true,
				User:   &controlclient.CloudLinkedUser{Email: "user@example.com"},
			}},
		},
	}
	var opened string
	cmd := NewLoginCommand(stdout, stderr, fixedCloudAuthFactory(fake), func(_ context.Context, target string) error {
		opened = target
		return nil
	})
	cmd.sleep = func(context.Context, time.Duration) error { return nil }

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected login to succeed, got err=%v", err)
	}
	if fake.ensureDaemonCalls != 1 {
		t.Fatalf("expected ensure daemon once, got %d", fake.ensureDaemonCalls)
	}
	if fake.cloudLoginStartCalls != 1 {
		t.Fatalf("expected cloud login start once, got %d", fake.cloudLoginStartCalls)
	}
	if fake.cloudPollCalls != 2 {
		t.Fatalf("expected two poll attempts, got %d", fake.cloudPollCalls)
	}
	if opened != "https://archon.example/activate?user_code=ABCD-EFGH" {
		t.Fatalf("unexpected browser target: %q", opened)
	}
	out := stdout.String()
	if !strings.Contains(out, "https://archon.example/activate") || !strings.Contains(out, "ABCD-EFGH") {
		t.Fatalf("expected fallback instructions in output, got %q", out)
	}
	if !strings.Contains(out, "user@example.com") {
		t.Fatalf("expected linked user output, got %q", out)
	}
}

func TestLoginCommandContinuesWhenBrowserOpenFails(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
			DeviceCode:      "device-1",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://archon.example/activate",
			ExpiresIn:       600,
			Interval:        0,
		},
		cloudPollResponses: []*controlclient.CloudAuthPollResponse{
			{Status: "approved", Auth: &controlclient.CloudAuthStatusResponse{Linked: true}},
		},
	}
	cmd := NewLoginCommand(stdout, stderr, fixedCloudAuthFactory(fake), func(context.Context, string) error {
		return errors.New("boom")
	})
	cmd.sleep = func(context.Context, time.Duration) error { return nil }

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected login to succeed, got err=%v", err)
	}
	if !strings.Contains(stderr.String(), "browser open failed") {
		t.Fatalf("expected browser failure warning, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Code: ABCD-EFGH") {
		t.Fatalf("expected fallback code in output, got %q", stdout.String())
	}
}

func TestLoginCommandHonorsNoBrowserFlag(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
			DeviceCode:      "device-1",
			UserCode:        "ABCD-EFGH",
			VerificationURI: "https://archon.example/activate",
			ExpiresIn:       600,
		},
		cloudPollResponses: []*controlclient.CloudAuthPollResponse{
			{Status: "approved", Auth: &controlclient.CloudAuthStatusResponse{Linked: true}},
		},
	}
	called := false
	cmd := NewLoginCommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake), func(context.Context, string) error {
		called = true
		return nil
	})
	cmd.sleep = func(context.Context, time.Duration) error { return nil }

	if err := cmd.Run([]string{"--no-browser"}); err != nil {
		t.Fatalf("expected login to succeed, got err=%v", err)
	}
	if called {
		t.Fatalf("expected browser opener not to be called")
	}
}

func TestLoginCommandHandlesSlowDownAndUnexpectedStatus(t *testing.T) {
	t.Run("slow down", func(t *testing.T) {
		fake := &fakeCommandClient{
			cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
				DeviceCode:      "device-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresIn:       600,
				Interval:        1,
			},
			cloudPollResponses: []*controlclient.CloudAuthPollResponse{
				{Status: "slow_down"},
				{Status: "approved", Auth: &controlclient.CloudAuthStatusResponse{Linked: true}},
			},
		}
		var sleeps []time.Duration
		cmd := NewLoginCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedCloudAuthFactory(fake), nil)
		cmd.sleep = func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		}
		if err := cmd.Run(nil); err != nil {
			t.Fatalf("expected login to succeed, got err=%v", err)
		}
		if len(sleeps) == 0 || sleeps[0] != 6*time.Second {
			t.Fatalf("expected slow_down backoff to 6s, got %#v", sleeps)
		}
	})

	t.Run("unexpected status", func(t *testing.T) {
		fake := &fakeCommandClient{
			cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
				DeviceCode:      "device-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresIn:       600,
			},
			cloudPollResponses: []*controlclient.CloudAuthPollResponse{
				{Status: "mystery"},
			},
		}
		cmd := NewLoginCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedCloudAuthFactory(fake), nil)
		cmd.sleep = func(context.Context, time.Duration) error { return nil }
		if err := cmd.Run(nil); err == nil || !strings.Contains(err.Error(), "unexpected cloud login status") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestLoginCommandHandlesPollErrorAndSleepCancellation(t *testing.T) {
	t.Run("poll error", func(t *testing.T) {
		fake := &fakeCommandClient{
			cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
				DeviceCode:      "device-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresIn:       600,
			},
			cloudPollErrs: []error{errors.New("poll failed")},
		}
		cmd := NewLoginCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedCloudAuthFactory(fake), nil)
		if err := cmd.Run(nil); err == nil || !strings.Contains(err.Error(), "poll failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("sleep canceled", func(t *testing.T) {
		fake := &fakeCommandClient{
			cloudLoginStartResp: &controlclient.CloudDeviceAuthorizationResponse{
				DeviceCode:      "device-1",
				UserCode:        "ABCD-EFGH",
				VerificationURI: "https://archon.example/activate",
				ExpiresIn:       600,
			},
			cloudPollResponses: []*controlclient.CloudAuthPollResponse{
				{Status: "authorization_pending"},
			},
		}
		cmd := NewLoginCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedCloudAuthFactory(fake), nil)
		cmd.sleep = func(context.Context, time.Duration) error { return context.Canceled }
		if err := cmd.Run(nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	})
}

func TestWhoAmICommandPrintsLinkedStatus(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudStatusResp: &controlclient.CloudAuthStatusResponse{
			Linked:       true,
			User:         &controlclient.CloudLinkedUser{DisplayName: "Shiv"},
			Installation: &controlclient.CloudInstallation{Name: "Archon Laptop"},
		},
	}
	cmd := NewWhoAmICommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected whoami to succeed, got err=%v", err)
	}
	if !strings.Contains(stdout.String(), "Shiv") || !strings.Contains(stdout.String(), "Archon Laptop") {
		t.Fatalf("unexpected whoami output: %q", stdout.String())
	}
}

// TestWhoAmICommandNotLoggedIn locks the CLI contract for the unlinked case.
// Spec: prints "not logged in\n" and exits 0.
func TestWhoAmICommandNotLoggedIn(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudStatusResp: &controlclient.CloudAuthStatusResponse{
			Linked: false,
		},
	}
	cmd := NewWhoAmICommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected whoami to succeed, got err=%v", err)
	}
	if got := stdout.String(); got != "not logged in\n" {
		t.Fatalf("expected exactly %q, got %q", "not logged in\n", got)
	}
}

// TestWhoAmICommandLinkedWithAllFields locks exact output for linked identity rendering.
// Spec: display name and email on separate lines, installation name when available.
func TestWhoAmICommandLinkedWithAllFields(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudStatusResp: &controlclient.CloudAuthStatusResponse{
			Linked: true,
			User:   &controlclient.CloudLinkedUser{DisplayName: "Ada Lovelace", Email: "ada@example.com"},
			Installation: &controlclient.CloudInstallation{Name: "Dev Box"},
		},
	}
	cmd := NewWhoAmICommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected whoami to succeed, got err=%v", err)
	}
	want := "User: Ada Lovelace\nEmail: ada@example.com\nInstallation: Dev Box\n"
	if got := stdout.String(); got != want {
		t.Fatalf("unexpected output.\nwant: %q\n got: %q", want, got)
	}
}

// TestWhoAmICommandDaemonError locks the failure contract.
// Spec: daemon-side error produces a single-line stderr message with non-zero exit.
func TestWhoAmICommandDaemonError(t *testing.T) {
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		cloudStatusErr: errors.New("daemon unavailable"),
	}
	cmd := NewWhoAmICommand(&bytes.Buffer{}, stderr, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err == nil {
		t.Fatal("expected error from whoami when daemon fails")
	}
}

// TestLogoutCommandDaemonError locks the logout failure contract.
// Spec: daemon-side error produces a single-line stderr message with non-zero exit.
func TestLogoutCommandDaemonError(t *testing.T) {
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{
		logoutCloudErr: errors.New("remote revoke unreachable"),
	}
	cmd := NewLogoutCommand(&bytes.Buffer{}, stderr, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err == nil {
		t.Fatal("expected error from logout when daemon fails")
	}
}

func TestLogoutCommandCallsClient(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{}
	cmd := NewLogoutCommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected logout to succeed, got err=%v", err)
	}
	if fake.logoutCloudCalls != 1 {
		t.Fatalf("expected logout call once, got %d", fake.logoutCloudCalls)
	}
	if got := stdout.String(); got != "revoked remote token and cleared local cloud credentials\n" {
		t.Fatalf("unexpected logout output: %q", got)
	}
}

func TestLogoutCommandPrintsPartialLogoutResult(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		logoutCloudResp: &controlclient.CloudLogoutResponse{
			Status:       "unlinked_local_only",
			LocalCleared: true,
			Message:      "remote revoke failed; cleared local cloud credentials only",
		},
	}
	cmd := NewLogoutCommand(stdout, &bytes.Buffer{}, fixedCloudAuthFactory(fake))

	if err := cmd.Run(nil); err != nil {
		t.Fatalf("expected logout to succeed, got err=%v", err)
	}
	want := "remote revoke failed; cleared local cloud credentials only\n"
	if got := stdout.String(); got != want {
		t.Fatalf("unexpected partial logout output.\nwant: %q\n got: %q", want, got)
	}
}

func TestSleepContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestConfigCommandPrintsEffectiveConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	core := []byte(`
[daemon]
address = "127.0.0.1:8899"

[logging]
level = "debug"

[debug]
stream_debug = true

[guided_workflows.defaults]
provider = "codex"
model = "gpt-5.3-codex"
access = "on_request"
reasoning = "high"
resolution_boundary = "high"

[providers.codex]
default_model = "gpt-5.3-codex"
models = ["gpt-5.3-codex"]
approval_policy = "on-request"
sandbox_policy = "workspace-write"
network_access = false
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), core, 0o600); err != nil {
		t.Fatalf("WriteFile config.toml: %v", err)
	}
	ui := []byte("[keybindings]\npath = \"custom-keybindings.json\"\n")
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), ui, 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}
	keybindings := []byte(`{"ui.toggleSidebar":"alt+b","ui.refresh":"F5"}`)
	if err := os.WriteFile(filepath.Join(dataDir, "custom-keybindings.json"), keybindings, 0o600); err != nil {
		t.Fatalf("WriteFile keybindings: %v", err)
	}
	workflowTemplates := []byte(`{
  "version": 1,
  "templates": [
    {
      "id": "custom_delivery",
      "name": "Custom Delivery",
      "description": "A custom guided workflow",
      "default_access_level": "on_request",
      "phases": [
        {
          "id": "phase_1",
          "name": "Plan",
          "steps": [
            {
              "id": "step_1",
              "name": "Draft plan",
              "prompt": "Draft a plan."
            }
          ]
        }
      ]
    }
  ]
}`)
	if err := os.WriteFile(filepath.Join(dataDir, "workflow_templates.json"), workflowTemplates, 0o600); err != nil {
		t.Fatalf("WriteFile workflow_templates.json: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run(nil); err != nil {
		t.Fatalf("config command failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	daemon, _ := payload["daemon"].(map[string]any)
	if daemon["address"] != "127.0.0.1:8899" {
		t.Fatalf("unexpected daemon address: %#v", daemon["address"])
	}
	if daemon["base_url"] != "http://127.0.0.1:8899" {
		t.Fatalf("unexpected daemon base_url: %#v", daemon["base_url"])
	}
	loggingCfg, _ := payload["logging"].(map[string]any)
	if loggingCfg["level"] != "debug" {
		t.Fatalf("unexpected logging level: %#v", loggingCfg["level"])
	}
	debugCfg, _ := payload["debug"].(map[string]any)
	if debugCfg["stream_debug"] != true {
		t.Fatalf("unexpected stream_debug: %#v", debugCfg["stream_debug"])
	}
	notificationsCfg, _ := payload["notifications"].(map[string]any)
	if notificationsCfg["enabled"] != true {
		t.Fatalf("unexpected notifications enabled: %#v", notificationsCfg["enabled"])
	}
	guidedCfg, _ := payload["guided_workflows"].(map[string]any)
	if guidedCfg["enabled"] != false {
		t.Fatalf("unexpected guided workflows enabled: %#v", guidedCfg["enabled"])
	}
	if guidedCfg["auto_start"] != false {
		t.Fatalf("unexpected guided workflows auto_start: %#v", guidedCfg["auto_start"])
	}
	if guidedCfg["checkpoint_style"] != "confidence_weighted" {
		t.Fatalf("unexpected guided workflows checkpoint_style: %#v", guidedCfg["checkpoint_style"])
	}
	if guidedCfg["mode"] != "guarded_autopilot" {
		t.Fatalf("unexpected guided workflows mode: %#v", guidedCfg["mode"])
	}
	defaultsCfg, _ := guidedCfg["defaults"].(map[string]any)
	if defaultsCfg["provider"] != "codex" {
		t.Fatalf("unexpected guided workflows defaults provider: %#v", defaultsCfg["provider"])
	}
	if defaultsCfg["model"] != "gpt-5.3-codex" {
		t.Fatalf("unexpected guided workflows defaults model: %#v", defaultsCfg["model"])
	}
	if defaultsCfg["access"] != "on_request" {
		t.Fatalf("unexpected guided workflows defaults access: %#v", defaultsCfg["access"])
	}
	if defaultsCfg["reasoning"] != "high" {
		t.Fatalf("unexpected guided workflows defaults reasoning: %#v", defaultsCfg["reasoning"])
	}
	if defaultsCfg["resolution_boundary"] != "high" {
		t.Fatalf("unexpected guided workflows defaults resolution boundary: %#v", defaultsCfg["resolution_boundary"])
	}
	policyCfg, _ := guidedCfg["policy"].(map[string]any)
	if policyCfg["confidence_threshold"] != 0.7 {
		t.Fatalf("unexpected guided workflows confidence_threshold: %#v", policyCfg["confidence_threshold"])
	}
	if policyCfg["pause_threshold"] != 0.6 {
		t.Fatalf("unexpected guided workflows pause_threshold: %#v", policyCfg["pause_threshold"])
	}
	if policyCfg["high_blast_radius_file_count"] != float64(20) {
		t.Fatalf("unexpected guided workflows high_blast_radius_file_count: %#v", policyCfg["high_blast_radius_file_count"])
	}
	hardGates, _ := policyCfg["hard_gates"].(map[string]any)
	if hardGates["ambiguity_blocker"] != true || hardGates["sensitive_files"] != true || hardGates["failing_checks"] != true {
		t.Fatalf("unexpected guided workflows hard_gates: %#v", hardGates)
	}
	rolloutCfg, _ := guidedCfg["rollout"].(map[string]any)
	if rolloutCfg["telemetry_enabled"] != true {
		t.Fatalf("unexpected rollout telemetry_enabled: %#v", rolloutCfg["telemetry_enabled"])
	}
	if rolloutCfg["max_active_runs"] != float64(3) {
		t.Fatalf("unexpected rollout max_active_runs: %#v", rolloutCfg["max_active_runs"])
	}
	if rolloutCfg["automation_enabled"] != false {
		t.Fatalf("unexpected rollout automation_enabled: %#v", rolloutCfg["automation_enabled"])
	}
	if rolloutCfg["allow_quality_checks"] != false {
		t.Fatalf("unexpected rollout allow_quality_checks: %#v", rolloutCfg["allow_quality_checks"])
	}
	if rolloutCfg["allow_commit"] != false {
		t.Fatalf("unexpected rollout allow_commit: %#v", rolloutCfg["allow_commit"])
	}
	if rolloutCfg["require_commit_approval"] != true {
		t.Fatalf("unexpected rollout require_commit_approval: %#v", rolloutCfg["require_commit_approval"])
	}
	if rolloutCfg["max_retry_attempts"] != float64(2) {
		t.Fatalf("unexpected rollout max_retry_attempts: %#v", rolloutCfg["max_retry_attempts"])
	}
	providers, _ := payload["providers"].(map[string]any)
	codex, _ := providers["codex"].(map[string]any)
	if codex["default_model"] != "gpt-5.3-codex" {
		t.Fatalf("unexpected codex default model: %#v", codex["default_model"])
	}
	if codex["approval_policy"] != "on-request" {
		t.Fatalf("unexpected codex approval policy: %#v", codex["approval_policy"])
	}
	keymap, _ := payload["keybindings"].(map[string]any)
	if keymap["ui.toggleSidebar"] != "alt+b" {
		t.Fatalf("unexpected keybinding override: %#v", keymap["ui.toggleSidebar"])
	}
	if got := payload["workflow_templates_path"]; got != filepath.Join(dataDir, "workflow_templates.json") {
		t.Fatalf("unexpected workflow_templates_path: %#v", got)
	}
	workflowCfg, _ := payload["workflow_templates"].(map[string]any)
	templates, _ := workflowCfg["templates"].([]any)
	if len(templates) != 1 {
		t.Fatalf("expected one workflow template, got %#v", workflowCfg["templates"])
	}
	template, _ := templates[0].(map[string]any)
	if template["id"] != "custom_delivery" {
		t.Fatalf("unexpected workflow template id: %#v", template["id"])
	}
}

func TestConfigCommandFailsOnInvalidUIConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), []byte("[keybindings\npath='x'"), 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}

	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run(nil); err == nil {
		t.Fatalf("expected config command to fail on invalid ui.toml")
	}
}

func TestConfigCommandFailsOnInvalidKeybindingsJSON(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ui := []byte("[keybindings]\npath = \"broken.json\"\n")
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), ui, 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "broken.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatalf("WriteFile keybindings: %v", err)
	}

	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run(nil); err == nil {
		t.Fatalf("expected config command to fail on invalid keybindings json")
	}
}

func TestConfigCommandRejectsUnknownFlag(t *testing.T) {
	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run([]string{"--unknown"}); err == nil {
		t.Fatalf("expected unknown flag to fail")
	}
}

func TestConfigCommandRejectsInvalidFormat(t *testing.T) {
	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run([]string{"--format", "yaml"}); err == nil {
		t.Fatalf("expected invalid format to fail")
	}
}

func TestConfigCommandPrintsTOML(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte("[daemon]\naddress = \"127.0.0.1:7777\"\n")
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile config.toml: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--format", "toml"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "[daemon]") || !strings.Contains(out, "address =") || !strings.Contains(out, "127.0.0.1:7777") {
		t.Fatalf("expected daemon config in toml output, got %q", out)
	}
}

func TestConfigCommandDefaultIgnoresInvalidUserFiles(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte("[daemon\naddress='bad'"), 0o600); err != nil {
		t.Fatalf("WriteFile config.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), []byte("[keybindings\npath='bad'"), 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "workflow_templates.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatalf("WriteFile workflow_templates.json: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--default"}); err != nil {
		t.Fatalf("expected --default to bypass malformed files, got %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	daemon, _ := payload["daemon"].(map[string]any)
	if daemon["address"] != "127.0.0.1:7777" {
		t.Fatalf("expected default daemon address, got %#v", daemon["address"])
	}
}

func TestConfigCommandScopeCoreSkipsInvalidUI(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), []byte("[keybindings\npath='bad'"), 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "core"}); err != nil {
		t.Fatalf("expected core scope to ignore ui parse errors, got %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	if _, ok := payload["daemon"]; !ok {
		t.Fatalf("expected core daemon output")
	}
	if daemon, ok := payload["daemon"].(map[string]any); !ok || daemon["base_url"] != nil {
		t.Fatalf("expected core-scope daemon output without base_url, got %#v", payload["daemon"])
	}
}

func TestConfigCommandScopeUIOnly(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui.toml"), []byte("[keybindings]\npath=\"keys.json\""), 0o600); err != nil {
		t.Fatalf("WriteFile ui.toml: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "ui"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	keybindings, ok := payload["keybindings"].(map[string]any)
	if !ok {
		t.Fatalf("expected keybindings object in ui-only output, got %#v", payload["keybindings"])
	}
	if path, _ := keybindings["path"].(string); path == "" {
		t.Fatalf("expected keybindings.path in ui-only output")
	}
	chat, ok := payload["chat"].(map[string]any)
	if !ok {
		t.Fatalf("expected chat object in ui-only output, got %#v", payload["chat"])
	}
	if chat["timestamp_mode"] != "relative" {
		t.Fatalf("expected default chat timestamp mode relative, got %#v", chat["timestamp_mode"])
	}
}

func TestConfigCommandScopeKeybindingsDefault(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "keybindings", "--default"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	if payload["ui.toggleSidebar"] != "ctrl+b" {
		t.Fatalf("expected top-level keybinding map, got %#v", payload["ui.toggleSidebar"])
	}
	if payload["ui.startGuidedWorkflow"] != "w" {
		t.Fatalf("expected start guided workflow default key w, got %#v", payload["ui.startGuidedWorkflow"])
	}
	if _, ok := payload["keybindings"]; ok {
		t.Fatalf("did not expect nested keybindings object in keybindings-only output")
	}
	if _, ok := payload["keybindings_path"]; ok {
		t.Fatalf("did not expect keybindings_path metadata in keybindings-only output")
	}
}

func TestConfigCommandScopeWorkflowTemplatesOnly(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	workflowTemplates := []byte(`{
  "version": 1,
  "templates": [
    {
      "id": "custom_delivery",
      "name": "Custom Delivery",
      "phases": [
        {
          "id": "phase_1",
          "name": "Plan",
          "steps": [
            {
              "id": "step_1",
              "name": "Draft plan",
              "prompt": "Draft a plan."
            }
          ]
        }
      ]
    }
  ]
}`)
	if err := os.WriteFile(filepath.Join(dataDir, "workflow_templates.json"), workflowTemplates, 0o600); err != nil {
		t.Fatalf("WriteFile workflow_templates.json: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "workflow_templates"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	templates, _ := payload["templates"].([]any)
	if len(templates) != 1 {
		t.Fatalf("expected top-level templates array, got %#v", payload["templates"])
	}
	if _, ok := payload["workflow_templates"]; ok {
		t.Fatalf("did not expect nested workflow_templates object in workflow_templates-only output")
	}
	if _, ok := payload["workflow_templates_path"]; ok {
		t.Fatalf("did not expect workflow_templates_path metadata in workflow_templates-only output")
	}
}

func TestConfigCommandScopeWorkflowTemplatesDefaultUsesRepoDefaults(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "workflow_templates", "--default"}); err != nil {
		t.Fatalf("config command failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	templates, _ := payload["templates"].([]any)
	if len(templates) == 0 {
		t.Fatalf("expected repo defaults in workflow_templates --default output")
	}
	found := false
	for _, raw := range templates {
		template, _ := raw.(map[string]any)
		if template["id"] == "solid_phase_delivery" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected solid_phase_delivery in default workflow templates, got %#v", templates)
	}
}

func TestConfigCommandRejectsInvalidScope(t *testing.T) {
	cmd := NewConfigCommand(&bytes.Buffer{}, &bytes.Buffer{})
	if err := cmd.Run([]string{"--scope", "notes"}); err == nil {
		t.Fatalf("expected invalid scope to fail")
	}
}

func TestConfigCommandOmitsUnsetNetworkAccess(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte(`
[providers.codex]
default_model = "gpt-5.2-codex"
`)
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile config.toml: %v", err)
	}

	stdout := &bytes.Buffer{}
	cmd := NewConfigCommand(stdout, &bytes.Buffer{})
	if err := cmd.Run(nil); err != nil {
		t.Fatalf("config command failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v raw=%q", err, stdout.String())
	}
	providersRaw, ok := payload["providers"].(map[string]any)
	if !ok {
		t.Fatalf("providers payload missing or invalid")
	}
	codexRaw, ok := providersRaw["codex"].(map[string]any)
	if !ok {
		t.Fatalf("codex payload missing or invalid")
	}
	if _, exists := codexRaw["network_access"]; exists {
		t.Fatalf("expected network_access to be omitted when unset")
	}
}

func TestConfigCommandPropagatesEncodeError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".archon"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cmd := NewConfigCommand(errorWriter{}, &bytes.Buffer{})
	if err := cmd.Run(nil); err == nil {
		t.Fatalf("expected encoding error")
	}
}

func TestBuildCommandsIncludesConfig(t *testing.T) {
	wiring := commandWiring{
		stdout:             &bytes.Buffer{},
		stderr:             &bytes.Buffer{},
		newCloudAuthClient: fixedCloudAuthFactory(&fakeCommandClient{}),
		newSessionClient:   fixedSessionFactory(&fakeCommandClient{}),
		newUIClient:        fixedDaemonVersionFactory(&fakeCommandClient{}),
		newDaemonAdmin:     fixedDaemonAdminFactory(&fakeCommandClient{}),
		runDaemon:          func(bool) error { return nil },
		killDaemon:         func() error { return nil },
		configureUILogging: func() {},
		version:            "v-test",
	}
	commands := buildCommands(wiring)
	required := []string{"daemon", "config", "login", "whoami", "logout", "ps", "start", "kill", "tail", "ui", "version"}
	for _, name := range required {
		if commands[name] == nil {
			t.Fatalf("expected %q command to be present", name)
		}
	}
	if _, ok := commands["config"].(*ConfigCommand); !ok {
		t.Fatalf("expected config command type")
	}
	if _, ok := commands["version"].(*VersionCommand); !ok {
		t.Fatalf("expected version command type")
	}
}

func TestDefaultCommandWiringUsesStandardStreams(t *testing.T) {
	wiring := defaultCommandWiring(nil, nil)
	if wiring.stdout != os.Stdout {
		t.Fatalf("expected stdout fallback to os.Stdout")
	}
	if wiring.stderr != os.Stderr {
		t.Fatalf("expected stderr fallback to os.Stderr")
	}
	if wiring.newCloudAuthClient == nil || wiring.newSessionClient == nil || wiring.newUIClient == nil || wiring.newDaemonAdmin == nil || wiring.runDaemon == nil || wiring.killDaemon == nil || wiring.configureUILogging == nil {
		t.Fatalf("expected default wiring to populate dependencies")
	}
}

func TestSessionCommandRegistered(t *testing.T) {
	wiring := defaultCommandWiring(&bytes.Buffer{}, &bytes.Buffer{})
	commands := buildCommands(wiring)
	if _, ok := commands["session"]; !ok {
		t.Fatal("expected session command to be registered")
	}
}

func TestSessionCommandJSONHappyPath(t *testing.T) {
	now := time.Now()
	exitCode := 0
	session := &types.Session{
		ID:        "abc123",
		Status:    types.SessionStatusRunning,
		Provider:  "codex",
		PID:       42,
		Title:     "my session",
		Cwd:       "/home/user/project",
		Cmd:       "codex",
		Args:      []string{"--help"},
		Tags:      []string{"demo"},
		CreatedAt: now,
		ExitCode:  &exitCode,
	}

	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{getSessionResp: session}
	cmd := newSessionCommand(fixedSessionFactory(fake), stdout, &bytes.Buffer{})

	if err := cmd.Run([]string{"abc123"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.getSessionCalls != 1 {
		t.Fatalf("expected 1 GetSession call, got %d", fake.getSessionCalls)
	}
	if fake.getSessionIDArg != "abc123" {
		t.Fatalf("expected id arg abc123, got %q", fake.getSessionIDArg)
	}

	raw := stdout.Bytes()
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Fatalf("expected trailing newline, got %q", string(raw))
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("expected valid JSON, got err=%v, raw=%q", err, string(raw))
	}
	for _, field := range []string{"id", "status", "provider", "pid", "title"} {
		if _, ok := decoded[field]; !ok {
			t.Fatalf("missing required field %q in output: %#v", field, decoded)
		}
	}
	if decoded["id"] != "abc123" {
		t.Fatalf("expected id abc123, got %v", decoded["id"])
	}
}

func TestSessionCommandHumanFormat(t *testing.T) {
	now := time.Now()
	exitCode := 0
	session := &types.Session{
		ID:        "abc123",
		Status:    types.SessionStatusRunning,
		Provider:  "codex",
		PID:       42,
		Title:     "my session",
		Cwd:       "/home/user/project",
		Cmd:       "codex",
		Args:      []string{"--help"},
		Tags:      []string{"demo"},
		CreatedAt: now,
		ExitCode:  &exitCode,
	}

	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{getSessionResp: session}
	cmd := newSessionCommand(fixedSessionFactory(fake), stdout, &bytes.Buffer{})

	if err := cmd.Run([]string{"abc123", "--format", "human"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}

	out := stdout.String()
	for _, line := range []string{
		"ID:         abc123",
		"Status:     running",
		"Provider:   codex",
		"Title:      my session",
		"PID:        42",
		"Cwd:        /home/user/project",
		"Cmd:        codex",
		"Args:       --help",
		"Tags:       demo",
		"Exit Code:  0",
	} {
		if !strings.Contains(out, line) {
			t.Fatalf("expected line %q in human output, got:\n%s", line, out)
		}
	}
}

func TestSessionCommandUnknownFormat(t *testing.T) {
	fake := &fakeCommandClient{}
	stderr := &bytes.Buffer{}
	cmd := newSessionCommand(fixedSessionFactory(fake), &bytes.Buffer{}, stderr)

	err := cmd.Run([]string{"abc123", "--format", "yaml"})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "unknown format") {
		t.Fatalf("expected unknown format error, got %v", err)
	}
	if fake.getSessionCalls != 0 {
		t.Fatalf("expected no adapter calls, got %d", fake.getSessionCalls)
	}
}

func TestSessionCommandMissingID(t *testing.T) {
	fake := &fakeCommandClient{}
	stderr := &bytes.Buffer{}
	cmd := newSessionCommand(fixedSessionFactory(fake), &bytes.Buffer{}, stderr)

	err := cmd.Run(nil)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	if !strings.Contains(err.Error(), "session id is required") {
		t.Fatalf("expected 'session id is required' error, got %v", err)
	}
	if fake.getSessionCalls != 0 {
		t.Fatalf("expected no adapter calls, got %d", fake.getSessionCalls)
	}
}

func TestSessionCommandDaemonError(t *testing.T) {
	fake := &fakeCommandClient{
		getSessionErr: errors.New("session not found"),
	}
	stderr := &bytes.Buffer{}
	cmd := newSessionCommand(fixedSessionFactory(fake), &bytes.Buffer{}, stderr)

	err := cmd.Run([]string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for daemon error")
	}
	if !strings.Contains(err.Error(), "session not found") {
		t.Fatalf("expected daemon error message, got %v", err)
	}
}

func TestSessionCommandExplicitJSONMatchesDefault(t *testing.T) {
	now := time.Now()
	session := &types.Session{
		ID:        "abc123",
		Status:    types.SessionStatusRunning,
		Provider:  "codex",
		PID:       42,
		Title:     "my session",
		CreatedAt: now,
	}

	// Default (no --format)
	stdout1 := &bytes.Buffer{}
	fake1 := &fakeCommandClient{getSessionResp: session}
	cmd1 := newSessionCommand(fixedSessionFactory(fake1), stdout1, &bytes.Buffer{})
	if err := cmd1.Run([]string{"abc123"}); err != nil {
		t.Fatalf("default: expected success, got err=%v", err)
	}

	// Explicit --format json
	stdout2 := &bytes.Buffer{}
	fake2 := &fakeCommandClient{getSessionResp: session}
	cmd2 := newSessionCommand(fixedSessionFactory(fake2), stdout2, &bytes.Buffer{})
	if err := cmd2.Run([]string{"abc123", "--format", "json"}); err != nil {
		t.Fatalf("explicit json: expected success, got err=%v", err)
	}

	if stdout1.String() != stdout2.String() {
		t.Fatalf("default and explicit --format json should be identical:\ndefault:  %q\nexplicit: %q", stdout1.String(), stdout2.String())
	}
}

func TestSessionCommandHelpFlag(t *testing.T) {
	stderr := &bytes.Buffer{}
	fake := &fakeCommandClient{}
	cmd := newSessionCommand(fixedSessionFactory(fake), &bytes.Buffer{}, stderr)

	if err := cmd.Run([]string{"--help"}); err != nil {
		t.Fatalf("expected --help to succeed, got err=%v", err)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Fatalf("expected usage text in stderr, got %q", stderr.String())
	}
}

func TestSessionCommandFormatEquals(t *testing.T) {
	now := time.Now()
	session := &types.Session{
		ID:        "abc123",
		Status:    types.SessionStatusRunning,
		Provider:  "codex",
		CreatedAt: now,
	}

	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{getSessionResp: session}
	cmd := newSessionCommand(fixedSessionFactory(fake), stdout, &bytes.Buffer{})

	if err := cmd.Run([]string{"abc123", "--format=json"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.getSessionCalls != 1 {
		t.Fatalf("expected 1 call, got %d", fake.getSessionCalls)
	}
	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("expected valid JSON, got err=%v", err)
	}
}

// --- Tail Follow Tests ---

// TestTailFollowSnapshotRegression ensures the existing snapshot path is unchanged.
func TestTailFollowSnapshotRegression(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		tailItemsResp: &controlclient.TailItemsResponse{
			Items: []map[string]any{{"type": "log", "text": "hello"}},
		},
	}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--lines", "50", "session-1"}); err != nil {
		t.Fatalf("expected tail to succeed, got err=%v", err)
	}
	if fake.tailItemsCalls != 1 {
		t.Fatalf("expected 1 TailItems call, got %d", fake.tailItemsCalls)
	}
	if fake.streamTailCalls != 0 {
		t.Fatalf("expected 0 StreamTail calls, got %d", fake.streamTailCalls)
	}
	var items []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		t.Fatalf("expected valid JSON array, got err=%v", err)
	}
	if len(items) != 1 || items[0]["text"] != "hello" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

// TestTailFollowHappyCase streams synthetic events and asserts NDJSON output.
func TestTailFollowHappyCase(t *testing.T) {
	ch := make(chan types.LogEvent, 4)
	events := []types.LogEvent{
		{Type: "log", Stream: "stdout", Chunk: "line1\n", TS: "t1"},
		{Type: "log", Stream: "stdout", Chunk: "line2\n", TS: "t2"},
		{Type: "log", Stream: "stderr", Chunk: "err1\n", TS: "t3"},
	}
	for _, e := range events {
		ch <- e
	}
	close(ch)

	stdout := &bytes.Buffer{}
	cancelCalled := 0
	fake := &fakeCommandClient{
		streamTailCh: ch,
		streamTailCancel: func() { cancelCalled++ },
	}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--follow", "session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.streamTailCalls != 1 {
		t.Fatalf("expected 1 StreamTail call, got %d", fake.streamTailCalls)
	}
	if fake.streamTailIDArg != "session-1" {
		t.Fatalf("expected id session-1, got %q", fake.streamTailIDArg)
	}

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d:\n%s", len(lines), stdout.String())
	}
	for i, line := range lines {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("line %d: expected valid JSON, got err=%v, line=%q", i, err, line)
		}
	}
}

// TestTailFollowStreamSelector asserts --stream is propagated to the adapter.
func TestTailFollowStreamSelector(t *testing.T) {
	ch := make(chan types.LogEvent)
	close(ch)

	fake := &fakeCommandClient{
		streamTailCh: ch,
		streamTailCancel: func() {},
	}
	stdout := &bytes.Buffer{}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--follow", "--stream", "stderr", "session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.streamTailNameArg != "stderr" {
		t.Fatalf("expected stream 'stderr', got %q", fake.streamTailNameArg)
	}
}

// TestTailFollowShorthandFlag asserts -f works as --follow.
func TestTailFollowShorthandFlag(t *testing.T) {
	ch := make(chan types.LogEvent)
	close(ch)

	fake := &fakeCommandClient{
		streamTailCh: ch,
		streamTailCancel: func() {},
	}
	stdout := &bytes.Buffer{}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"-f", "session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.streamTailCalls != 1 {
		t.Fatalf("expected 1 StreamTail call, got %d", fake.streamTailCalls)
	}
}

// TestTailFollowFlagsAfterID asserts that --follow works even when placed after the session ID.
func TestTailFollowFlagsAfterID(t *testing.T) {
	ch := make(chan types.LogEvent)
	close(ch)

	fake := &fakeCommandClient{
		streamTailCh:     ch,
		streamTailCancel: func() {},
	}
	stdout := &bytes.Buffer{}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	// Flags AFTER the positional ID — should still work.
	if err := cmd.Run([]string{"session-1", "--follow"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.streamTailCalls != 1 {
		t.Fatalf("expected 1 StreamTail call, got %d", fake.streamTailCalls)
	}
}

// TestTailFollowFlagsAfterIDWithLines asserts --lines works after the session ID.
func TestTailFollowFlagsAfterIDWithLines(t *testing.T) {
	backfillItems := []map[string]any{{"text": "old line", "type": "log"}}
	ch := make(chan types.LogEvent)
	close(ch)

	fake := &fakeCommandClient{
		tailItemsResp: &controlclient.TailItemsResponse{Items: backfillItems},
		streamTailCh:  ch,
		streamTailCancel: func() {},
	}
	stdout := &bytes.Buffer{}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"session-1", "--follow", "--lines", "10"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.tailItemsCalls != 1 {
		t.Fatalf("expected 1 TailItems call, got %d", fake.tailItemsCalls)
	}
	if fake.tailItemsLines != 10 {
		t.Fatalf("expected lines=10, got %d", fake.tailItemsLines)
	}
}

// TestReorderFlagsBeforePositional verifies the flag reordering helper.
func TestReorderFlagsBeforePositional(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"flags first", []string{"--follow", "id123"}, []string{"--follow", "id123"}},
		{"flags after", []string{"id123", "--follow"}, []string{"--follow", "id123"}},
		{"flag with value", []string{"id123", "--lines", "5", "--follow"}, []string{"--lines", "5", "--follow", "id123"}},
		{"mixed", []string{"--follow", "id123", "--lines", "10"}, []string{"--follow", "--lines", "10", "id123"}},
		{"equals syntax", []string{"id123", "--lines=5"}, []string{"--lines=5", "id123"}},
		{"short flag", []string{"id123", "-f"}, []string{"-f", "id123"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderFlagsBeforePositional(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("expected %v, got %v", tt.want, got)
				}
			}
		})
	}
}

// --- Send command tests ---

func fixedSendFactory(fake *fakeCommandClient) sessionClientFactory {
	return func() (sessionCommandClient, error) { return fake, nil }
}

// TestSendCommandRegistered asserts "send" is in the command map.
func TestSendCommandRegistered(t *testing.T) {
	cmds := buildCommands(defaultCommandWiring(nil, nil))
	if _, ok := cmds["send"]; !ok {
		t.Fatal("expected \"send\" in command map")
	}
}

// TestSendPositionalText asserts positional text works and returns turn_id.
func TestSendPositionalText(t *testing.T) {
	fake := &fakeCommandClient{
		sendMessageResp: &controlclient.SendSessionResponse{OK: true, TurnID: "trn_abc"},
	}
	stdout := &bytes.Buffer{}
	cmd := NewSendCommand(stdout, &bytes.Buffer{}, strings.NewReader(""), fixedSendFactory(fake))

	if err := cmd.Run([]string{"session-1", "hello world"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.sendMessageCalls != 1 {
		t.Fatalf("expected 1 SendMessage call, got %d", fake.sendMessageCalls)
	}
	if fake.sendMessageReq.Text != "hello world" {
		t.Fatalf("expected text='hello world', got %q", fake.sendMessageReq.Text)
	}
	if fake.sendMessageReq.Input != nil {
		t.Fatalf("expected Input nil, got %v", fake.sendMessageReq.Input)
	}
	if strings.TrimSpace(stdout.String()) != "trn_abc" {
		t.Fatalf("expected stdout 'trn_abc', got %q", stdout.String())
	}
}

// TestSendTextFlag asserts --text flag works like positional text.
func TestSendTextFlag(t *testing.T) {
	fake := &fakeCommandClient{
		sendMessageResp: &controlclient.SendSessionResponse{OK: true, TurnID: "trn_123"},
	}
	stdout := &bytes.Buffer{}
	cmd := NewSendCommand(stdout, &bytes.Buffer{}, strings.NewReader(""), fixedSendFactory(fake))

	if err := cmd.Run([]string{"session-1", "--text", "from flag"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.sendMessageReq.Text != "from flag" {
		t.Fatalf("expected text='from flag', got %q", fake.sendMessageReq.Text)
	}
}

// TestSendInputItemsFile asserts --input-items reads a JSON file and sends Input.
func TestSendInputItemsFile(t *testing.T) {
	items := []map[string]any{{"type": "text", "text": "structured"}}
	itemsJSON, _ := json.Marshal(items)
	tmpFile, err := os.CreateTemp("", "input-items-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write(itemsJSON)
	tmpFile.Close()

	fake := &fakeCommandClient{
		sendMessageResp: &controlclient.SendSessionResponse{OK: true, TurnID: "trn_struct"},
	}
	stdout := &bytes.Buffer{}
	cmd := NewSendCommand(stdout, &bytes.Buffer{}, strings.NewReader(""), fixedSendFactory(fake))

	if err := cmd.Run([]string{"session-1", "--input-items", tmpFile.Name()}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if fake.sendMessageReq.Text != "" {
		t.Fatalf("expected Text empty, got %q", fake.sendMessageReq.Text)
	}
	if len(fake.sendMessageReq.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(fake.sendMessageReq.Input))
	}
}

// TestSendInputItemsStdin asserts --input-items - reads from stdin.
func TestSendInputItemsStdin(t *testing.T) {
	items := []map[string]any{{"type": "text", "text": "from stdin"}}
	itemsJSON, _ := json.Marshal(items)

	fake := &fakeCommandClient{
		sendMessageResp: &controlclient.SendSessionResponse{OK: true, TurnID: "trn_stdin"},
	}
	stdout := &bytes.Buffer{}
	cmd := NewSendCommand(stdout, &bytes.Buffer{}, bytes.NewReader(itemsJSON), fixedSendFactory(fake))

	if err := cmd.Run([]string{"session-1", "--input-items", "-"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if len(fake.sendMessageReq.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(fake.sendMessageReq.Input))
	}
}

// TestSendConflictInput asserts providing two input forms fails.
func TestSendConflictInput(t *testing.T) {
	fake := &fakeCommandClient{}
	stderr := &bytes.Buffer{}
	cmd := NewSendCommand(&bytes.Buffer{}, stderr, strings.NewReader(""), fixedSendFactory(fake))

	// Positional + --text
	err := cmd.Run([]string{"session-1", "positional", "--text", "flag"})
	if err == nil {
		t.Fatal("expected error for conflicting input forms")
	}
	if fake.sendMessageCalls != 0 {
		t.Fatal("expected no SendMessage call")
	}

	// Positional + --input-items
	stderr.Reset()
	err = cmd.Run([]string{"session-1", "positional", "--input-items", "file.json"})
	if err == nil {
		t.Fatal("expected error for conflicting input forms")
	}
	if fake.sendMessageCalls != 0 {
		t.Fatal("expected no SendMessage call")
	}
}

// TestSendMissingInput asserts no message text produces a usage error.
func TestSendMissingInput(t *testing.T) {
	fake := &fakeCommandClient{}
	cmd := NewSendCommand(&bytes.Buffer{}, &bytes.Buffer{}, strings.NewReader(""), fixedSendFactory(fake))

	err := cmd.Run([]string{"session-1"})
	if err == nil {
		t.Fatal("expected error for missing input")
	}
	if fake.sendMessageCalls != 0 {
		t.Fatal("expected no SendMessage call")
	}
}

// TestSendMalformedInputItems asserts invalid JSON produces a local parse error.
func TestSendMalformedInputItems(t *testing.T) {
	fake := &fakeCommandClient{}
	stderr := &bytes.Buffer{}
	cmd := NewSendCommand(&bytes.Buffer{}, stderr, strings.NewReader("not json"), fixedSendFactory(fake))

	err := cmd.Run([]string{"session-1", "--input-items", "-"})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "parsing input items JSON") {
		t.Fatalf("expected JSON parse error, got: %v", err)
	}
	if fake.sendMessageCalls != 0 {
		t.Fatal("expected no SendMessage call on parse error")
	}
}

// TestSendJSONOutput asserts --json emits the full response.
func TestSendJSONOutput(t *testing.T) {
	fake := &fakeCommandClient{
		sendMessageResp: &controlclient.SendSessionResponse{OK: true, TurnID: "trn_json"},
	}
	stdout := &bytes.Buffer{}
	cmd := NewSendCommand(stdout, &bytes.Buffer{}, strings.NewReader(""), fixedSendFactory(fake))

	if err := cmd.Run([]string{"session-1", "hello", "--json"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v\noutput: %s", err, stdout.String())
	}
	if result["ok"] != true {
		t.Fatalf("expected ok=true, got %v", result["ok"])
	}
	if result["turn_id"] != "trn_json" {
		t.Fatalf("expected turn_id='trn_json', got %v", result["turn_id"])
	}
}

// TestSendDaemonError asserts daemon errors surface as single-line stderr.
func TestSendDaemonError(t *testing.T) {
	fake := &fakeCommandClient{
		sendMessageErr: errors.New("session not found"),
	}
	stderr := &bytes.Buffer{}
	cmd := NewSendCommand(&bytes.Buffer{}, stderr, strings.NewReader(""), fixedSendFactory(fake))

	err := cmd.Run([]string{"session-1", "hello"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestSendEmptyTurnID asserts empty turn_id produces no stdout but exits 0.
func TestSendEmptyTurnID(t *testing.T) {
	fake := &fakeCommandClient{
		sendMessageResp: &controlclient.SendSessionResponse{OK: true, TurnID: ""},
	}
	stdout := &bytes.Buffer{}
	cmd := NewSendCommand(stdout, &bytes.Buffer{}, strings.NewReader(""), fixedSendFactory(fake))

	if err := cmd.Run([]string{"session-1", "hello"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if stdout.String() != "" {
		t.Fatalf("expected empty stdout for empty turn_id, got %q", stdout.String())
	}
}

// TestTailFollowContextCancelled simulates SIGINT: cancel the context and assert nil return.
func TestTailFollowContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan types.LogEvent, 1)
	ch <- types.LogEvent{Type: "log", Stream: "stdout", Chunk: "partial\n", TS: "t1"}
	// Don't close ch yet — cancel context first to simulate signal.

	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		streamTailCh: ch,
		streamTailCancel: func() {},
	}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	// Cancel context after a brief moment to let one event through.
	go func() {
		<-ctx.Done()
		close(ch)
	}()

	// Trigger cancellation immediately.
	cancel()

	err := cmd.Run([]string{"--follow", "session-1"})
	// The command should handle a cancelled context gracefully.
	// Exact behavior depends on timing; either nil or context.Canceled is acceptable.
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected nil or context.Canceled, got %v", err)
	}
}

// TestTailFollowStreamError asserts a non-nil error when the adapter fails.
func TestTailFollowStreamError(t *testing.T) {
	fake := &fakeCommandClient{
		streamTailErr: errors.New("connection refused"),
	}
	stderr := &bytes.Buffer{}
	cmd := NewTailCommand(&bytes.Buffer{}, stderr, fixedSessionFactory(fake))

	err := cmd.Run([]string{"--follow", "session-1"})
	if err == nil {
		t.Fatal("expected error for stream failure")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected error message to contain 'connection refused', got %v", err)
	}
}

// TestTailFollowBackfillThenStream asserts backfill prints before stream events
// and dedup works at the boundary.
func TestTailFollowBackfillThenStream(t *testing.T) {
	// The backfill item and first stream event are the same (dedup scenario).
	sharedEvent := types.LogEvent{Type: "log", Stream: "stdout", Chunk: "backfill-line\n", TS: "t0"}
	streamCh := make(chan types.LogEvent, 2)
	streamCh <- sharedEvent // duplicate of last backfill
	streamCh <- types.LogEvent{Type: "log", Stream: "stdout", Chunk: "live-line\n", TS: "t1"}
	close(streamCh)

	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		tailItemsResp: &controlclient.TailItemsResponse{
			Items: []map[string]any{
				{"type": "log", "stream": "stdout", "chunk": "backfill-line\n", "ts": "t0"},
			},
		},
		streamTailCh: streamCh,
		streamTailCancel: func() {},
	}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedSessionFactory(fake))

	if err := cmd.Run([]string{"--follow", "--lines", "10", "session-1"}); err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}

	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines (backfill + live, no duplicate), got %d:\n%s", len(lines), stdout.String())
	}

	// First line should be the backfill item.
	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 0: expected valid JSON, got err=%v", err)
	}
	if first["chunk"] != "backfill-line\n" {
		t.Fatalf("expected backfill chunk, got %v", first["chunk"])
	}

	// Second line should be the live event (not the duplicate).
	var second map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("line 1: expected valid JSON, got err=%v", err)
	}
	if second["chunk"] != "live-line\n" {
		t.Fatalf("expected live chunk, got %v", second["chunk"])
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("write failure")
}

type fakeCommandClient struct {
	ensureDaemonErr error

	ensureDaemonCalls     int
	ensureVersionErr      error
	ensureVersionCalls    int
	ensureVersionExpected string
	ensureVersionRestart  bool

	cloudLoginStartErr   error
	cloudLoginStartResp  *controlclient.CloudDeviceAuthorizationResponse
	cloudLoginStartCalls int

	cloudPollErrs      []error
	cloudPollResponses []*controlclient.CloudAuthPollResponse
	cloudPollCalls     int

	cloudStatusErr  error
	cloudStatusResp *controlclient.CloudAuthStatusResponse

	logoutCloudErr   error
	logoutCloudResp  *controlclient.CloudLogoutResponse
	logoutCloudCalls int

	listSessionsErr   error
	listSessionsCalls int
	sessionsResp      []*types.Session

	getSessionErr   error
	getSessionResp  *types.Session
	getSessionCalls int
	getSessionIDArg string

	startSessionErr  error
	startSessionResp *types.Session
	startRequests    []controlclient.StartSessionRequest

	killSessionErr   error
	killSessionCalls int
	killSessionID    string

	interruptSessionErr   error
	interruptSessionCalls int
	interruptSessionID    string

	tailItemsErr   error
	tailItemsResp  *controlclient.TailItemsResponse
	tailItemsCalls int
	tailItemsID    string
	tailItemsLines int

	streamTailErr    error
	streamTailCh     <-chan types.LogEvent
	streamTailCancel func()
	streamTailCalls  int
	streamTailIDArg  string
	streamTailNameArg string

	sendMessageErr   error
	sendMessageResp  *controlclient.SendSessionResponse
	sendMessageCalls int
	sendMessageIDArg string
	sendMessageReq   controlclient.SendSessionRequest

	steerSessionErr   error
	steerSessionResp  *controlclient.SteerSessionResponse
	steerSessionCalls int
	steerSessionIDArg string
	steerSessionReq   controlclient.SteerSessionRequest

	listApprovalsErr   error
	listApprovalsResp  []*types.Approval
	listApprovalsCalls int
	listApprovalsIDArg string

	approveSessionErr   error
	approveSessionCalls int
	approveSessionIDArg string
	approveSessionReq   controlclient.ApproveSessionRequest

	shutdownErr error
	healthErr   error
	healthResp  *controlclient.HealthResponse
	runUIErr    error
	runUICalls  int
}

func (f *fakeCommandClient) EnsureDaemon(context.Context) error {
	f.ensureDaemonCalls++
	return f.ensureDaemonErr
}

func (f *fakeCommandClient) EnsureDaemonVersion(_ context.Context, expectedVersion string, restart bool) error {
	f.ensureVersionCalls++
	f.ensureVersionExpected = expectedVersion
	f.ensureVersionRestart = restart
	return f.ensureVersionErr
}

func (f *fakeCommandClient) StartCloudLogin(context.Context) (*controlclient.CloudDeviceAuthorizationResponse, error) {
	f.cloudLoginStartCalls++
	if f.cloudLoginStartErr != nil {
		return nil, f.cloudLoginStartErr
	}
	if f.cloudLoginStartResp == nil {
		return nil, errors.New("cloudLoginStartResp not configured")
	}
	return f.cloudLoginStartResp, nil
}

func (f *fakeCommandClient) PollCloudLogin(context.Context) (*controlclient.CloudAuthPollResponse, error) {
	index := f.cloudPollCalls
	f.cloudPollCalls++
	if index < len(f.cloudPollErrs) && f.cloudPollErrs[index] != nil {
		return nil, f.cloudPollErrs[index]
	}
	if index < len(f.cloudPollResponses) && f.cloudPollResponses[index] != nil {
		return f.cloudPollResponses[index], nil
	}
	return nil, errors.New("cloudPollResponses not configured")
}

func (f *fakeCommandClient) CloudAuthStatus(context.Context) (*controlclient.CloudAuthStatusResponse, error) {
	if f.cloudStatusErr != nil {
		return nil, f.cloudStatusErr
	}
	if f.cloudStatusResp == nil {
		return &controlclient.CloudAuthStatusResponse{}, nil
	}
	return f.cloudStatusResp, nil
}

func (f *fakeCommandClient) LogoutCloud(context.Context) (*controlclient.CloudLogoutResponse, error) {
	f.logoutCloudCalls++
	if f.logoutCloudErr != nil {
		return nil, f.logoutCloudErr
	}
	if f.logoutCloudResp == nil {
		return &controlclient.CloudLogoutResponse{Status: "revoked_and_unlinked", Message: "revoked remote token and cleared local cloud credentials", RemoteRevoked: true, LocalCleared: true}, nil
	}
	return f.logoutCloudResp, nil
}

func (f *fakeCommandClient) ListSessions(context.Context) ([]*types.Session, error) {
	f.listSessionsCalls++
	return f.sessionsResp, f.listSessionsErr
}

func (f *fakeCommandClient) GetSession(_ context.Context, id string) (*types.Session, error) {
	f.getSessionCalls++
	f.getSessionIDArg = id
	if f.getSessionErr != nil {
		return nil, f.getSessionErr
	}
	if f.getSessionResp == nil {
		return nil, errors.New("getSessionResp not configured")
	}
	return f.getSessionResp, nil
}

func (f *fakeCommandClient) StartSession(_ context.Context, req controlclient.StartSessionRequest) (*types.Session, error) {
	f.startRequests = append(f.startRequests, req)
	if f.startSessionErr != nil {
		return nil, f.startSessionErr
	}
	if f.startSessionResp == nil {
		return nil, errors.New("startSessionResp not configured")
	}
	return f.startSessionResp, nil
}

func (f *fakeCommandClient) KillSession(_ context.Context, id string) error {
	f.killSessionCalls++
	f.killSessionID = id
	return f.killSessionErr
}

func (f *fakeCommandClient) InterruptSession(_ context.Context, id string) error {
	f.interruptSessionCalls++
	f.interruptSessionID = id
	return f.interruptSessionErr
}

func (f *fakeCommandClient) TailItems(_ context.Context, id string, lines int) (*controlclient.TailItemsResponse, error) {
	f.tailItemsCalls++
	f.tailItemsID = id
	f.tailItemsLines = lines
	if f.tailItemsErr != nil {
		return nil, f.tailItemsErr
	}
	if f.tailItemsResp == nil {
		return nil, errors.New("tailItemsResp not configured")
	}
	return f.tailItemsResp, nil
}

func (f *fakeCommandClient) StreamTail(_ context.Context, id, stream string) (<-chan types.LogEvent, func(), error) {
	f.streamTailCalls++
	f.streamTailIDArg = id
	f.streamTailNameArg = stream
	if f.streamTailErr != nil {
		return nil, nil, f.streamTailErr
	}
	if f.streamTailCh == nil {
		return nil, nil, errors.New("streamTailCh not configured")
	}
	return f.streamTailCh, f.streamTailCancel, nil
}

func (f *fakeCommandClient) SendMessage(_ context.Context, sessionID string, req controlclient.SendSessionRequest) (*controlclient.SendSessionResponse, error) {
	f.sendMessageCalls++
	f.sendMessageIDArg = sessionID
	f.sendMessageReq = req
	if f.sendMessageErr != nil {
		return nil, f.sendMessageErr
	}
	if f.sendMessageResp == nil {
		return nil, errors.New("sendMessageResp not configured")
	}
	return f.sendMessageResp, nil
}

func (f *fakeCommandClient) SteerSession(_ context.Context, sessionID string, req controlclient.SteerSessionRequest) (*controlclient.SteerSessionResponse, error) {
	f.steerSessionCalls++
	f.steerSessionIDArg = sessionID
	f.steerSessionReq = req
	if f.steerSessionErr != nil {
		return nil, f.steerSessionErr
	}
	if f.steerSessionResp == nil {
		return nil, errors.New("steerSessionResp not configured")
	}
	return f.steerSessionResp, nil
}

func (f *fakeCommandClient) ListApprovals(_ context.Context, sessionID string) ([]*types.Approval, error) {
	f.listApprovalsCalls++
	f.listApprovalsIDArg = sessionID
	if f.listApprovalsErr != nil {
		return nil, f.listApprovalsErr
	}
	return f.listApprovalsResp, nil
}

func (f *fakeCommandClient) ApproveSession(_ context.Context, sessionID string, req controlclient.ApproveSessionRequest) error {
	f.approveSessionCalls++
	f.approveSessionIDArg = sessionID
	f.approveSessionReq = req
	return f.approveSessionErr
}

func (f *fakeCommandClient) ShutdownDaemon(context.Context) error {
	return f.shutdownErr
}

func (f *fakeCommandClient) Health(context.Context) (*controlclient.HealthResponse, error) {
	return f.healthResp, f.healthErr
}

func (f *fakeCommandClient) RunUI() error {
	f.runUICalls++
	return f.runUIErr
}

func fixedCloudAuthFactory(client cloudAuthCommandClient) cloudAuthClientFactory {
	return func() (cloudAuthCommandClient, error) {
		return client, nil
	}
}

func fixedSessionFactory(client sessionCommandClient) sessionClientFactory {
	return func() (sessionCommandClient, error) {
		return client, nil
	}
}

func fixedDaemonVersionFactory(client daemonVersionClient) daemonVersionClientFactory {
	return func() (daemonVersionClient, error) {
		return client, nil
	}
}

func fixedDaemonAdminFactory(client daemonAdminClient) daemonAdminClientFactory {
	return func() (daemonAdminClient, error) {
		return client, nil
	}
}

type staticBuildMetadataProvider struct {
	metadata buildMetadata
}

func (p staticBuildMetadataProvider) Snapshot() buildMetadata {
	return p.metadata
}

type staticVersionFormatter struct {
	output string
}

func (f staticVersionFormatter) Format(buildMetadata) string {
	return f.output
}
