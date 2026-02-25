package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var openCodeAutoStartState = struct {
	mu          sync.Mutex
	lastAttempt map[string]time.Time
	lastCleanup map[string]time.Time
	runtimeURL  map[string]string
}{
	lastAttempt: map[string]time.Time{},
	lastCleanup: map[string]time.Time{},
	runtimeURL:  map[string]string{},
}

var startOpenCodeServeProcess = func(cmdName string, args []string, env []string, sink ProviderBaseSink) error {
	cmd := exec.Command(cmdName, args...)
	cmd.Env = env
	if sink != nil {
		cmd.Stdout = sink.StdoutWriter()
		cmd.Stderr = sink.StderrWriter()
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}

var probeOpenCodeServer = probeOpenCodeServerImpl
var waitForOpenCodeServerReady = waitForOpenCodeServerReadyImpl
var pickOpenCodeFallbackPortFn = pickOpenCodeFallbackPortImpl
var findOpenCodeServerPID = findOpenCodeServerPIDImpl
var readOpenCodeProcessCmdline = readOpenCodeProcessCmdlineImpl
var waitForOpenCodePortClosed = waitForOpenCodePortClosedImpl
var terminateOpenCodeProcess = terminateOpenCodeProcessImpl

func maybeAutoStartOpenCodeServer(provider, baseURL, token string, sink ProviderBaseSink) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", errors.New("base_url is required for auto-start")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
	}
	host := strings.TrimSpace(parsed.Hostname())
	if !isLocalOpenCodeHost(host) {
		return "", fmt.Errorf("auto-start only supports localhost base_url (got %q)", host)
	}
	port := parsed.Port()
	if strings.TrimSpace(port) == "" {
		switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("invalid base_url port %q", port)
	}
	baseURL = openCodeServerBaseURL(parsed.Scheme, host, port)
	if probeErr := probeOpenCodeServer(baseURL); probeErr == nil {
		rememberOpenCodeRuntimeBaseURL(provider, baseURL)
		return baseURL, nil
	}

	cmdName, err := resolveOpenCodeServeCommand(provider)
	if err != nil {
		return "", err
	}
	if cleaned, cleanupErr := maybeCleanupStaleOpenCodeServer(provider, cmdName, host, port, baseURL, sink); cleanupErr != nil {
		if sink != nil {
			sink.Write("stderr", []byte("opencode stale cleanup skipped: "+cleanupErr.Error()+"\n"))
		}
	} else if cleaned {
		if probeErr := probeOpenCodeServer(baseURL); probeErr == nil {
			rememberOpenCodeRuntimeBaseURL(provider, baseURL)
			return baseURL, nil
		}
	}

	if !allowOpenCodeAutoStart(baseURL) {
		return baseURL, nil
	}

	args := []string{"serve", "--hostname", host, "--port", port}
	env := withOpenCodeServerPassword(os.Environ(), provider, token)
	launchErr := launchOpenCodeServer(cmdName, args, env, baseURL, 6*time.Second, sink)
	if launchErr == nil {
		rememberOpenCodeRuntimeBaseURL(provider, baseURL)
		return baseURL, nil
	}
	if sink != nil {
		sink.Write("stderr", []byte("opencode auto-start: primary launch unavailable ("+launchErr.Error()+"), trying fallback\n"))
	}

	fallbackPort, err := pickOpenCodeFallbackPortFn(host)
	if err != nil {
		return "", launchErr
	}
	if fallbackPort == port {
		return "", launchErr
	}
	fallbackURL := openCodeServerBaseURL(parsed.Scheme, host, fallbackPort)
	fallbackArgs := []string{"serve", "--hostname", host, "--port", fallbackPort}
	if err := launchOpenCodeServer(cmdName, fallbackArgs, env, fallbackURL, 8*time.Second, sink); err != nil {
		return "", fmt.Errorf("auto-start failed on fallback %s: %w", fallbackURL, err)
	}
	rememberOpenCodeRuntimeBaseURL(provider, fallbackURL)
	return fallbackURL, nil
}

