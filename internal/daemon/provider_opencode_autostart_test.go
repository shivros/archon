package daemon

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMaybeAutoStartOpenCodeServerLaunchesLocalServer(t *testing.T) {
	t.Cleanup(resetOpenCodeAutoStartStateForTest)
	home := filepath.Join(t.TempDir(), "home")
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := []byte("[providers.opencode]\ncommand = \"" + os.Args[0] + "\"\n")
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), content, 0o600); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
	t.Setenv("HOME", home)

	origStart := startOpenCodeServeProcess
	origProbe := probeOpenCodeServer
	origWait := waitForOpenCodeServerReady
	origFallback := pickOpenCodeFallbackPortFn
	origFindPID := findOpenCodeServerPID
	origReadCmdline := readOpenCodeProcessCmdline
	origTerminate := terminateOpenCodeProcess
	t.Cleanup(func() {
		startOpenCodeServeProcess = origStart
		probeOpenCodeServer = origProbe
		waitForOpenCodeServerReady = origWait
		pickOpenCodeFallbackPortFn = origFallback
		findOpenCodeServerPID = origFindPID
		readOpenCodeProcessCmdline = origReadCmdline
		terminateOpenCodeProcess = origTerminate
	})

	called := false
	var (
		gotCmd  string
		gotArgs []string
		gotEnv  []string
	)
	startOpenCodeServeProcess = func(cmdName string, args []string, env []string, _ ProviderBaseSink) error {
		called = true
		gotCmd = cmdName
		gotArgs = append([]string{}, args...)
		gotEnv = append([]string{}, env...)
		return nil
	}
	probeOpenCodeServer = func(string) error { return errors.New("unreachable") }
	waitForOpenCodeServerReady = func(string, time.Duration) error { return nil }
	pickOpenCodeFallbackPortFn = func(string) (string, error) { return "49999", nil }
	findOpenCodeServerPID = func(string, string) (int, error) { return 0, nil }
	readOpenCodeProcessCmdline = func(int) (string, error) { return "", nil }
	terminateOpenCodeProcess = func(int, string, string) error { return nil }

	resetOpenCodeAutoStartStateForTest()
	startedURL, err := maybeAutoStartOpenCodeServer("opencode", "http://127.0.0.1:49123", "token-123", &testProviderLogSink{})
	if err != nil {
		t.Fatalf("maybeAutoStartOpenCodeServer: %v", err)
	}
	if startedURL != "http://127.0.0.1:49123" {
		t.Fatalf("unexpected started url: %q", startedURL)
	}
	if !called {
		t.Fatalf("expected startOpenCodeServeProcess to be invoked")
	}
	if gotCmd != os.Args[0] {
		t.Fatalf("unexpected command: %q", gotCmd)
	}
	if strings.Join(gotArgs, " ") != "serve --hostname 127.0.0.1 --port 49123" {
		t.Fatalf("unexpected args: %#v", gotArgs)
	}
	foundPassword := false
	for _, entry := range gotEnv {
		if entry == "OPENCODE_SERVER_PASSWORD=token-123" {
			foundPassword = true
			break
		}
	}
	if !foundPassword {
		t.Fatalf("expected OPENCODE_SERVER_PASSWORD in env")
	}
}

func TestMaybeAutoStartOpenCodeServerRejectsRemoteHosts(t *testing.T) {
	t.Cleanup(resetOpenCodeAutoStartStateForTest)
	resetOpenCodeAutoStartStateForTest()
	_, err := maybeAutoStartOpenCodeServer("opencode", "https://example.com:443", "", nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "localhost") {
		t.Fatalf("expected localhost guard error, got %v", err)
	}
}

