package daemon

import "strings"

// newAgentMessageDeltaItem returns the canonical cross-provider delta shape.
// We include both delta and text fields for backward compatibility while
// standardizing consumers on "delta".
func newAgentMessageDeltaItem(delta string) map[string]any {
	if strings.TrimSpace(delta) == "" {
		return nil
	}
	return map[string]any{
		"type":  "agentMessageDelta",
		"delta": delta,
		"text":  delta,
	}
}
