package app

import (
	"testing"
	"time"
)

func TestUILatencyTrackerRecordsSupersededAndCompletion(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	tracker := newUILatencyTracker(sink)

	base := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	tick := 0
	tracker.nowFn = func() time.Time {
		ts := base.Add(time.Duration(tick) * time.Millisecond)
		tick++
		return ts
	}

	tracker.startAction(uiLatencyActionSwitchSession, "sess:s1")
	tracker.startAction(uiLatencyActionSwitchSession, "sess:s2")
	tracker.finishAction(uiLatencyActionSwitchSession, "sess:s1", uiLatencyOutcomeOK)
	tracker.finishAction(uiLatencyActionSwitchSession, "sess:s2", uiLatencyOutcomeOK)

	metrics := sink.Snapshot()
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d (%#v)", len(metrics), metrics)
	}

	if metrics[0].Outcome != uiLatencyOutcomeSuperseded || metrics[0].Token != "sess:s1" {
		t.Fatalf("unexpected superseded metric: %#v", metrics[0])
	}
	if metrics[1].Outcome != uiLatencyOutcomeOK || metrics[1].Token != "sess:s2" {
		t.Fatalf("unexpected completion metric: %#v", metrics[1])
	}
}

func TestUILatencyTrackerRecordsSpan(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	tracker := newUILatencyTracker(sink)

	base := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	tracker.nowFn = func() time.Time {
		return base.Add(25 * time.Millisecond)
	}
	tracker.recordSpan(uiLatencySpanModelUpdate, base)

	metrics := sink.Snapshot()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	metric := metrics[0]
	if metric.Name != uiLatencySpanModelUpdate {
		t.Fatalf("expected %q, got %q", uiLatencySpanModelUpdate, metric.Name)
	}
	if metric.Category != UILatencyCategorySpan {
		t.Fatalf("expected span metric, got %q", metric.Category)
	}
	if metric.Duration != 25*time.Millisecond {
		t.Fatalf("expected 25ms duration, got %s", metric.Duration)
	}
}