func TestMaybeAutoStartOpenCodeServerFallsBackToAlternatePort(t *testing.T) {
	t.Cleanup(resetOpenCodeAutoStartStateForTest)
	origStart := startOpenCodeServeProcess
	origProbe := probeOpenCodeServer
	origWait := waitForOpenCodeServerReady
	origFallback := pickOpenCodeFallbackPortFn
	origFindPID := findOpenCodeServerPID
	origReadCmdline := readOpenCodeProcessCmdline
	origTerminate := terminateOpenCodeProcess
	t.Cleanup(func() {
		startOpenCodeServeProcess = origStart
		probeOpenCodeServer = origProbe
		waitForOpenCodeServerReady = origWait
		pickOpenCodeFallbackPortFn = origFallback
		findOpenCodeServerPID = origFindPID
		readOpenCodeProcessCmdline = origReadCmdline
		terminateOpenCodeProcess = origTerminate
	})

	startArgs := make([][]string, 0, 2)
	startOpenCodeServeProcess = func(_ string, args []string, _ []string, _ ProviderBaseSink) error {
		startArgs = append(startArgs, append([]string{}, args...))
		return nil
	}
	probeOpenCodeServer = func(string) error { return errors.New("unreachable") }
	waitForOpenCodeServerReady = func(baseURL string, _ time.Duration) error {
		if strings.HasSuffix(baseURL, ":49123") {
			return errors.New("timed out")
		}
		return nil
	}
	pickOpenCodeFallbackPortFn = func(string) (string, error) { return "49124", nil }
	findOpenCodeServerPID = func(string, string) (int, error) { return 0, nil }
	readOpenCodeProcessCmdline = func(int) (string, error) { return "", nil }
	terminateOpenCodeProcess = func(int, string, string) error { return nil }

	resetOpenCodeAutoStartStateForTest()
	startedURL, err := maybeAutoStartOpenCodeServer("opencode", "http://127.0.0.1:49123", "", nil)
	if err != nil {
		t.Fatalf("maybeAutoStartOpenCodeServer fallback: %v", err)
	}
	if startedURL != "http://127.0.0.1:49124" {
		t.Fatalf("unexpected fallback url: %q", startedURL)
	}
	if len(startArgs) != 2 {
		t.Fatalf("expected two launch attempts, got %d", len(startArgs))
	}
	if strings.Join(startArgs[0], " ") != "serve --hostname 127.0.0.1 --port 49123" {
		t.Fatalf("unexpected primary launch args: %#v", startArgs[0])
	}
	if strings.Join(startArgs[1], " ") != "serve --hostname 127.0.0.1 --port 49124" {
		t.Fatalf("unexpected fallback launch args: %#v", startArgs[1])
	}
	if got := resolveOpenCodeRuntimeBaseURL("opencode", "http://127.0.0.1:49123"); got != "http://127.0.0.1:49124" {
		t.Fatalf("expected runtime base url override, got %q", got)
	}
}

func TestMaybeAutoStartOpenCodeServerCleansMatchedStaleProcess(t *testing.T) {
	t.Cleanup(resetOpenCodeAutoStartStateForTest)
	origStart := startOpenCodeServeProcess
	origProbe := probeOpenCodeServer
	origWait := waitForOpenCodeServerReady
	origFindPID := findOpenCodeServerPID
	origReadCmdline := readOpenCodeProcessCmdline
	origTerminate := terminateOpenCodeProcess
	t.Cleanup(func() {
		startOpenCodeServeProcess = origStart
		probeOpenCodeServer = origProbe
		waitForOpenCodeServerReady = origWait
		findOpenCodeServerPID = origFindPID
		readOpenCodeProcessCmdline = origReadCmdline
		terminateOpenCodeProcess = origTerminate
	})

	probeCalls := 0
	probeOpenCodeServer = func(string) error {
		probeCalls++
		if probeCalls >= 2 {
			return nil
		}
		return errors.New("timeout")
	}
	startCalled := false
	startOpenCodeServeProcess = func(string, []string, []string, ProviderBaseSink) error {
		startCalled = true
		return nil
	}
	waitForOpenCodeServerReady = func(string, time.Duration) error { return nil }
	findOpenCodeServerPID = func(string, string) (int, error) { return 424242, nil }
	readOpenCodeProcessCmdline = func(pid int) (string, error) {
		if pid != 424242 {
			t.Fatalf("unexpected pid: %d", pid)
		}
		return "opencode serve --hostname 127.0.0.1 --port 49123", nil
	}
	terminated := false
	terminateOpenCodeProcess = func(pid int, host, port string) error {
		if pid != 424242 || host != "127.0.0.1" || port != "49123" {
			t.Fatalf("unexpected terminate args: pid=%d host=%s port=%s", pid, host, port)
		}
		terminated = true
		return nil
	}

	resetOpenCodeAutoStartStateForTest()
	startedURL, err := maybeAutoStartOpenCodeServer("opencode", "http://127.0.0.1:49123", "", nil)
	if err != nil {
		t.Fatalf("maybeAutoStartOpenCodeServer: %v", err)
	}
	if startedURL != "http://127.0.0.1:49123" {
		t.Fatalf("unexpected started url: %q", startedURL)
	}
	if !terminated {
		t.Fatalf("expected stale process termination")
	}
	if startCalled {
		t.Fatalf("did not expect launcher call when stale cleanup restored health")
	}
}

