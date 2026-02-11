package app

import (
	"fmt"
	"strings"
)

type codexReasoningAccumulator struct {
	groupID  string
	segments map[string]string
	order    []string
	anonSeq  int
	lastText string
}

func newCodexReasoningAccumulator(groupID string) *codexReasoningAccumulator {
	acc := &codexReasoningAccumulator{}
	acc.Reset(groupID)
	return acc
}

func (a *codexReasoningAccumulator) Reset(groupID string) {
	if a == nil {
		return
	}
	a.groupID = strings.TrimSpace(groupID)
	a.segments = map[string]string{}
	a.order = nil
	a.anonSeq = 0
	a.lastText = ""
}

func (a *codexReasoningAccumulator) Add(itemID, text string) (aggregateID string, aggregateText string, changed bool) {
	if a == nil {
		return "", "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", false
	}
	key := strings.TrimSpace(itemID)
	if key == "" {
		a.anonSeq++
		key = fmt.Sprintf("__anon_%d", a.anonSeq)
	}
	if _, ok := a.segments[key]; !ok {
		a.order = append(a.order, key)
	}
	a.segments[key] = text
	combined := a.combinedText()
	if combined == "" {
		return "", "", false
	}
	changed = strings.TrimSpace(combined) != strings.TrimSpace(a.lastText)
	a.lastText = combined
	return a.groupID, combined, changed
}

func (a *codexReasoningAccumulator) combinedText() string {
	if a == nil || len(a.order) == 0 {
		return ""
	}
	parts := make([]string, 0, len(a.order))
	for _, key := range a.order {
		text := strings.TrimSpace(a.segments[key])
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
