package app

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCopyTextToClipboardUsesSystemBackend(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	t.Cleanup(func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	})

	fallbackCalled := false
	clipboardWriteAll = func(string) error { return nil }
	clipboardWriteOSC52 = func(string) error {
		fallbackCalled = true
		return nil
	}

	method, err := copyTextToClipboard("hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if method != clipboardMethodSystem {
		t.Fatalf("expected system method, got %v", method)
	}
	if fallbackCalled {
		t.Fatalf("expected no OSC52 fallback call")
	}
}

func TestCopyTextToClipboardFallsBackToOSC52(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	t.Cleanup(func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	})

	fallbackCalled := false
	clipboardWriteAll = func(string) error { return errors.New("exit status 1") }
	clipboardWriteOSC52 = func(string) error {
		fallbackCalled = true
		return nil
	}

	method, err := copyTextToClipboard("hello")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if method != clipboardMethodOSC52 {
		t.Fatalf("expected OSC52 method, got %v", method)
	}
	if !fallbackCalled {
		t.Fatalf("expected OSC52 fallback call")
	}
}

func TestCopyTextToClipboardHelpfulErrorWhenDisplayMissing(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	t.Cleanup(func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	})

	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("TERM", "xterm-256color")

	clipboardWriteAll = func(string) error { return errors.New("exit status 1") }
	clipboardWriteOSC52 = func(string) error { return errors.New("open /dev/tty: no such device") }

	_, err := copyTextToClipboard("hello")
	if err == nil {
		t.Fatalf("expected copy error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no GUI clipboard available") {
		t.Fatalf("expected no-display guidance, got %q", msg)
	}
	if !strings.Contains(msg, "OSC52 fallback failed") {
		t.Fatalf("expected OSC52 fallback details, got %q", msg)
	}
}

func TestWriteOSC52ClipboardReportsTTYError(t *testing.T) {
	origOpenTTY := openTTYForWrite
	t.Cleanup(func() { openTTYForWrite = origOpenTTY })
	openTTYForWrite = func() (io.WriteCloser, error) {
		return nil, os.ErrNotExist
	}

	err := writeOSC52Clipboard("hello")
	if err == nil {
		t.Fatalf("expected writeOSC52Clipboard to fail without /dev/tty in test process")
	}
	if !strings.Contains(err.Error(), "open /dev/tty") {
		t.Fatalf("expected /dev/tty error, got %q", err.Error())
	}
}

func TestDefaultClipboardServiceCopyHonorsContextCancellation(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	t.Cleanup(func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	})

	clipboardWriteAll = func(string) error {
		time.Sleep(40 * time.Millisecond)
		return nil
	}
	clipboardWriteOSC52 = func(string) error { return nil }

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := defaultClipboardService{}.Copy(ctx, "hello")
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
