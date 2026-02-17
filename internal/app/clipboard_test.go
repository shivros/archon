package app

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

	copyDone := make(chan struct{})
	clipboardWriteAll = func(string) error {
		defer close(copyDone)
		time.Sleep(40 * time.Millisecond)
		return nil
	}
	clipboardWriteOSC52 = func(string) error { return nil }

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := defaultClipboardService{}.Copy(ctx, "hello")
	select {
	case <-copyDone:
	case <-time.After(time.Second):
		t.Fatalf("clipboard copy goroutine did not finish")
	}
	if err == nil {
		t.Fatalf("expected context cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

type observedClipboardBackend struct {
	active int32
	peak   int32
	delay  time.Duration
}

func (b *observedClipboardBackend) Copy(string) (clipboardMethod, error) {
	current := atomic.AddInt32(&b.active, 1)
	for {
		peak := atomic.LoadInt32(&b.peak)
		if current <= peak {
			break
		}
		if atomic.CompareAndSwapInt32(&b.peak, peak, current) {
			break
		}
	}
	time.Sleep(b.delay)
	atomic.AddInt32(&b.active, -1)
	return clipboardMethodSystem, nil
}

func TestQueuedClipboardCopyRunnerSerializesBackendCalls(t *testing.T) {
	backend := &observedClipboardBackend{delay: 20 * time.Millisecond}
	runner := NewQueuedClipboardCopyRunner(backend, 8)
	const workers = 6
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := runner.Copy(context.Background(), "hello")
			if err != nil {
				t.Errorf("unexpected copy error: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&backend.peak); got > 1 {
		t.Fatalf("expected runner to serialize backend calls, max in-flight=%d", got)
	}
}

type blockingClipboardBackend struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingClipboardBackend) Copy(string) (clipboardMethod, error) {
	b.started <- struct{}{}
	<-b.release
	return clipboardMethodSystem, nil
}

func TestQueuedClipboardCopyRunnerRespectsContextWhenQueueFull(t *testing.T) {
	backend := &blockingClipboardBackend{
		started: make(chan struct{}, 4),
		release: make(chan struct{}),
	}
	runner := NewQueuedClipboardCopyRunner(backend, 1)
	errs := make(chan error, 2)

	go func() {
		_, err := runner.Copy(context.Background(), "first")
		errs <- err
	}()
	<-backend.started

	go func() {
		_, err := runner.Copy(context.Background(), "second")
		errs <- err
	}()

	// Give the second request a moment to enqueue before issuing the third call.
	time.Sleep(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := runner.Copy(ctx, "third")
	if err == nil {
		t.Fatalf("expected context error for queue-full request")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}

	backend.release <- struct{}{}
	<-backend.started
	backend.release <- struct{}{}
	for i := 0; i < 2; i++ {
		if runErr := <-errs; runErr != nil {
			t.Fatalf("expected queued request to complete without error, got %v", runErr)
		}
	}
}
