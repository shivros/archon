package app

import (
	"fmt"
	"testing"
)

func TestTranscriptSessionTraceCapsEntriesAndReturnsCopy(t *testing.T) {
	m := NewModel(nil)
	for i := 0; i < 70; i++ {
		m.appendTranscriptSessionTrace("s1", "event-%d", i)
	}

	entries := m.transcriptSessionTrace("s1")
	if len(entries) != transcriptTraceMaxEntriesPerSession {
		t.Fatalf("expected capped trace length %d, got %d", transcriptTraceMaxEntriesPerSession, len(entries))
	}
	if entries[0].Event != "event-6" {
		t.Fatalf("expected oldest retained trace event to be event-6, got %#v", entries[0])
	}
	if entries[len(entries)-1].Event != "event-69" {
		t.Fatalf("expected latest trace event to be event-69, got %#v", entries[len(entries)-1])
	}

	entries[0].Event = "mutated"
	again := m.transcriptSessionTrace("s1")
	if again[0].Event != "event-6" {
		t.Fatalf("expected trace getter to return copy, got %#v", again[0])
	}
}

func TestTranscriptSessionTraceGuards(t *testing.T) {
	var nilModel *Model
	nilModel.appendTranscriptSessionTrace("s1", "event")
	if traces := nilModel.transcriptSessionTrace("s1"); traces != nil {
		t.Fatalf("expected nil model trace lookup to return nil, got %#v", traces)
	}

	m := NewModel(nil)
	m.appendTranscriptSessionTrace("", "event")
	m.appendTranscriptSessionTrace("s1", "   ")
	if traces := m.transcriptSessionTrace("s1"); len(traces) != 0 {
		t.Fatalf("expected blank session/event append to be ignored, got %#v", traces)
	}
	if traces := m.transcriptSessionTrace("missing"); traces != nil {
		t.Fatalf("expected missing session trace lookup to return nil, got %#v", traces)
	}

	m.appendTranscriptSessionTrace("s1", "event-%s", "ok")
	traces := m.transcriptSessionTrace("s1")
	if len(traces) != 1 || traces[0].Event != fmt.Sprintf("event-%s", "ok") {
		t.Fatalf("expected trace append to record formatted event, got %#v", traces)
	}
}
