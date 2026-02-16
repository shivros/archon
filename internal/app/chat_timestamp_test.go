package app

import (
	"testing"
	"time"
)

func TestDefaultChatTimestampFormatterRelative(t *testing.T) {
	formatter := defaultChatTimestampFormatter{}
	now := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	created := now.Add(-2 * time.Minute)

	if got := formatter.FormatTimestamp(created, now, ChatTimestampModeRelative); got != "2 minutes ago" {
		t.Fatalf("unexpected relative timestamp: %q", got)
	}
}

func TestDefaultChatTimestampFormatterISO(t *testing.T) {
	formatter := defaultChatTimestampFormatter{}
	created := time.Date(2026, 2, 16, 12, 34, 56, 0, time.UTC)

	if got := formatter.FormatTimestamp(created, time.Time{}, ChatTimestampModeISO); got != "2026-02-16T12:34:56Z" {
		t.Fatalf("unexpected ISO timestamp: %q", got)
	}
}

func TestParseChatTimestampModeNormalizesUnknown(t *testing.T) {
	if got := parseChatTimestampMode(" weird "); got != ChatTimestampModeRelative {
		t.Fatalf("unexpected normalized mode: %q", got)
	}
	if got := parseChatTimestampMode(" ISO "); got != ChatTimestampModeISO {
		t.Fatalf("unexpected ISO normalized mode: %q", got)
	}
}
