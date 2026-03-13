package transcriptdomain

import "testing"

func TestNoopTranscriptDecisionLogger(t *testing.T) {
	logger := NewNoopTranscriptDecisionLogger()
	if logger == nil {
		t.Fatalf("expected noop decision logger instance")
	}
	logger.LogDecision(TranscriptDecisionLogEntry{
		Layer:    "test",
		Decision: "accepted_new",
		Reason:   "noop",
	})
}
