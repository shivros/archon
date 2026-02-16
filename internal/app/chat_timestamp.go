package app

import (
	"fmt"
	"strings"
	"time"
)

type ChatTimestampMode string

const (
	ChatTimestampModeRelative ChatTimestampMode = "relative"
	ChatTimestampModeISO      ChatTimestampMode = "iso"
)

type ChatTimestampFormatter interface {
	FormatTimestamp(createdAt time.Time, now time.Time, mode ChatTimestampMode) string
}

type defaultChatTimestampFormatter struct{}

func normalizeChatTimestampMode(raw ChatTimestampMode) ChatTimestampMode {
	switch strings.ToLower(strings.TrimSpace(string(raw))) {
	case string(ChatTimestampModeISO):
		return ChatTimestampModeISO
	default:
		return ChatTimestampModeRelative
	}
}

func parseChatTimestampMode(raw string) ChatTimestampMode {
	return normalizeChatTimestampMode(ChatTimestampMode(strings.TrimSpace(raw)))
}

func chatTimestampRenderBucket(mode ChatTimestampMode, now time.Time) int64 {
	if normalizeChatTimestampMode(mode) != ChatTimestampModeRelative {
		return -1
	}
	if now.IsZero() {
		now = time.Now()
	}
	return now.UTC().Unix() / 60
}

func (defaultChatTimestampFormatter) FormatTimestamp(createdAt time.Time, now time.Time, mode ChatTimestampMode) string {
	if createdAt.IsZero() {
		return ""
	}
	mode = normalizeChatTimestampMode(mode)
	if mode == ChatTimestampModeISO {
		return createdAt.UTC().Format(time.RFC3339)
	}
	if now.IsZero() {
		now = time.Now()
	}
	delta := now.Sub(createdAt)
	if delta < 0 {
		delta = 0
	}
	if delta < 30*time.Second {
		return "just now"
	}
	if delta < time.Minute {
		secs := int(delta.Round(time.Second).Seconds())
		if secs <= 1 {
			return "1 second ago"
		}
		return fmt.Sprintf("%d seconds ago", secs)
	}
	if delta < time.Hour {
		minutes := int(delta.Round(time.Minute).Minutes())
		if minutes <= 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}
	if delta < 24*time.Hour {
		hours := int(delta.Round(time.Hour).Hours())
		if hours <= 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(delta.Round(24*time.Hour).Hours() / 24)
	if days <= 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
