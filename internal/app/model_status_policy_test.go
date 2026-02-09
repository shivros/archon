package app

import "testing"

func TestSetStatusMessageDoesNotCreateToast(t *testing.T) {
	m := NewModel(nil)
	m.setStatusMessage("loading sessions")

	if m.status != "loading sessions" {
		t.Fatalf("expected status message, got %q", m.status)
	}
	if m.toastText != "" {
		t.Fatalf("expected no toast for status-only event, got %q", m.toastText)
	}
}

func TestSetBackgroundStatusDoesNotCreateToast(t *testing.T) {
	m := NewModel(nil)
	m.setBackgroundStatus("streaming")

	if m.status != "streaming" {
		t.Fatalf("expected background status message, got %q", m.status)
	}
	if m.toastText != "" {
		t.Fatalf("expected no toast for background status event, got %q", m.toastText)
	}
}

func TestSetCopyStatusUsesCopyPolicy(t *testing.T) {
	m := NewModel(nil)
	m.setCopyStatusWarning("no session selected")

	if m.status != "no session selected" {
		t.Fatalf("expected status message, got %q", m.status)
	}
	if m.toastText != "no session selected" {
		t.Fatalf("expected toast text, got %q", m.toastText)
	}
	if m.toastLevel != toastLevelWarning {
		t.Fatalf("expected warning toast level, got %v", m.toastLevel)
	}
}

func TestSetValidationStatusUsesWarningPolicy(t *testing.T) {
	m := NewModel(nil)
	m.setValidationStatus("name is required")

	if m.status != "name is required" {
		t.Fatalf("expected status message, got %q", m.status)
	}
	if m.toastText != "name is required" {
		t.Fatalf("expected toast text, got %q", m.toastText)
	}
	if m.toastLevel != toastLevelWarning {
		t.Fatalf("expected warning toast level, got %v", m.toastLevel)
	}
}

func TestSetStatusErrorUsesActionPolicy(t *testing.T) {
	m := NewModel(nil)
	m.setStatusError("send error: boom")

	if m.status != "send error: boom" {
		t.Fatalf("expected status message, got %q", m.status)
	}
	if m.toastText != "send error: boom" {
		t.Fatalf("expected toast text, got %q", m.toastText)
	}
	if m.toastLevel != toastLevelError {
		t.Fatalf("expected error toast level, got %v", m.toastLevel)
	}
}

func TestSetBackgroundErrorUsesErrorPolicy(t *testing.T) {
	m := NewModel(nil)
	m.setBackgroundError("stream error: boom")

	if m.status != "stream error: boom" {
		t.Fatalf("expected status message, got %q", m.status)
	}
	if m.toastText != "stream error: boom" {
		t.Fatalf("expected toast text, got %q", m.toastText)
	}
	if m.toastLevel != toastLevelError {
		t.Fatalf("expected error toast level, got %v", m.toastLevel)
	}
}