func launchOpenCodeServer(cmdName string, args []string, env []string, baseURL string, waitTimeout time.Duration, sink ProviderBaseSink) error {
	if sink != nil {
		sink.Write("stderr", []byte("opencode auto-start: launching "+cmdName+" "+strings.Join(args, " ")+"\n"))
	}
	if err := startOpenCodeServeProcess(cmdName, args, env, sink); err != nil {
		return err
	}
	return waitForOpenCodeServerReady(baseURL, waitTimeout)
}

func maybeCleanupStaleOpenCodeServer(provider, cmdName, host, port, baseURL string, sink ProviderBaseSink) (bool, error) {
	if !allowOpenCodeCleanupAttempt(baseURL) {
		return false, nil
	}
	pid, err := findOpenCodeServerPID(host, port)
	if err != nil {
		return false, err
	}
	if pid <= 0 || pid == os.Getpid() {
		return false, nil
	}
	cmdline, err := readOpenCodeProcessCmdline(pid)
	if err != nil {
		return false, err
	}
	if !matchesOpenCodeServeProcess(provider, cmdline, cmdName) {
		return false, nil
	}
	if sink != nil {
		sink.Write("stderr", []byte(fmt.Sprintf("opencode stale cleanup: terminating pid %d (%s)\n", pid, strings.TrimSpace(cmdline))))
	}
	if err := terminateOpenCodeProcess(pid, host, port); err != nil {
		return false, err
	}
	if sink != nil {
		sink.Write("stderr", []byte(fmt.Sprintf("opencode stale cleanup: released %s via pid %d\n", net.JoinHostPort(host, port), pid)))
	}
	return true, nil
}

func resolveOpenCodeServeCommand(provider string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	coreCfg := loadCoreConfigOrDefault()
	if configured := strings.TrimSpace(coreCfg.ProviderCommand(provider)); configured != "" {
		return lookupCommand(configured)
	}
	switch provider {
	case "kilocode":
		return lookupCommand("kilocode")
	default:
		return lookupCommand("opencode")
	}
}

func withOpenCodeServerPassword(env []string, provider, token string) []string {
	token = strings.TrimSpace(token)
	if token == "" {
		return env
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	out := upsertEnvValue(env, "OPENCODE_SERVER_PASSWORD", token)
	if provider == "kilocode" {
		out = upsertEnvValue(out, "KILOCODE_SERVER_PASSWORD", token)
	}
	return out
}

func upsertEnvValue(values []string, key, value string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return values
	}
	prefix := key + "="
	out := make([]string, 0, len(values)+1)
	replaced := false
	for _, entry := range values {
		if strings.HasPrefix(entry, prefix) {
			out = append(out, prefix+value)
			replaced = true
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}

func isLocalOpenCodeHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func allowOpenCodeAutoStart(baseURL string) bool {
	openCodeAutoStartState.mu.Lock()
	defer openCodeAutoStartState.mu.Unlock()
	now := time.Now().UTC()
	last := openCodeAutoStartState.lastAttempt[baseURL]
	if !last.IsZero() && now.Sub(last) < 5*time.Second {
		return false
	}
	openCodeAutoStartState.lastAttempt[baseURL] = now
	return true
}

func allowOpenCodeCleanupAttempt(baseURL string) bool {
	openCodeAutoStartState.mu.Lock()
	defer openCodeAutoStartState.mu.Unlock()
	now := time.Now().UTC()
	last := openCodeAutoStartState.lastCleanup[baseURL]
	if !last.IsZero() && now.Sub(last) < 5*time.Second {
		return false
	}
	openCodeAutoStartState.lastCleanup[baseURL] = now
	return true
}

func matchesOpenCodeServeProcess(provider, cmdline, cmdName string) bool {
	tokens := normalizedCmdlineTokens(cmdline)
	if len(tokens) == 0 || !containsToken(tokens, "serve") {
		return false
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "kilocode":
		if !containsToken(tokens, "kilocode") && !containsToken(tokens, "kilo") {
			return false
		}
	default:
		if !containsToken(tokens, "opencode") {
			return false
		}
	}
	cmdBase := strings.ToLower(strings.TrimSpace(filepath.Base(strings.TrimSpace(cmdName))))
	if cmdBase != "" && !containsToken(tokens, cmdBase) {
		return false
	}
	return true
}

func normalizedCmdlineTokens(cmdline string) []string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(cmdline)))
	out := make([]string, 0, len(fields)*2)
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		out = append(out, field)
		base := strings.TrimSpace(filepath.Base(field))
		if base != "" && base != field {
			out = append(out, base)
		}
	}
	return out
}

