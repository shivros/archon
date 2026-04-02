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

// IsIncrementalDeltaKind reports whether a block Kind represents an
// incremental streaming delta whose text is new content only (not a
// cumulative replay of previous content). The merge policy uses this
// to skip substring-containment deduplication for such blocks.
func IsIncrementalDeltaKind(kind string) bool {
	k := strings.ToLower(strings.TrimSpace(kind))
	if k == "" {
		return false
	}
	if strings.Contains(k, "delta") {
		return true
	}
	// "agentmessage" is produced by itemKindFromMethod for
	// item/agentMessage/delta codex notifications.
	if k == "agentmessage" {
		return true
	}
	return false
}