func TestMaybeAutoStartOpenCodeServerDoesNotTerminateUnmatchedProcess(t *testing.T) {
	t.Cleanup(resetOpenCodeAutoStartStateForTest)
	origStart := startOpenCodeServeProcess
	origProbe := probeOpenCodeServer
	origWait := waitForOpenCodeServerReady
	origFindPID := findOpenCodeServerPID
	origReadCmdline := readOpenCodeProcessCmdline
	origTerminate := terminateOpenCodeProcess
	t.Cleanup(func() {
		startOpenCodeServeProcess = origStart
		probeOpenCodeServer = origProbe
		waitForOpenCodeServerReady = origWait
		findOpenCodeServerPID = origFindPID
		readOpenCodeProcessCmdline = origReadCmdline
		terminateOpenCodeProcess = origTerminate
	})

	probeOpenCodeServer = func(string) error { return errors.New("timeout") }
	startCalled := false
	startOpenCodeServeProcess = func(string, []string, []string, ProviderBaseSink) error {
		startCalled = true
		return nil
	}
	waitForOpenCodeServerReady = func(string, time.Duration) error { return nil }
	findOpenCodeServerPID = func(string, string) (int, error) { return 424242, nil }
	readOpenCodeProcessCmdline = func(int) (string, error) { return "python -m http.server 49123", nil }
	terminated := false
	terminateOpenCodeProcess = func(int, string, string) error {
		terminated = true
		return nil
	}

	resetOpenCodeAutoStartStateForTest()
	startedURL, err := maybeAutoStartOpenCodeServer("opencode", "http://127.0.0.1:49123", "", nil)
	if err != nil {
		t.Fatalf("maybeAutoStartOpenCodeServer: %v", err)
	}
	if startedURL != "http://127.0.0.1:49123" {
		t.Fatalf("unexpected started url: %q", startedURL)
	}
	if terminated {
		t.Fatalf("did not expect termination for unmatched process")
	}
	if !startCalled {
		t.Fatalf("expected launcher call when cleanup is skipped")
	}
}

func TestMatchesOpenCodeServeProcess(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		cmdline  string
		cmdName  string
		want     bool
	}{
		{
			name:     "opencode serve matches",
			provider: "opencode",
			cmdline:  "/usr/bin/opencode serve --hostname 127.0.0.1 --port 4096",
			cmdName:  "/usr/bin/opencode",
			want:     true,
		},
		{
			name:     "kilocode serve matches kilo alias",
			provider: "kilocode",
			cmdline:  "/home/shiv/.local/share/pnpm/kilo serve --hostname 127.0.0.1 --port 4097",
			cmdName:  "kilo",
			want:     true,
		},
		{
			name:     "missing serve rejected",
			provider: "opencode",
			cmdline:  "/usr/bin/opencode run",
			cmdName:  "/usr/bin/opencode",
			want:     false,
		},
		{
			name:     "mismatched command rejected",
			provider: "opencode",
			cmdline:  "/usr/bin/opencode serve",
			cmdName:  "/usr/local/bin/custom-opencode-wrapper",
			want:     false,
		},
		{
			name:     "wrong provider token rejected",
			provider: "opencode",
			cmdline:  "/usr/bin/kilo serve --hostname 127.0.0.1 --port 4097",
			cmdName:  "kilo",
			want:     false,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := matchesOpenCodeServeProcess(tc.provider, tc.cmdline, tc.cmdName)
			if got != tc.want {
				t.Fatalf("matchesOpenCodeServeProcess()=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestIsOpenCodeUnreachable(t *testing.T) {
	if !isOpenCodeUnreachable(&url.Error{Err: errors.New("dial tcp 127.0.0.1:4096: connect: connection refused")}) {
		t.Fatalf("expected url.Error to be considered unreachable")
	}
	if !isOpenCodeUnreachable(&openCodeRequestError{StatusCode: 503}) {
		t.Fatalf("expected 503 to be considered unreachable")
	}
	if isOpenCodeUnreachable(&openCodeRequestError{StatusCode: 400}) {
		t.Fatalf("did not expect 400 to be considered unreachable")
	}
}

func resetOpenCodeAutoStartStateForTest() {
	openCodeAutoStartState.mu.Lock()
	defer openCodeAutoStartState.mu.Unlock()
	openCodeAutoStartState.lastAttempt = map[string]time.Time{}
	openCodeAutoStartState.lastCleanup = map[string]time.Time{}
	openCodeAutoStartState.runtimeURL = map[string]string{}
}
