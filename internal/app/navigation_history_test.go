package app

import "testing"

func TestSelectionHistoryBackForwardAndVisit(t *testing.T) {
	h := NewSelectionHistory(8)
	h.Visit("sess:s1")
	h.Visit("sess:s2")
	h.Visit("sess:s3")

	if got, ok := h.Back(nil); !ok || got != "sess:s2" {
		t.Fatalf("expected back to sess:s2, got ok=%v key=%q", ok, got)
	}
	if got, ok := h.Back(nil); !ok || got != "sess:s1" {
		t.Fatalf("expected back to sess:s1, got ok=%v key=%q", ok, got)
	}
	if got, ok := h.Forward(nil); !ok || got != "sess:s2" {
		t.Fatalf("expected forward to sess:s2, got ok=%v key=%q", ok, got)
	}

	h.Visit("sess:s4")
	if _, ok := h.Forward(nil); ok {
		t.Fatalf("expected forward branch to clear after new visit")
	}
}

func TestSelectionHistoryBackSkipsInvalidKeysWithoutMovingWhenNoneValid(t *testing.T) {
	h := NewSelectionHistory(8)
	h.Visit("ws:one")
	h.Visit("ws:two")

	if _, ok := h.Back(func(string) bool { return false }); ok {
		t.Fatalf("expected no back target when all keys are invalid")
	}
	if got, ok := h.Back(nil); !ok || got != "ws:one" {
		t.Fatalf("expected next back call to still reach ws:one, got ok=%v key=%q", ok, got)
	}
}

func TestSelectionHistoryBackAndForwardSkipInvalidEntries(t *testing.T) {
	h := NewSelectionHistory(8)
	h.Visit("sess:a")
	h.Visit("sess:b")
	h.Visit("sess:c")

	if got, ok := h.Back(func(key string) bool { return key != "sess:b" }); !ok || got != "sess:a" {
		t.Fatalf("expected back to skip sess:b and land on sess:a, got ok=%v key=%q", ok, got)
	}
	if got, ok := h.Forward(func(key string) bool { return key != "sess:b" }); !ok || got != "sess:c" {
		t.Fatalf("expected forward to skip sess:b and land on sess:c, got ok=%v key=%q", ok, got)
	}
}
