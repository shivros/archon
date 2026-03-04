package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type captureTranscriptRecoveryScheduler struct {
	calls    int
	sessions []string
}

func (s *captureTranscriptRecoveryScheduler) Schedule(_ *Model, sessionID, _ string) tea.Cmd {
	s.calls++
	s.sessions = append(s.sessions, sessionID)
	return func() tea.Msg { return nil }
}

func TestMaybeRecoverTranscriptFromControlOnlySignalsTriggersAfterSustainedBatches(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")
	m.requestActivity.lastVisibleAt = time.Now().UTC().Add(-10 * time.Second)
	scheduler := &captureTranscriptRecoveryScheduler{}
	m.transcriptRecoveryScheduler = scheduler

	now := time.Now().UTC()
	if cmd := m.maybeRecoverTranscriptFromControlOnlySignals(now, "s1", "codex", TranscriptTickSignals{
		Events:        6,
		ControlEvents: 6,
	}); cmd != nil {
		t.Fatalf("expected first control-only batch not to trigger recovery")
	}
	cmd := m.maybeRecoverTranscriptFromControlOnlySignals(now.Add(1*time.Second), "s1", "codex", TranscriptTickSignals{
		Events:        6,
		ControlEvents: 6,
	})
	if cmd == nil {
		t.Fatalf("expected second sustained control-only batch to trigger recovery")
	}
	if scheduler.calls != 1 {
		t.Fatalf("expected one recovery call, got %d", scheduler.calls)
	}
	if scheduler.sessions[0] != "s1" {
		t.Fatalf("expected recovery for s1, got %#v", scheduler.sessions)
	}
}

func TestMaybeRecoverTranscriptFromControlOnlySignalsRespectsCooldownAndContentReset(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")
	m.requestActivity.lastVisibleAt = time.Now().UTC().Add(-10 * time.Second)
	scheduler := &captureTranscriptRecoveryScheduler{}
	m.transcriptRecoveryScheduler = scheduler

	base := time.Now().UTC()
	_ = m.maybeRecoverTranscriptFromControlOnlySignals(base, "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6})
	_ = m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(1*time.Second), "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6})
	if scheduler.calls != 1 {
		t.Fatalf("expected initial recovery call, got %d", scheduler.calls)
	}
	if cmd := m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(2*time.Second), "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6}); cmd != nil {
		t.Fatalf("expected cooldown to suppress immediate re-recovery")
	}
	if scheduler.calls != 1 {
		t.Fatalf("expected no additional recovery call during cooldown, got %d", scheduler.calls)
	}

	_ = m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(5*time.Second), "s1", "codex", TranscriptTickSignals{Events: 1, ContentEvents: 1})
	if cmd := m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(6*time.Second), "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6}); cmd != nil {
		t.Fatalf("expected post-content first control batch not to recover")
	}
	cmd := m.maybeRecoverTranscriptFromControlOnlySignals(base.Add(7*time.Second), "s1", "codex", TranscriptTickSignals{Events: 6, ControlEvents: 6})
	if cmd == nil {
		t.Fatalf("expected second post-content control batch to recover")
	}
	if scheduler.calls != 2 {
		t.Fatalf("expected second recovery call after reset, got %d", scheduler.calls)
	}
}
