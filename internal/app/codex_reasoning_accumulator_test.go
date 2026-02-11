package app

import "testing"

func TestCodexReasoningAccumulatorAggregatesByEncounterOrder(t *testing.T) {
	acc := newCodexReasoningAccumulator("group-1")
	if acc == nil {
		t.Fatalf("expected accumulator")
	}

	id, text, changed := acc.Add("r1", "- first")
	if !changed {
		t.Fatalf("expected first segment to change aggregate")
	}
	if id != "group-1" || text != "- first" {
		t.Fatalf("unexpected first aggregate id=%q text=%q", id, text)
	}

	_, text, changed = acc.Add("r2", "- second")
	if !changed {
		t.Fatalf("expected second segment to change aggregate")
	}
	if text != "- first\n\n- second" {
		t.Fatalf("unexpected aggregate text %q", text)
	}

	_, text, changed = acc.Add("r1", "- first updated")
	if !changed {
		t.Fatalf("expected id update to change aggregate")
	}
	if text != "- first updated\n\n- second" {
		t.Fatalf("unexpected updated aggregate text %q", text)
	}

	_, _, changed = acc.Add("r1", "- first updated")
	if changed {
		t.Fatalf("expected duplicate segment to be ignored")
	}
}

func TestCodexReasoningAccumulatorResetStartsFreshGroup(t *testing.T) {
	acc := newCodexReasoningAccumulator("group-1")
	acc.Add("r1", "- first")
	acc.Reset("group-2")

	id, text, changed := acc.Add("r2", "- second")
	if !changed {
		t.Fatalf("expected first post-reset segment to change aggregate")
	}
	if id != "group-2" || text != "- second" {
		t.Fatalf("unexpected post-reset aggregate id=%q text=%q", id, text)
	}
}
