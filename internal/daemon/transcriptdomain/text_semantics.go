package transcriptdomain

import "strings"

// IsSemanticallyEmpty reports whether transcript text carries no user-visible content.
func IsSemanticallyEmpty(text string) bool {
	return strings.TrimSpace(text) == ""
}

// PreserveText is an explicit no-op boundary for transcript text pipelines.
func PreserveText(text string) string {
	return text
}