func containsToken(tokens []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, token := range tokens {
		if token == want {
			return true
		}
	}
	return false
}

func isOpenCodeUnreachable(err error) bool {
	if err == nil {
		return false
	}
	var reqErr *openCodeRequestError
	if errors.As(err, &reqErr) {
		// Treat gateway/unavailable upstream as unreachable.
		if reqErr.StatusCode == 502 || reqErr.StatusCode == 503 || reqErr.StatusCode == 504 {
			return true
		}
		return false
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connect: cannot assign requested address") ||
		strings.Contains(msg, "dial tcp") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "timeout")
}

func openCodeServerBaseURL(scheme, host, port string) string {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme == "" {
		scheme = "http"
	}
	u := &url.URL{
		Scheme: scheme,
		Host:   net.JoinHostPort(strings.TrimSpace(host), strings.TrimSpace(port)),
	}
	return strings.TrimRight(u.String(), "/")
}

func pickOpenCodeFallbackPortImpl(host string) (string, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(strings.TrimSpace(host), "0"))
	if err != nil {
		return "", fmt.Errorf("allocate fallback port: %w", err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "", fmt.Errorf("resolve fallback port: %w", err)
	}
	if strings.TrimSpace(port) == "" {
		return "", errors.New("fallback port is empty")
	}
	return port, nil
}

func waitForOpenCodePortClosedImpl(host, port string, timeout time.Duration) bool {
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if host == "" {
		host = "127.0.0.1"
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 200*time.Millisecond)
		if err != nil {
			return true
		}
		_ = conn.Close()
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func terminateOpenCodeProcessImpl(pid int, host, port string) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := signalTerminate(process); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	if waitForOpenCodePortClosed(host, port, 4*time.Second) {
		return nil
	}
	if err := signalKill(process); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	if !waitForOpenCodePortClosed(host, port, 2*time.Second) {
		return errors.New("stale process did not release server port")
	}
	return nil
}

func probeOpenCodeServerImpl(baseURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/config/providers", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden:
		return nil
	default:
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
}

func waitForOpenCodeServerReadyImpl(baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	delay := 200 * time.Millisecond
	var lastErr error
	for time.Now().Before(deadline) {
		if err := probeOpenCodeServer(baseURL); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(delay)
		if delay < time.Second {
			delay *= 2
		}
	}
	if lastErr == nil {
		lastErr = errors.New("server did not become ready")
	}
	return lastErr
}

func rememberOpenCodeRuntimeBaseURL(provider, baseURL string) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	baseURL = strings.TrimSpace(baseURL)
	if provider == "" || baseURL == "" {
		return
	}
	openCodeAutoStartState.mu.Lock()
	defer openCodeAutoStartState.mu.Unlock()
	openCodeAutoStartState.runtimeURL[provider] = strings.TrimRight(baseURL, "/")
}

func resolveOpenCodeRuntimeBaseURL(provider, fallback string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	fallback = strings.TrimSpace(fallback)
	openCodeAutoStartState.mu.Lock()
	defer openCodeAutoStartState.mu.Unlock()
	if provider != "" {
		if value := strings.TrimSpace(openCodeAutoStartState.runtimeURL[provider]); value != "" {
			return value
		}
	}
	return fallback
}
