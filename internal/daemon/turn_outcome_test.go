package daemon

import "testing"

func TestClassifyTurnOutcome(t *testing.T) {
	out := classifyTurnOutcome("failed", "")
	if !out.Terminal || !out.Failed {
		t.Fatalf("expected failed status to be terminal+failed, got %#v", out)
	}
	out = classifyTurnOutcome("completed", "")
	if !out.Terminal || out.Failed {
		t.Fatalf("expected completed status to be terminal success, got %#v", out)
	}
	out = classifyTurnOutcome("", "provider exploded")
	if !out.Terminal || !out.Failed {
		t.Fatalf("expected explicit error to be terminal+failed, got %#v", out)
	}
	out = classifyTurnOutcome("in_progress", "")
	if out.Terminal || out.Failed {
		t.Fatalf("expected in_progress to be non-terminal, got %#v", out)
	}
}
