package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	osc52 "github.com/aymanbagabas/go-osc52/v2"
)

type clipboardMethod uint8

const (
	clipboardMethodSystem clipboardMethod = iota
	clipboardMethodOSC52
	clipboardCopyTimeout   = 2 * time.Second
	clipboardCopyQueueSize = 16
)

type ClipboardService interface {
	Copy(context.Context, string) (clipboardMethod, error)
}

type ClipboardBackend interface {
	Copy(text string) (clipboardMethod, error)
}

type ClipboardCopyRunner interface {
	Copy(context.Context, string) (clipboardMethod, error)
}

type defaultClipboardService struct{}

type defaultClipboardBackend struct{}

type clipboardCopyResult struct {
	method clipboardMethod
	err    error
}

type clipboardCopyRequest struct {
	text   string
	result chan clipboardCopyResult
}

type queuedClipboardCopyRunner struct {
	requests chan clipboardCopyRequest
}

type runnerClipboardService struct {
	runner ClipboardCopyRunner
}

var (
	defaultClipboardRunnerOnce sync.Once
	defaultClipboardRunner     ClipboardCopyRunner
	newDefaultClipboardRunner  = func() ClipboardCopyRunner {
		return NewQueuedClipboardCopyRunner(defaultClipboardBackend{}, clipboardCopyQueueSize)
	}
)

func (defaultClipboardBackend) Copy(text string) (clipboardMethod, error) {
	return copyTextToClipboard(text)
}

func NewQueuedClipboardCopyRunner(backend ClipboardBackend, queueSize int) ClipboardCopyRunner {
	if backend == nil {
		backend = defaultClipboardBackend{}
	}
	if queueSize <= 0 {
		queueSize = 1
	}
	runner := &queuedClipboardCopyRunner{
		requests: make(chan clipboardCopyRequest, queueSize),
	}
	go func() {
		for request := range runner.requests {
			method, err := backend.Copy(request.text)
			request.result <- clipboardCopyResult{method: method, err: err}
		}
	}()
	return runner
}

func (r *queuedClipboardCopyRunner) Copy(ctx context.Context, text string) (clipboardMethod, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return clipboardMethodSystem, err
	}
	request := clipboardCopyRequest{
		text:   text,
		result: make(chan clipboardCopyResult, 1),
	}
	select {
	case <-ctx.Done():
		return clipboardMethodSystem, ctx.Err()
	case r.requests <- request:
	}
	select {
	case <-ctx.Done():
		return clipboardMethodSystem, ctx.Err()
	case result := <-request.result:
		return result.method, result.err
	}
}

func (s runnerClipboardService) Copy(ctx context.Context, text string) (clipboardMethod, error) {
	if s.runner == nil {
		return clipboardMethodSystem, fmt.Errorf("clipboard runner is not configured")
	}
	return s.runner.Copy(ctx, text)
}

func resolveDefaultClipboardRunner() ClipboardCopyRunner {
	defaultClipboardRunnerOnce.Do(func() {
		defaultClipboardRunner = newDefaultClipboardRunner()
	})
	return defaultClipboardRunner
}

func (defaultClipboardService) Copy(ctx context.Context, text string) (clipboardMethod, error) {
	runner := resolveDefaultClipboardRunner()
	if runner == nil {
		return clipboardMethodSystem, fmt.Errorf("default clipboard runner is not configured")
	}
	return runner.Copy(ctx, text)
}

func WithClipboardCopyRunner(runner ClipboardCopyRunner) ModelOption {
	return func(m *Model) {
		if m == nil || runner == nil {
			return
		}
		m.clipboard = runnerClipboardService{runner: runner}
	}
}

func WithClipboardBackend(backend ClipboardBackend) ModelOption {
	if backend == nil {
		return nil
	}
	return WithClipboardCopyRunner(NewQueuedClipboardCopyRunner(backend, clipboardCopyQueueSize))
}

func WithClipboardService(service ClipboardService) ModelOption {
	return func(m *Model) {
		if m == nil || service == nil {
			return
		}
		m.clipboard = service
	}
}

func (m *Model) copyWithStatusCmd(text, success string) tea.Cmd {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	service := m.clipboard
	if service == nil {
		service = defaultClipboardService{}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), clipboardCopyTimeout)
		defer cancel()
		_, err := service.Copy(ctx, text)
		return clipboardResultMsg{
			success: success,
			err:     err,
		}
	}
}

var clipboardWriteAll = clipboard.WriteAll
var clipboardWriteOSC52 = writeOSC52Clipboard
var openTTYForWrite = func() (io.WriteCloser, error) {
	return os.OpenFile("/dev/tty", os.O_WRONLY, 0)
}

func copyTextToClipboard(text string) (clipboardMethod, error) {
	if err := clipboardWriteAll(text); err == nil {
		return clipboardMethodSystem, nil
	} else {
		if oscErr := clipboardWriteOSC52(text); oscErr == nil {
			return clipboardMethodOSC52, nil
		} else {
			return clipboardMethodSystem, combineClipboardErrors(err, oscErr)
		}
	}
}

func writeOSC52Clipboard(text string) error {
	tty, err := openTTYForWrite()
	if err != nil {
		return fmt.Errorf("open /dev/tty: %w", err)
	}
	defer tty.Close()
	return writeOSC52Sequence(tty, text)
}

func writeOSC52Sequence(w io.Writer, text string) error {
	termName := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if os.Getenv("TMUX") != "" {
		// Emit both plain and tmux-wrapped OSC52 for compatibility with
		// different tmux clipboard configurations.
		if _, err := osc52.New(text).WriteTo(w); err != nil {
			return err
		}
		if _, err := osc52.New(text).Tmux().WriteTo(w); err != nil {
			return err
		}
		return nil
	} else if strings.HasPrefix(termName, "screen") {
		if _, err := osc52.New(text).Screen().WriteTo(w); err != nil {
			return err
		}
		return nil
	}
	if _, err := osc52.New(text).WriteTo(w); err != nil {
		return err
	}
	return nil
}

func combineClipboardErrors(systemErr, oscErr error) error {
	systemMsg := humanizeClipboardError(systemErr)
	oscMsg := humanizeClipboardError(oscErr)
	if missingDisplay() {
		return fmt.Errorf("no GUI clipboard available (DISPLAY/WAYLAND_DISPLAY unset); OSC52 fallback failed: %s", oscMsg)
	}
	return fmt.Errorf("system clipboard failed: %s; OSC52 fallback failed: %s", systemMsg, oscMsg)
}

func humanizeClipboardError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "exit status 1" {
		if missingDisplay() {
			return "no GUI clipboard available (DISPLAY/WAYLAND_DISPLAY unset)"
		}
		return "clipboard helper exited with status 1"
	}
	return msg
}

func missingDisplay() bool {
	return strings.TrimSpace(os.Getenv("DISPLAY")) == "" && strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) == ""
}
