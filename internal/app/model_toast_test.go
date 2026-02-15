package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestSetStatusInfoSetsStatusAndToast(t *testing.T) {
	m := NewModel(nil)
	m.setStatusInfo("message copied")

	if m.status != "message copied" {
		t.Fatalf("expected status to be set, got %q", m.status)
	}
	if m.toastText != "message copied" {
		t.Fatalf("expected toast text to be set, got %q", m.toastText)
	}
	if m.toastLevel != toastLevelInfo {
		t.Fatalf("expected info toast level, got %v", m.toastLevel)
	}
	if !m.toastActive(time.Now()) {
		t.Fatalf("expected toast to be active")
	}
}

func TestHandleTickClearsExpiredToast(t *testing.T) {
	m := NewModel(nil)
	m.showWarningToast("copied session id")

	m.handleTick(tickMsg(time.Now().Add(toastDuration + time.Millisecond)))
	if m.toastText != "" {
		t.Fatalf("expected toast to clear after expiry, got %q", m.toastText)
	}
	if m.toastLevel != toastLevelInfo {
		t.Fatalf("expected level reset after clear, got %v", m.toastLevel)
	}
}

func TestViewShowsToastOverlay(t *testing.T) {
	m := NewModel(nil)
	m.resize(100, 20)
	m.showErrorToast("copied session id")

	view := m.View()
	plain := xansi.Strip(fmt.Sprint(view.Content))
	if !strings.Contains(plain, "copied session id") {
		t.Fatalf("expected toast text in view output: %q", plain)
	}
}

func TestStartupToastQueueAdvancesAfterExpiry(t *testing.T) {
	m := NewModel(nil)
	m.enqueueStartupToast(toastLevelError, "conflict one")
	m.enqueueStartupToast(toastLevelError, "conflict two")

	if m.toastText != "conflict one" {
		t.Fatalf("expected first startup toast, got %q", m.toastText)
	}
	if len(m.startupToasts) != 1 {
		t.Fatalf("expected one queued startup toast, got %d", len(m.startupToasts))
	}

	m.handleTick(tickMsg(time.Now().Add(toastDuration + time.Millisecond)))
	if m.toastText != "conflict two" {
		t.Fatalf("expected second startup toast after expiry, got %q", m.toastText)
	}
	if len(m.startupToasts) != 0 {
		t.Fatalf("expected startup toast queue to be empty, got %d", len(m.startupToasts))
	}
}
