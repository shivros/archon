package daemon

import (
	"bytes"
	"strings"
	"testing"

	"control/internal/logging"
)

func TestItemTimestampLogMetricsSinkEmitsPeriodicAndFinal(t *testing.T) {
	var out bytes.Buffer
	sink := newItemTimestampLogMetricsSink(logging.New(&out, logging.Info))
	sink.logEvery = 2

	sink.Record(itemTimestampClassification{HasProviderTimestamp: false, UsedDaemonTimestamp: true})
	sink.Record(itemTimestampClassification{HasProviderTimestamp: true, UsedDaemonTimestamp: false})
	sink.Close()

	logs := out.String()
	if strings.Count(logs, "msg=item_timestamp_stats") != 2 {
		t.Fatalf("expected periodic and final telemetry logs, got %q", logs)
	}
	if !strings.Contains(logs, "missing_provider_timestamp_count=1") {
		t.Fatalf("expected missing provider timestamp count in telemetry, got %q", logs)
	}
	if !strings.Contains(logs, "daemon_filled_count=1") {
		t.Fatalf("expected daemon filled count in telemetry, got %q", logs)
	}
}

func TestItemTimestampLogMetricsSinkCloseNoItemsNoLog(t *testing.T) {
	var out bytes.Buffer
	sink := newItemTimestampLogMetricsSink(logging.New(&out, logging.Info))
	sink.Close()
	if strings.TrimSpace(out.String()) != "" {
		t.Fatalf("expected no telemetry log when no items were recorded, got %q", out.String())
	}
}
