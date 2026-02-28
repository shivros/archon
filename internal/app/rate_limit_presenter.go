package app

import (
	"fmt"
	"strings"
	"time"
)

func presentRateLimitSystemText(item map[string]any, now time.Time, providerPolicy ProviderDisplayPolicy) (string, bool) {
	if item == nil {
		return "", false
	}
	if strings.TrimSpace(asString(item["type"])) != "rateLimit" {
		return "", false
	}
	if providerPolicy == nil {
		providerPolicy = DefaultProviderDisplayPolicy()
	}
	provider := strings.ToLower(strings.TrimSpace(asString(item["provider"])))
	name := providerPolicy.DisplayName(provider)
	retryAt := parseChatTimestamp(item["retry_at"])
	if retryAt.IsZero() {
		retryAt = parseChatTimestamp(item["retry_unix"])
	}
	lines := []string{fmt.Sprintf("%s is rate-limited.", name)}
	if retryAt.IsZero() {
		lines = append(lines, "Try again later.")
		return strings.Join(lines, "\n"), true
	}
	if now.IsZero() {
		now = time.Now()
	}
	local := retryAt.Local()
	if !local.After(now.Local()) {
		lines = append(lines, "Try again now.")
		return strings.Join(lines, "\n"), true
	}
	lines = append(lines, fmt.Sprintf("Try again at %s (%s).", local.Format("Jan 2, 3:04 PM"), formatRateLimitWait(now, retryAt)))
	return strings.Join(lines, "\n"), true
}

func formatRateLimitWait(now, retryAt time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	wait := retryAt.Sub(now)
	if wait <= 0 {
		return "available now"
	}
	if wait < time.Minute {
		return "in under a minute"
	}
	if wait < time.Hour {
		mins := int(wait.Round(time.Minute).Minutes())
		if mins <= 1 {
			return "in 1 minute"
		}
		return fmt.Sprintf("in %d minutes", mins)
	}
	hours := int(wait.Round(time.Hour).Hours())
	if hours <= 1 {
		return "in 1 hour"
	}
	return fmt.Sprintf("in %d hours", hours)
}
