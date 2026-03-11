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

func TestDefaultTranscriptRecoverySchedulerPlan(t *testing.T) {
	scheduler := defaultTranscriptRecoveryScheduler{}
	empty := scheduler.Plan(TranscriptRecoveryRequest{})
	if empty.FetchTranscriptSnapshot || empty.FetchHistory || empty.FetchApprovals {
		t.Fatalf("expected empty session request to produce empty recovery plan, got %#v", empty)
	}

	planned := scheduler.Plan(TranscriptRecoveryRequest{SessionID: "s1", Provider: "codex"})
	if !planned.FetchTranscriptSnapshot || !planned.FetchHistory || !planned.FetchApprovals {
		t.Fatalf("expected non-empty request to enable recovery fetches, got %#v", planned)
	}
	if planned.SnapshotSource != transcriptAttachmentSourceRecovery || !planned.AuthoritativeSnapshot {
		t.Fatalf("expected recovery plan to require authoritative recovery snapshot, got %#v", planned)
	}
}

func TestTranscriptHealthModelOptionsInstallFallbacks(t *testing.T) {
	model := NewModel(nil,
		WithTranscriptSignalClassifier(nil),
		WithStreamHealthPolicy(nil),
		WithTranscriptRecoveryScheduler(nil),
	)
	if model.transcriptSignalClassifierOrDefault() == nil {
		t.Fatalf("expected signal classifier fallback")
	}
	if model.streamHealthPolicyOrDefault() == nil {
		t.Fatalf("expected stream health policy fallback")
	}
	if model.transcriptRecoverySchedulerOrDefault() == nil {
		t.Fatalf("expected recovery scheduler fallback")
	}

	model = NewModel(nil)
	WithTranscriptSignalClassifier(defaultTranscriptSignalClassifier{})(&model)
	WithStreamHealthPolicy(defaultStreamHealthPolicy{})(&model)
	WithTranscriptRecoveryScheduler(defaultTranscriptRecoveryScheduler{})(&model)
	if model.transcriptSignalClassifier == nil || model.streamHealthPolicy == nil || model.transcriptRecoveryScheduler == nil {
		t.Fatalf("expected explicit health options to install provided implementations")
	}
}
