package app

import (
	"testing"
	"time"
)

func TestDefaultTranscriptSignalClassifierSummarizesTickSignals(t *testing.T) {
	classifier := defaultTranscriptSignalClassifier{}
	summary := classifier.Summarize("codex", TranscriptTickSignals{
		Events:        5,
		ContentEvents: 2,
		ControlEvents: 3,
	})
	if summary.Total != 5 || summary.Content != 2 || summary.Control != 3 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
}

func TestDefaultStreamHealthPolicyRequiresCodexControlOnlyAndStaleWindow(t *testing.T) {
	policy := defaultStreamHealthPolicy{}
	now := time.Now().UTC()
	if policy.ShouldRecover(StreamHealthObservation{
		SessionID:            "s1",
		Provider:             "claude",
		Now:                  now,
		LastVisibleAt:        now.Add(-10 * time.Second),
		RequestActivityAlive: true,
		Signals:              transcriptSignalSummary{Total: 8, Control: 8},
	}) {
		t.Fatalf("expected non-codex provider not to trigger recovery")
	}
	if policy.ShouldRecover(StreamHealthObservation{
		SessionID:            "s1",
		Provider:             "codex",
		Now:                  now,
		LastVisibleAt:        now.Add(-10 * time.Second),
		RequestActivityAlive: true,
		Signals:              transcriptSignalSummary{Total: 8, Control: 4},
	}) {
		t.Fatalf("expected low control-only volume not to trigger recovery")
	}
	if policy.ShouldRecover(StreamHealthObservation{
		SessionID:            "s1",
		Provider:             "codex",
		Now:                  now,
		LastVisibleAt:        now.Add(-500 * time.Millisecond),
		RequestActivityAlive: true,
		Signals:              transcriptSignalSummary{Total: 8, Control: 8},
	}) {
		t.Fatalf("expected fresh visible activity not to trigger recovery")
	}
	if !policy.ShouldRecover(StreamHealthObservation{
		SessionID:            "s1",
		Provider:             "codex",
		Now:                  now,
		LastVisibleAt:        now.Add(-10 * time.Second),
		RequestActivityAlive: true,
		Signals:              transcriptSignalSummary{Total: 8, Control: 8},
	}) {
		t.Fatalf("expected control-only stale codex stream to trigger recovery")
	}
}
