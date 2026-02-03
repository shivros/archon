package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

func TestSessionManagerStartAndTail(t *testing.T) {
	manager := newTestManager(t)

	cfg := StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=hello", "stderr=oops", "stdout_lines=2", "stderr_lines=1", "sleep_ms=50", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
		Title:    "demo",
		Tags:     []string{"test"},
	}

	session, err := manager.StartSession(cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected session id")
	}
	if session.Status != types.SessionStatusRunning && session.Status != types.SessionStatusExited {
		t.Fatalf("unexpected status: %s", session.Status)
	}

	waitForStatus(t, manager, session.ID, types.SessionStatusExited, 2*time.Second)

	lines, truncated, order, err := manager.TailSession(session.ID, "combined", 10)
	if err != nil {
		t.Fatalf("TailSession: %v", err)
	}
	if truncated {
		t.Fatalf("expected not truncated")
	}
	if order != "stdout_then_stderr" {
		t.Fatalf("unexpected order: %s", order)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "hello") {
		t.Fatalf("expected stdout in tail")
	}
	if !strings.Contains(joined, "oops") {
		t.Fatalf("expected stderr in tail")
	}

	sessionDir := filepath.Join(manager.baseDir, session.ID)
	stdoutData, err := os.ReadFile(filepath.Join(sessionDir, "stdout.log"))
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	stderrData, err := os.ReadFile(filepath.Join(sessionDir, "stderr.log"))
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if !strings.Contains(string(stdoutData), "hello") {
		t.Fatalf("stdout log missing output")
	}
	if !strings.Contains(string(stderrData), "oops") {
		t.Fatalf("stderr log missing output")
	}
}

func TestSessionManagerKill(t *testing.T) {
	manager := newTestManager(t)

	cfg := StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=sleeping", "sleep_ms=2000", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
	}

	session, err := manager.StartSession(cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if err := manager.KillSession(session.ID); err != nil {
		t.Fatalf("KillSession: %v", err)
	}

	waitForStatus(t, manager, session.ID, types.SessionStatusKilled, 3*time.Second)
}

func TestTailInvalidStream(t *testing.T) {
	manager := newTestManager(t)

	cfg := StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=ok", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
	}

	session, err := manager.StartSession(cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	waitForStatus(t, manager, session.ID, types.SessionStatusExited, 2*time.Second)

	_, _, _, err = manager.TailSession(session.ID, "weird", 10)
	if err == nil {
		t.Fatalf("expected error for invalid stream")
	}
}

func TestResolveProviderCustomPath(t *testing.T) {
	provider, err := ResolveProvider("custom", os.Args[0])
	if err != nil {
		t.Fatalf("ResolveProvider: %v", err)
	}
	if provider.Command() == "" {
		t.Fatalf("expected command")
	}
}

func TestResolveProviderUnknown(t *testing.T) {
	_, err := ResolveProvider("nope", "")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func newTestManager(t *testing.T) *SessionManager {
	t.Helper()
	baseDir := t.TempDir()
	manager, err := NewSessionManager(baseDir)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	return manager
}

func helperArgs(args ...string) []string {
	return append([]string{"-test.run=TestHelperProcess", "--"}, args...)
}

func waitForStatus(t *testing.T, manager *SessionManager, id string, status types.SessionStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session, ok := manager.GetSession(id)
		if ok && session.Status == status {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	session, _ := manager.GetSession(id)
	if session == nil {
		t.Fatalf("session not found while waiting for status")
	}
	t.Fatalf("timeout waiting for status %s; got %s", status, session.Status)
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep >= 0 {
		args = args[sep+1:]
	} else {
		args = []string{}
	}

	stdoutText := ""
	stderrText := ""
	stdoutLines := 0
	stderrLines := 0
	var sleepMs int
	exitCode := 0

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "stdout="):
			stdoutText = strings.TrimPrefix(arg, "stdout=")
			if stdoutLines == 0 {
				stdoutLines = 1
			}
		case strings.HasPrefix(arg, "stderr="):
			stderrText = strings.TrimPrefix(arg, "stderr=")
			if stderrLines == 0 {
				stderrLines = 1
			}
		case strings.HasPrefix(arg, "stdout_lines="):
			fmt.Sscanf(strings.TrimPrefix(arg, "stdout_lines="), "%d", &stdoutLines)
		case strings.HasPrefix(arg, "stderr_lines="):
			fmt.Sscanf(strings.TrimPrefix(arg, "stderr_lines="), "%d", &stderrLines)
		case strings.HasPrefix(arg, "sleep_ms="):
			fmt.Sscanf(strings.TrimPrefix(arg, "sleep_ms="), "%d", &sleepMs)
		case strings.HasPrefix(arg, "exit="):
			fmt.Sscanf(strings.TrimPrefix(arg, "exit="), "%d", &exitCode)
		}
	}

	for i := 0; i < stdoutLines; i++ {
		if stdoutText != "" {
			fmt.Fprintln(os.Stdout, stdoutText)
		}
	}
	for i := 0; i < stderrLines; i++ {
		if stderrText != "" {
			fmt.Fprintln(os.Stderr, stderrText)
		}
	}
	if sleepMs > 0 {
		time.Sleep(time.Duration(sleepMs) * time.Millisecond)
	}
	os.Exit(exitCode)
}
