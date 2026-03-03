package app

import (
	"testing"

	"control/internal/daemon/transcriptdomain"
)

type fixedRecentsCompletionSignalPolicy struct {
	turnID  string
	matched bool
}

func (f fixedRecentsCompletionSignalPolicy) CompletionFromTranscriptEvent(transcriptdomain.TranscriptEvent) (string, bool) {
	return f.turnID, f.matched
}

func TestRecentsCompletionSignalPolicyOrDefaultUsesDefaultPolicy(t *testing.T) {
	m := NewModel(nil)
	m.recentsCompletionSignalPolicy = nil
	policy := m.recentsCompletionSignalPolicyOrDefault()
	if policy == nil {
		t.Fatalf("expected default signal policy")
	}
}

func TestRecentsCompletionSignalPolicyOrDefaultNilModelUsesDefault(t *testing.T) {
	var m *Model
	policy := m.recentsCompletionSignalPolicyOrDefault()
	if policy == nil {
		t.Fatalf("expected default signal policy for nil model")
	}
	_, matched := policy.CompletionFromTranscriptEvent(transcriptdomain.TranscriptEvent{
		Kind: transcriptdomain.TranscriptEventTurnCompleted,
	})
	if !matched {
		t.Fatalf("expected default signal policy behavior")
	}
}

func TestWithRecentsCompletionSignalPolicyNilUsesDefault(t *testing.T) {
	m := NewModel(nil)
	opt := WithRecentsCompletionSignalPolicy(nil)
	opt(&m)

	turnID, matched := m.recentsCompletionSignalPolicyOrDefault().CompletionFromTranscriptEvent(
		transcriptdomain.TranscriptEvent{
			Kind: transcriptdomain.TranscriptEventTurnCompleted,
			Turn: &transcriptdomain.TurnState{TurnID: "turn-a"},
		},
	)
	if !matched || turnID != "turn-a" {
		t.Fatalf("expected default signal policy behavior, got matched=%v turnID=%q", matched, turnID)
	}
}

func TestWithRecentsCompletionSignalPolicyAssignsCustom(t *testing.T) {
	m := NewModel(nil)
	opt := WithRecentsCompletionSignalPolicy(fixedRecentsCompletionSignalPolicy{turnID: "turn-custom", matched: true})
	opt(&m)
	turnID, matched := m.recentsCompletionSignalPolicyOrDefault().CompletionFromTranscriptEvent(transcriptdomain.TranscriptEvent{})
	if !matched || turnID != "turn-custom" {
		t.Fatalf("expected custom signal policy behavior, got matched=%v turnID=%q", matched, turnID)
	}
}

func TestWithRecentsCompletionSignalPolicyNilModelNoop(t *testing.T) {
	var m *Model
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("expected nil-model option to be safe, panic=%v", r)
		}
	}()
	WithRecentsCompletionSignalPolicy(fixedRecentsCompletionSignalPolicy{turnID: "x", matched: true})(m)
}

func TestTranscriptEventRecentsCompletionSignalPolicyTrimsTurnID(t *testing.T) {
	policy := transcriptEventRecentsCompletionSignalPolicy{}
	turnID, matched := policy.CompletionFromTranscriptEvent(transcriptdomain.TranscriptEvent{
		Kind: transcriptdomain.TranscriptEventTurnCompleted,
		Turn: &transcriptdomain.TurnState{TurnID: "  turn-space  "},
	})
	if !matched || turnID != "turn-space" {
		t.Fatalf("expected trimmed turn id, got matched=%v turnID=%q", matched, turnID)
	}
}
