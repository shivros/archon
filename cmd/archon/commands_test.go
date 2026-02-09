package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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

func TestStartCommandWritesSessionID(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		startSessionResp: &types.Session{ID: "session-123"},
	}
	cmd := NewStartCommand(stdout, &bytes.Buffer{}, fixedFactory(fake))

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
	if req.Provider != "codex" || req.Cwd != "/tmp/project" || req.Cmd != "codex" {
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
	cmd := NewPSCommand(stdout, &bytes.Buffer{}, fixedFactory(fake))

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

func TestTailCommandWritesItemsJSON(t *testing.T) {
	stdout := &bytes.Buffer{}
	fake := &fakeCommandClient{
		tailItemsResp: &controlclient.TailItemsResponse{
			Items: []map[string]any{{"type": "log", "text": "hello"}},
		},
	}
	cmd := NewTailCommand(stdout, &bytes.Buffer{}, fixedFactory(fake))

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
		fixedFactory(fake),
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
	if fake.ensureVersionExpected != "v-test" || !fake.ensureVersionRestart {
		t.Fatalf("unexpected ensure version args: expected=%q restart=%v", fake.ensureVersionExpected, fake.ensureVersionRestart)
	}
	if fake.runUICalls != 1 {
		t.Fatalf("expected ui runner once, got %d", fake.runUICalls)
	}
}

func TestStartCommandRequiresProvider(t *testing.T) {
	cmd := NewStartCommand(&bytes.Buffer{}, &bytes.Buffer{}, fixedFactory(&fakeCommandClient{}))
	err := cmd.Run(nil)
	if err == nil || !strings.Contains(err.Error(), "provider is required") {
		t.Fatalf("expected provider validation error, got %v", err)
	}
}

type fakeCommandClient struct {
	ensureDaemonErr error

	ensureDaemonCalls     int
	ensureVersionErr      error
	ensureVersionCalls    int
	ensureVersionExpected string
	ensureVersionRestart  bool

	listSessionsErr   error
	listSessionsCalls int
	sessionsResp      []*types.Session

	startSessionErr  error
	startSessionResp *types.Session
	startRequests    []controlclient.StartSessionRequest

	killSessionErr   error
	killSessionCalls int

	tailItemsErr   error
	tailItemsResp  *controlclient.TailItemsResponse
	tailItemsCalls int
	tailItemsID    string
	tailItemsLines int

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

func (f *fakeCommandClient) ListSessions(context.Context) ([]*types.Session, error) {
	f.listSessionsCalls++
	return f.sessionsResp, f.listSessionsErr
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

func (f *fakeCommandClient) KillSession(context.Context, string) error {
	f.killSessionCalls++
	return f.killSessionErr
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

func fixedFactory(client commandClient) clientFactory {
	return func() (commandClient, error) {
		return client, nil
	}
}
