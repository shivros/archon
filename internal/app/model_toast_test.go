package app

import (
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

	plain := xansi.Strip(m.View())
	if !strings.Contains(plain, "copied session id") {
		t.Fatalf("expected toast text in view output: %q", plain)
	}
}
